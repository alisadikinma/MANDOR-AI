package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// standupSummary is the literal trigger handed to a re-woken squad leader. It
// tells the leader why it was woken with no new comment, and points it at the
// squad instructions for the actual policy (chase / bounce / escalate). Keep it
// short — the leader already has full context via the issue and its briefing.
const standupSummary = "Scheduled standup check: this issue has gone silent with no recent activity. " +
	"Review the delegated work and act per the squad instructions — chase a member who has gone quiet, " +
	"bounce a handoff that does not meet the Definition of Done, or escalate to the reporter if blocked."

// squadStandupConfig tunes the standup sweep. Setting Interval <= 0 disables it.
type squadStandupConfig struct {
	Interval     time.Duration // how often to sweep
	StaleAfter   time.Duration // silence before a squad issue is re-woken
	StartupDelay time.Duration // stagger the first sweep after boot
	BatchLimit   int32         // max issues re-woken per sweep
}

func defaultSquadStandupConfig() squadStandupConfig {
	return squadStandupConfig{
		Interval:     30 * time.Minute,
		StaleAfter:   2 * time.Hour,
		StartupDelay: 1 * time.Minute,
		BatchLimit:   50,
	}
}

func envSquadStandupConfig() squadStandupConfig {
	cfg := defaultSquadStandupConfig()
	cfg.Interval = envDurationOrZero("SQUAD_STANDUP_INTERVAL", cfg.Interval)
	cfg.StaleAfter = envDurationPositive("SQUAD_STANDUP_STALE_AFTER", cfg.StaleAfter)
	cfg.StartupDelay = envDurationNonNegative("SQUAD_STANDUP_STARTUP_DELAY", cfg.StartupDelay)
	if v, ok := envInt64Positive("SQUAD_STANDUP_BATCH_LIMIT"); ok {
		cfg.BatchLimit = int32(v)
	}
	return cfg
}

// runSquadStandupScheduler periodically re-wakes squad leaders for issues that
// have gone silent. The squad model only re-triggers the leader when a member
// posts a comment, so a member that simply stops working freezes the whole
// issue — there is no daily-standup heartbeat to notice. This sweep is that
// heartbeat: it finds mid-flight squad issues with no activity since the cutoff
// and no in-flight task, and re-enqueues the leader so it can chase, bounce, or
// escalate. Disable with SQUAD_STANDUP_INTERVAL=0.
func runSquadStandupScheduler(ctx context.Context, queries *db.Queries, taskSvc *service.TaskService, cfg squadStandupConfig) {
	if cfg.Interval <= 0 {
		slog.Info("squad standup scheduler: disabled (interval <= 0)")
		return
	}

	slog.Info("squad standup scheduler: starting",
		"interval", cfg.Interval.String(),
		"stale_after", cfg.StaleAfter.String(),
		"batch_limit", cfg.BatchLimit,
	)

	if cfg.StartupDelay > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(cfg.StartupDelay):
		}
	}

	tickSquadStandup(ctx, queries, taskSvc, cfg)

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tickSquadStandup(ctx, queries, taskSvc, cfg)
		}
	}
}

func tickSquadStandup(ctx context.Context, queries *db.Queries, taskSvc *service.TaskService, cfg squadStandupConfig) {
	cutoff := time.Now().Add(-cfg.StaleAfter)
	issues, err := queries.ListStaleSquadIssues(ctx, db.ListStaleSquadIssuesParams{
		Cutoff: pgtype.Timestamptz{Time: cutoff, Valid: true},
		Lim:    cfg.BatchLimit,
	})
	if err != nil {
		slog.Warn("squad standup: failed to query stale issues", "error", err)
		return
	}
	if len(issues) == 0 {
		return
	}

	slog.Info("squad standup: re-waking leaders for stale squad issues", "count", len(issues))
	for _, issue := range issues {
		// The issue is assigned to a squad; assignee_id is the squad id. Scope the
		// lookup by workspace to match every other squad-load call site, even
		// though the squad UUID is globally unique.
		squad, err := queries.GetSquadByAssignee(ctx, db.GetSquadByAssigneeParams{
			ID:          issue.AssigneeID,
			WorkspaceID: issue.WorkspaceID,
		})
		if err != nil {
			slog.Warn("squad standup: load squad failed", "issue_id", util.UUIDToString(issue.ID), "error", err)
			continue
		}
		if !squad.LeaderID.Valid {
			continue
		}
		// Close the TOCTOU window between the ListStaleSquadIssues NOT EXISTS guard
		// and the enqueue: the leader may have been woken by another path in
		// between, and two sweeps must not stack duplicate standup tasks. Mirrors
		// the comment-trigger dedup in handler/squad.go.
		if pending, err := queries.HasPendingTaskForIssueAndAgent(ctx, db.HasPendingTaskForIssueAndAgentParams{
			IssueID: issue.ID,
			AgentID: squad.LeaderID,
		}); err != nil {
			slog.Warn("squad standup: pending-task check failed", "issue_id", util.UUIDToString(issue.ID), "error", err)
			continue
		} else if pending {
			continue
		}
		if _, err := taskSvc.EnqueueStandupForSquadLeader(ctx, issue, squad.LeaderID, standupSummary); err != nil {
			// Benign: leader agent archived / no runtime — nothing to wake.
			slog.Debug("squad standup: enqueue leader failed", "issue_id", util.UUIDToString(issue.ID), "error", err)
		}
	}
}
