package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/multica-ai/multica/server/internal/util"
	"gopkg.in/yaml.v3"
)

// vaultRoot resolves the vault root directory for the request's workspace: the
// per-workspace `settings.vault_path` (set in workspace settings) if present,
// otherwise the operator-wide VAULT_PATH default from config. Returns "" when
// neither is configured — handlers then report disabled / 404.
func (h *Handler) vaultRoot(r *http.Request) string {
	if p := h.workspaceVaultPath(r); p != "" {
		return p
	}
	return strings.TrimSpace(h.cfg.VaultPath)
}

// workspaceVaultPath reads settings.vault_path from the request's workspace
// row. Any failure (no workspace id in context — e.g. direct unit-test calls,
// bad UUID, DB error, missing key) degrades to "" so vaultRoot falls back to
// the config default.
func (h *Handler) workspaceVaultPath(r *http.Request) string {
	id := workspaceIDFromURL(r, "id")
	if id == "" {
		return ""
	}
	uid, err := util.ParseUUID(id)
	if err != nil {
		return ""
	}
	ws, err := h.Queries.GetWorkspace(r.Context(), uid)
	if err != nil {
		return ""
	}
	var s struct {
		VaultPath string `json:"vault_path"`
	}
	if len(ws.Settings) > 0 {
		_ = json.Unmarshal(ws.Settings, &s)
	}
	return strings.TrimSpace(s.VaultPath)
}

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
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": h.vaultRoot(r) != ""})
}

// GetVaultTree returns the vault's folder/.md tree. Dotfiles (.obsidian, .git,
// .trash, …) and non-.md files are excluded; dirs sort before files, each group
// alphabetically (case-insensitive).
func (h *Handler) GetVaultTree(w http.ResponseWriter, r *http.Request) {
	root := h.vaultRoot(r)
	if root == "" {
		writeError(w, http.StatusNotFound, "vault not configured")
		return
	}
	children, err := readVaultDir(root, "")
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
	root := h.vaultRoot(r)
	if root == "" {
		writeError(w, http.StatusNotFound, "vault not configured")
		return
	}
	rel := r.URL.Query().Get("path")
	if rel == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	abs, err := safeVaultPath(root, rel)
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

// GetVaultFile serves a vault file's raw bytes (for image / `![[embed]]`
// rendering). Content-Type is set by http.ServeContent from the file extension.
// The `path` query param is confined to the vault before any read.
func (h *Handler) GetVaultFile(w http.ResponseWriter, r *http.Request) {
	root := h.vaultRoot(r)
	if root == "" {
		writeError(w, http.StatusNotFound, "vault not configured")
		return
	}
	rel := r.URL.Query().Get("path")
	if rel == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	abs, err := safeVaultPath(root, rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	f, err := os.Open(abs)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || fi.IsDir() {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	http.ServeContent(w, r, filepath.Base(abs), fi.ModTime(), f)
}

type vaultSearchResult struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Snippet string `json:"snippet"`
}

// vaultSearchLimit caps results so a broad query over a large vault can't blow
// up the response. A personal vault rarely exceeds this; bump or add an index
// only if it becomes a felt limit.
const vaultSearchLimit = 50

// SearchVault matches `q` (case-insensitive) against `.md` filenames and
// contents, returning up to vaultSearchLimit results with a context snippet for
// content matches. An empty `q` returns an empty list — never the whole vault.
func (h *Handler) SearchVault(w http.ResponseWriter, r *http.Request) {
	root := h.vaultRoot(r)
	if root == "" {
		writeError(w, http.StatusNotFound, "vault not configured")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	results := []vaultSearchResult{}
	if q == "" {
		writeJSON(w, http.StatusOK, results)
		return
	}
	lq := strings.ToLower(q)

	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries, keep walking
		}
		if d.IsDir() {
			if p != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir // skip .obsidian/.git/.trash subtrees
			}
			return nil
		}
		if len(results) >= vaultSearchLimit {
			return filepath.SkipAll
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") || !strings.EqualFold(filepath.Ext(name), ".md") {
			return nil
		}

		nameMatch := strings.Contains(strings.ToLower(name), lq)
		snippet := ""
		contentMatch := false
		if content, readErr := os.ReadFile(p); readErr == nil {
			if idx := strings.Index(strings.ToLower(string(content)), lq); idx != -1 {
				contentMatch = true
				snippet = vaultSnippet(string(content), idx, len(q))
			}
		}
		if nameMatch || contentMatch {
			rel, _ := filepath.Rel(root, p)
			results = append(results, vaultSearchResult{Name: name, Path: filepath.ToSlash(rel), Snippet: snippet})
		}
		return nil
	})

	writeJSON(w, http.StatusOK, results)
}

// vaultSnippet returns a single-line, whitespace-collapsed window of content
// around the match at byte offset idx, ellipsised when clipped.
func vaultSnippet(content string, idx, matchLen int) string {
	const pad = 40
	start := idx - pad
	if start < 0 {
		start = 0
	}
	end := idx + matchLen + pad
	if end > len(content) {
		end = len(content)
	}
	// ponytail: byte-sliced window may cut a multi-byte rune at the edges; the
	// JSON encoder substitutes U+FFFD so it's safe, just occasionally a stray
	// glyph. Move to rune-aware windowing only if snippets look ugly in the UI.
	window := strings.Join(strings.Fields(content[start:end]), " ")
	if start > 0 {
		window = "…" + window
	}
	if end < len(content) {
		window += "…"
	}
	return window
}
