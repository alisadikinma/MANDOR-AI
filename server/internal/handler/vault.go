package handler

import (
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// safeVaultPath resolves a caller-supplied relative path against the configured
// vault root and guarantees the result stays inside that root. It is the single
// trust-boundary check for every vault endpoint — no handler touches the
// filesystem without routing its `path` query param through here first.
//
// It rejects:
//   - an empty/unconfigured root,
//   - absolute paths,
//   - lexical `../` escapes (via filepath.Join's Clean),
//   - symlink escapes (a symlink inside the vault that resolves outside it).
//
// On success it returns the absolute, joined path (suitable for os.ReadFile /
// http.ServeFile). The error message is intentionally generic so it never leaks
// filesystem layout to the client.
func safeVaultPath(root, rel string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("vault path not configured")
	}
	if filepath.IsAbs(rel) {
		return "", errors.New("absolute paths are not allowed")
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	// Resolve the root's own symlinks so the prefix comparison below is against
	// the real on-disk path (e.g. /var -> /private/var on macOS).
	if resolved, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = resolved
	}

	joined := filepath.Join(absRoot, rel)

	// When the target exists, resolve its symlinks and re-check containment so a
	// symlink inside the vault cannot point at a file outside it.
	check := joined
	if resolved, err := filepath.EvalSymlinks(joined); err == nil {
		check = resolved
	}
	if check != absRoot && !strings.HasPrefix(check, absRoot+string(os.PathSeparator)) {
		return "", errors.New("path escapes the vault root")
	}
	return joined, nil
}

// vaultTreeNode is one entry in the folder tree. Path is relative to the vault
// root and always "/"-separated (so the frontend and the `path` query param
// agree regardless of the server OS). Children is present only for dirs.
type vaultTreeNode struct {
	Name     string          `json:"name"`
	Path     string          `json:"path"`
	Type     string          `json:"type"` // "dir" | "file"
	Children []vaultTreeNode `json:"children,omitempty"`
}

// GetVaultStatus reports whether a vault is configured, so the client can hide
// the Vault nav entry when VAULT_PATH is unset. It never touches the FS.
func (h *Handler) GetVaultStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": h.cfg.VaultPath != ""})
}

// GetVaultTree returns the vault's folder/.md tree. Dotfiles (.obsidian, .git,
// .trash, …) and non-.md files are excluded; dirs sort before files, each group
// alphabetically (case-insensitive).
func (h *Handler) GetVaultTree(w http.ResponseWriter, r *http.Request) {
	if h.cfg.VaultPath == "" {
		writeError(w, http.StatusNotFound, "vault not configured")
		return
	}
	children, err := readVaultDir(h.cfg.VaultPath, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read vault")
		return
	}
	writeJSON(w, http.StatusOK, children)
}

// readVaultDir walks absDir, returning the markdown tree rooted there. relDir is
// the "/"-separated path of absDir relative to the vault root ("" at the root).
func readVaultDir(absDir, relDir string) ([]vaultTreeNode, error) {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, err
	}
	var dirs, files []vaultTreeNode
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue // hide dotfiles/dirs (.obsidian, .git, .trash, …)
		}
		rel := path.Join(relDir, name)
		if e.IsDir() {
			grandchildren, err := readVaultDir(filepath.Join(absDir, name), rel)
			if err != nil {
				return nil, err
			}
			dirs = append(dirs, vaultTreeNode{Name: name, Path: rel, Type: "dir", Children: grandchildren})
			continue
		}
		if strings.EqualFold(filepath.Ext(name), ".md") {
			files = append(files, vaultTreeNode{Name: name, Path: rel, Type: "file"})
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name) })
	sort.Slice(files, func(i, j int) bool { return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name) })
	return append(dirs, files...), nil
}
