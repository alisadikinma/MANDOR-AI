package handler

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// TestListStaleSquadIssues exercises the four gate conditions of the standup
// query against a real DB. It must return a mid-flight squad-assigned issue that
// has gone silent, and exclude issues that are the wrong status, recently
// commented (the load-bearing LATERAL join — issue.updated_at is not bumped on
// comment), already have an active task (the NOT EXISTS guard), or sit under an
// archived squad (the JOIN filter). The query is cross-workspace, so assertions
// check presence/absence of specific issue IDs, never a total count.
func TestListStaleSquadIssues(t *testing.T) {
	if testPool == nil {
		t.Skip("no database")
	}
	ctx := context.Background()
	q := db.New(testPool)

	var leaderID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent (workspace_id, name, description, runtime_mode, runtime_config,
			runtime_id, visibility, max_concurrent_tasks, owner_id)
		VALUES ($1, 'Standup Leader', '', 'cloud', '{}'::jsonb, $2, 'workspace', 1, $3)
		RETURNING id
	`, testWorkspaceID, testRuntimeID, testUserID).Scan(&leaderID); err != nil {
		t.Fatalf("insert leader agent: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM agent WHERE id=$1`, leaderID) })

	makeSquad := func(name string, archived bool) string {
		var id string
		if err := testPool.QueryRow(ctx, `
			INSERT INTO squad (workspace_id, name, leader_id, creator_id, archived_at)
			VALUES ($1, $2, $3, $4, CASE WHEN $5 THEN now() ELSE NULL END)
			RETURNING id
		`, testWorkspaceID, name, leaderID, testUserID, archived).Scan(&id); err != nil {
			t.Fatalf("insert squad %s: %v", name, err)
		}
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM squad WHERE id=$1`, id) })
		return id
	}

	num := 900000
	makeIssue := func(squadID, status string, updatedAt time.Time) string {
		num++
		var id string
		if err := testPool.QueryRow(ctx, `
			INSERT INTO issue (workspace_id, title, status, assignee_type, assignee_id,
				creator_type, creator_id, number, updated_at)
			VALUES ($1, 'standup', $2, 'squad', $3, 'member', $4, $5, $6)
			RETURNING id
		`, testWorkspaceID, status, squadID, testUserID, num, updatedAt).Scan(&id); err != nil {
			t.Fatalf("insert issue: %v", err)
		}
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM issue WHERE id=$1`, id) })
		return id
	}

	stale := time.Now().Add(-1 * time.Hour)
	fresh := time.Now()

	activeSquad := makeSquad("Standup Active", false)
	archivedSquad := makeSquad("Standup Archived", true)

	wantPresent := makeIssue(activeSquad, "in_progress", stale)
	wrongStatus := makeIssue(activeSquad, "done", stale)
	freshIssue := makeIssue(activeSquad, "in_progress", fresh)
	commentedIssue := makeIssue(activeSquad, "in_progress", stale)
	taskedIssue := makeIssue(activeSquad, "in_progress", stale)
	archivedIssue := makeIssue(archivedSquad, "in_progress", stale)

	// A recent comment resets the activity clock even though issue.updated_at is old.
	if _, err := testPool.Exec(ctx, `
		INSERT INTO comment (workspace_id, issue_id, author_type, author_id, content, created_at)
		VALUES ($1, $2, 'member', $3, 'just now', now())
	`, testWorkspaceID, commentedIssue, testUserID); err != nil {
		t.Fatalf("insert comment: %v", err)
	}
	// An active task means the issue is already being worked.
	if _, err := testPool.Exec(ctx, `
		INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status)
		VALUES ($1, $2, $3, 'running')
	`, leaderID, testRuntimeID, taskedIssue); err != nil {
		t.Fatalf("insert task: %v", err)
	}

	cutoff := time.Now().Add(-5 * time.Minute)
	rows, err := q.ListStaleSquadIssues(ctx, db.ListStaleSquadIssuesParams{
		Cutoff: pgtype.Timestamptz{Time: cutoff, Valid: true},
		Lim:    1000,
	})
	if err != nil {
		t.Fatalf("ListStaleSquadIssues: %v", err)
	}

	got := map[string]bool{}
	for _, r := range rows {
		got[util.UUIDToString(r.ID)] = true
	}

	if !got[wantPresent] {
		t.Errorf("stale in_progress squad issue should be returned but was missing")
	}
	for reason, id := range map[string]string{
		"wrong status (done)":  wrongStatus,
		"fresh updated_at":     freshIssue,
		"recently commented":   commentedIssue,
		"has active task":      taskedIssue,
		"under archived squad": archivedIssue,
	} {
		if got[id] {
			t.Errorf("issue (%s) should be excluded but was returned", reason)
		}
	}
}
