package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeVaultFile creates root/rel (with parent dirs) holding body.
func writeVaultFile(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGetVaultStatus(t *testing.T) {
	t.Run("enabled when VaultPath set", func(t *testing.T) {
		h := newTestHandler(Config{VaultPath: t.TempDir()})
		w := httptest.NewRecorder()
		h.GetVaultStatus(w, httptest.NewRequest(http.MethodGet, "/vault/status", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var got struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if !got.Enabled {
			t.Fatal("enabled = false, want true")
		}
	})

	t.Run("disabled when VaultPath empty (no FS access)", func(t *testing.T) {
		h := newTestHandler(Config{VaultPath: ""})
		w := httptest.NewRecorder()
		h.GetVaultStatus(w, httptest.NewRequest(http.MethodGet, "/vault/status", nil))
		var got struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Enabled {
			t.Fatal("enabled = true, want false")
		}
	})
}

func TestGetVaultTree(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "zeta.md", "z")
	writeVaultFile(t, root, "alpha.md", "a")
	writeVaultFile(t, root, "folder/inner.md", "i")
	writeVaultFile(t, root, "folder/sub/deep.md", "d")
	writeVaultFile(t, root, "image.png", "PNG")            // non-md → excluded
	writeVaultFile(t, root, ".obsidian/config.json", "{}") // dotdir → excluded

	h := newTestHandler(Config{VaultPath: root})
	w := httptest.NewRecorder()
	h.GetVaultTree(w, httptest.NewRequest(http.MethodGet, "/vault/tree", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var tree []vaultTreeNode
	if err := json.Unmarshal(w.Body.Bytes(), &tree); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}

	// Top level: dir "folder" first (dirs before files), then alpha.md, zeta.md.
	if len(tree) != 3 {
		t.Fatalf("top-level nodes = %d, want 3 (folder, alpha.md, zeta.md); got %+v", len(tree), tree)
	}
	if tree[0].Type != "dir" || tree[0].Name != "folder" {
		t.Fatalf("tree[0] = %+v, want dir folder", tree[0])
	}
	if tree[1].Name != "alpha.md" || tree[2].Name != "zeta.md" {
		t.Fatalf("files not sorted: %q, %q", tree[1].Name, tree[2].Name)
	}
	// No png and no .obsidian anywhere at top level.
	for _, n := range tree {
		if n.Name == "image.png" || n.Name == ".obsidian" {
			t.Fatalf("excluded entry leaked: %q", n.Name)
		}
	}
	// Nested structure: folder has inner.md + sub/deep.md, paths are "/"-joined.
	folder := tree[0]
	if folder.Path != "folder" {
		t.Fatalf("folder.Path = %q, want %q", folder.Path, "folder")
	}
	var sawSubDeep bool
	for _, c := range folder.Children {
		if c.Type == "dir" && c.Name == "sub" {
			for _, gc := range c.Children {
				if gc.Name == "deep.md" && gc.Path == "folder/sub/deep.md" {
					sawSubDeep = true
				}
			}
		}
	}
	if !sawSubDeep {
		t.Fatalf("expected folder/sub/deep.md in tree; got %+v", folder.Children)
	}

	t.Run("404 when VaultPath empty", func(t *testing.T) {
		hh := newTestHandler(Config{VaultPath: ""})
		ww := httptest.NewRecorder()
		hh.GetVaultTree(ww, httptest.NewRequest(http.MethodGet, "/vault/tree", nil))
		if ww.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", ww.Code)
		}
	})
}

func getVaultNote(t *testing.T, h *Handler, relPath string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/vault/note?path="+relPath, nil)
	h.GetVaultNote(w, req)
	return w
}

func TestGetVaultNote(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "with-fm.md", "---\ntitle: Hello\ntags:\n  - alpha\n  - beta\n---\n\n# Body\n\nSome **content**.\n")
	writeVaultFile(t, root, "plain.md", "# No frontmatter\n\nJust body.")
	h := newTestHandler(Config{VaultPath: root})

	t.Run("splits frontmatter and body", func(t *testing.T) {
		w := getVaultNote(t, h, "with-fm.md")
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
		}
		var got struct {
			Path        string         `json:"path"`
			Frontmatter map[string]any `json:"frontmatter"`
			Body        string         `json:"body"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Path != "with-fm.md" {
			t.Fatalf("path = %q", got.Path)
		}
		if got.Frontmatter["title"] != "Hello" {
			t.Fatalf("title = %v, want Hello", got.Frontmatter["title"])
		}
		tags, ok := got.Frontmatter["tags"].([]any)
		if !ok || len(tags) != 2 || tags[0] != "alpha" || tags[1] != "beta" {
			t.Fatalf("tags = %v, want [alpha beta]", got.Frontmatter["tags"])
		}
		if !strings.Contains(got.Body, "# Body") || strings.Contains(got.Body, "title: Hello") {
			t.Fatalf("body not split correctly: %q", got.Body)
		}
	})

	t.Run("note without frontmatter returns empty map + full body", func(t *testing.T) {
		w := getVaultNote(t, h, "plain.md")
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var got struct {
			Frontmatter map[string]any `json:"frontmatter"`
			Body        string         `json:"body"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Frontmatter) != 0 {
			t.Fatalf("frontmatter = %v, want empty", got.Frontmatter)
		}
		if got.Body != "# No frontmatter\n\nJust body." {
			t.Fatalf("body = %q", got.Body)
		}
	})

	t.Run("traversal path is rejected with 400 before any read", func(t *testing.T) {
		w := getVaultNote(t, h, "../../etc/passwd")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("missing file returns 404", func(t *testing.T) {
		w := getVaultNote(t, h, "does-not-exist.md")
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("empty path returns 400", func(t *testing.T) {
		w := getVaultNote(t, h, "")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}

func TestGetVaultFile(t *testing.T) {
	root := t.TempDir()
	pngBytes := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x01, 0x02}
	if err := os.WriteFile(filepath.Join(root, "img.png"), pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	h := newTestHandler(Config{VaultPath: root})

	serveFile := func(relPath string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.GetVaultFile(w, httptest.NewRequest(http.MethodGet, "/vault/file?path="+relPath, nil))
		return w
	}

	t.Run("serves binary with correct content-type", func(t *testing.T) {
		w := serveFile("img.png")
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/png") {
			t.Fatalf("content-type = %q, want image/png", ct)
		}
		if !strings.EqualFold(w.Body.String(), string(pngBytes)) {
			t.Fatalf("body bytes mismatch")
		}
	})

	t.Run("traversal rejected with 400", func(t *testing.T) {
		if w := serveFile("../../etc/passwd"); w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("missing file returns 404", func(t *testing.T) {
		if w := serveFile("nope.png"); w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})
}

func TestSearchVault(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "needle-in-name.md", "nothing relevant here")
	writeVaultFile(t, root, "other.md", "this body contains the needle word")
	writeVaultFile(t, root, "unrelated.md", "completely different")
	h := newTestHandler(Config{VaultPath: root})

	search := func(q string) []map[string]any {
		w := httptest.NewRecorder()
		h.SearchVault(w, httptest.NewRequest(http.MethodGet, "/vault/search?q="+q, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var got []map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
		}
		return got
	}

	t.Run("matches by filename and by content", func(t *testing.T) {
		got := search("needle")
		paths := map[string]bool{}
		for _, r := range got {
			paths[r["path"].(string)] = true
		}
		if !paths["needle-in-name.md"] {
			t.Fatal("expected filename match needle-in-name.md")
		}
		if !paths["other.md"] {
			t.Fatal("expected content match other.md")
		}
		if paths["unrelated.md"] {
			t.Fatal("unrelated.md should not match")
		}
	})

	t.Run("content match carries a snippet", func(t *testing.T) {
		for _, r := range search("needle") {
			if r["path"] == "other.md" {
				if s, _ := r["snippet"].(string); !strings.Contains(strings.ToLower(s), "needle") {
					t.Fatalf("snippet missing match context: %q", s)
				}
			}
		}
	})

	t.Run("empty q returns empty list (not whole vault)", func(t *testing.T) {
		if got := search(""); len(got) != 0 {
			t.Fatalf("empty q returned %d results, want 0", len(got))
		}
	})

	t.Run("results are capped", func(t *testing.T) {
		capRoot := t.TempDir()
		for i := 0; i < 60; i++ {
			writeVaultFile(t, capRoot, "n"+string(rune('a'+i%26))+string(rune('0'+i/26))+".md", "shared needle token")
		}
		hh := newTestHandler(Config{VaultPath: capRoot})
		w := httptest.NewRecorder()
		hh.SearchVault(w, httptest.NewRequest(http.MethodGet, "/vault/search?q=needle", nil))
		var got []map[string]any
		json.Unmarshal(w.Body.Bytes(), &got)
		if len(got) > 50 {
			t.Fatalf("results = %d, want <= 50 (cap)", len(got))
		}
	})
}

func TestSafeVaultPath(t *testing.T) {
	root := t.TempDir()
	// A real subdir + file so the happy path resolves an existing target.
	sub := filepath.Join(root, "notes")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(sub, "a.md")
	if err := os.WriteFile(note, []byte("# hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Symlink inside the vault pointing OUTSIDE it — the classic escape.
	outside := t.TempDir()
	escapeTarget := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(escapeTarget, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}

	t.Run("valid subpath resolves inside root", func(t *testing.T) {
		got, err := safeVaultPath(root, "notes/a.md")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// The guard resolves the root's own symlinks (e.g. /var -> /private/var
		// on macOS), so compare against the symlink-resolved note path.
		want, _ := filepath.EvalSymlinks(note)
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("dot-dot escape returns error", func(t *testing.T) {
		if _, err := safeVaultPath(root, "../etc/passwd"); err == nil {
			t.Fatal("expected error for ../ escape, got nil")
		}
	})

	t.Run("absolute path outside root returns error", func(t *testing.T) {
		if _, err := safeVaultPath(root, "/etc/passwd"); err == nil {
			t.Fatal("expected error for absolute path, got nil")
		}
	})

	t.Run("symlink escape returns error", func(t *testing.T) {
		if _, err := safeVaultPath(root, "escape/secret.txt"); err == nil {
			t.Fatal("expected error for symlink escape, got nil")
		}
	})

	t.Run("empty root returns error", func(t *testing.T) {
		if _, err := safeVaultPath("", "notes/a.md"); err == nil {
			t.Fatal("expected error for empty root, got nil")
		}
	})
}

func TestGetVaultGraph(t *testing.T) {
	root := t.TempDir()
	// a → b (by basename), a → folder/c (by path), b → a (alias), self-link ignored.
	writeVaultFile(t, root, "a.md", "see [[b]] and [[folder/c|the c note]] and [[a]]")
	writeVaultFile(t, root, "b.md", "back to [[a#heading]]")
	writeVaultFile(t, root, "folder/c.md", "no links here")
	writeVaultFile(t, root, "image.png", "PNG")        // non-md → no node
	writeVaultFile(t, root, ".obsidian/x.md", "[[a]]") // dotdir → skipped

	h := newTestHandler(Config{VaultPath: root})
	w := httptest.NewRecorder()
	h.GetVaultGraph(w, httptest.NewRequest(http.MethodGet, "/vault/graph", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var graph vaultGraphResponse
	if err := json.Unmarshal(w.Body.Bytes(), &graph); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}

	if len(graph.Nodes) != 3 {
		t.Fatalf("nodes = %d, want 3 (a, b, folder/c); got %+v", len(graph.Nodes), graph.Nodes)
	}
	titles := map[string]string{}
	for _, n := range graph.Nodes {
		titles[n.ID] = n.Title
	}
	if titles["folder/c.md"] != "c" {
		t.Fatalf("folder/c.md title = %q, want c", titles["folder/c.md"])
	}

	want := map[string]bool{
		"a.md\x00b.md":        true,
		"a.md\x00folder/c.md": true,
		"b.md\x00a.md":        true,
	}
	got := map[string]bool{}
	for _, l := range graph.Links {
		got[l.Source+"\x00"+l.Target] = true
	}
	if len(got) != len(want) {
		t.Fatalf("links = %d, want %d; got %+v", len(got), len(want), graph.Links)
	}
	for edge := range want {
		if !got[edge] {
			t.Fatalf("missing edge %q in %+v", strings.ReplaceAll(edge, "\x00", "→"), graph.Links)
		}
	}
	// Self-link [[a]] inside a.md must not produce an a→a edge.
	if got["a.md\x00a.md"] {
		t.Fatal("self-link a→a should be dropped")
	}
}
