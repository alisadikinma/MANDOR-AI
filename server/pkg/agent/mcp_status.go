package agent

import (
	"bufio"
	"context"
	"os/exec"
	"strings"
	"time"
)

// McpStatus asks the runtime's own agent CLI which MCP servers it currently
// considers connected, using the CLI's stored credentials. This is the
// authoritative auth view: the daemon's standalone probe connects as a separate
// client and cannot see OAuth tokens the CLI holds (e.g. a Figma server the user
// authenticated through Claude Code), so the probe false-negatives those as
// needs_auth. Returns a name->connected map; a nil/absent entry means "unknown"
// (provider has no status command, the CLI is missing, or the call failed) and
// callers MUST treat that as "no opinion", never as "disconnected".
func McpStatus(ctx context.Context, providerType, executablePath string) map[string]bool {
	switch providerType {
	case "claude":
		return claudeMcpStatus(ctx, executablePath)
	default:
		// Other CLIs either lack an `mcp list` equivalent or print a format we
		// don't parse yet — report "unknown" so the raw probe result stands.
		return nil
	}
}

func claudeMcpStatus(ctx context.Context, executablePath string) map[string]bool {
	if executablePath == "" {
		executablePath = "claude"
	}
	if _, err := exec.LookPath(executablePath); err != nil {
		return nil
	}
	// `claude mcp list` health-checks every server sequentially; give it room
	// but stay under the daemon probe handler's overall budget.
	runCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, executablePath, "mcp", "list")
	hideAgentWindow(cmd)
	// Parse whatever printed, even on a non-zero exit (one failed server makes
	// the command exit non-zero while still listing the rest).
	out, _ := cmd.Output()
	return parseClaudeMcpStatus(string(out))
}

// parseClaudeMcpStatus reads `claude mcp list` output. Each server row looks
// like `name: detail - ✔ Connected` or `name: detail - ✘ Failed to connect`.
// The name is everything before the first ": " — server names may themselves
// contain colons (e.g. `plugin:github:github`) but never a colon-space.
func parseClaudeMcpStatus(output string) map[string]bool {
	status := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		sep := strings.Index(line, ": ")
		if sep <= 0 {
			continue
		}
		failed := strings.Contains(line, "✘") || strings.Contains(line, "Failed")
		connected := !failed && (strings.Contains(line, "✔") || strings.Contains(line, "Connected"))
		if !connected && !failed {
			continue // not a server health row (no verdict)
		}
		name := strings.TrimSpace(line[:sep])
		if name == "" {
			continue
		}
		status[name] = connected
	}
	return status
}
