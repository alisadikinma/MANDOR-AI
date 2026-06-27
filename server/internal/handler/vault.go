package handler

import (
	"errors"
	"os"
	"path/filepath"
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
