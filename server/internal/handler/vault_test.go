package handler

import (
	"os"
	"path/filepath"
	"testing"
)

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
