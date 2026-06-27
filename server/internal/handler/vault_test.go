package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	writeVaultFile(t, root, "image.png", "PNG")          // non-md → excluded
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
