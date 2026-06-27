package handler

import (
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
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

type vaultNoteResponse struct {
	Path        string         `json:"path"`
	Frontmatter map[string]any `json:"frontmatter"`
	Body        string         `json:"body"`
}

// GetVaultNote returns a single note split into its parsed YAML frontmatter and
// the remaining markdown body. The `path` query param is confined to the vault
// via safeVaultPath before any read.
func (h *Handler) GetVaultNote(w http.ResponseWriter, r *http.Request) {
	if h.cfg.VaultPath == "" {
		writeError(w, http.StatusNotFound, "vault not configured")
		return
	}
	rel := r.URL.Query().Get("path")
	if rel == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	abs, err := safeVaultPath(h.cfg.VaultPath, rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		writeError(w, http.StatusNotFound, "note not found")
		return
	}
	fm, body := splitFrontmatter(data)
	writeJSON(w, http.StatusOK, vaultNoteResponse{Path: rel, Frontmatter: fm, Body: body})
}

// splitFrontmatter separates a leading `---`-fenced YAML block from the markdown
// body. A note with no frontmatter (or malformed/unparseable frontmatter)
// returns an empty map and the original content as the body — the viewer must
// always render something.
func splitFrontmatter(data []byte) (map[string]any, string) {
	fm := map[string]any{}
	s := string(data)

	rest, ok := strings.CutPrefix(s, "---\n")
	if !ok {
		rest, ok = strings.CutPrefix(s, "---\r\n")
	}
	if !ok {
		return fm, s // no opening fence → all body
	}

	// Closing fence: a line containing only "---". Cover both the mid-file case
	// (newline after) and the EOF case (no trailing newline).
	yamlPart, body, found := "", "", false
	for _, sep := range []string{"\n---\n", "\n---\r\n"} {
		if idx := strings.Index(rest, sep); idx != -1 {
			yamlPart, body, found = rest[:idx], rest[idx+len(sep):], true
			break
		}
	}
	if !found {
		if trimmed, ok := strings.CutSuffix(rest, "\n---"); ok {
			yamlPart, body, found = trimmed, "", true
		}
	}
	if !found {
		return fm, s // opening fence but no closing one → treat as plain body
	}

	if err := yaml.Unmarshal([]byte(yamlPart), &fm); err != nil || fm == nil {
		// Non-map or invalid YAML — degrade to no frontmatter rather than error.
		return map[string]any{}, body
	}
	return fm, body
}
