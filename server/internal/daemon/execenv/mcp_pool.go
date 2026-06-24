package execenv

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/multica-ai/multica/server/pkg/protocol"
)

// ResolveMachineMcpServers reads the MCP servers configured on the runtime
// host for the given runtime type and returns them as name + transport only —
// no command/url/env/tokens. This is the runtime's pool: agents on the runtime
// reuse it instead of carrying their own server definitions.
//
// Each runtime stores config differently, so this dispatches per type:
//   - codex   → ~/.codex/config.toml `[mcp_servers]` (homeDir/config.toml in tests)
//   - claude  → ~/.claude.json top-level `mcpServers` (homeDir/.claude.json)
//   - openclaw → `openclaw config get mcp.servers --json` (no static file)
//
// homeDir is the directory holding the runtime's config (the codex home, or the
// user home for claude); tests pass a temp dir. It is ignored for openclaw,
// which resolves through its own CLI. An unknown runtime type or a missing
// config yields an empty slice, never an error — a runtime with no MCP servers
// is a normal state, not a failure.
func ResolveMachineMcpServers(runtimeType, homeDir string) ([]protocol.McpServerInfo, error) {
	switch strings.ToLower(strings.TrimSpace(runtimeType)) {
	case "codex":
		dir := homeDir
		if dir == "" {
			dir = resolveSharedCodexHome()
		}
		return codexMachineMcpServers(filepath.Join(dir, "config.toml"))
	case "claude":
		dir := homeDir
		if dir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, nil // no home → no machine config to read; not an error
			}
			dir = home
		}
		return claudeMachineMcpServers(filepath.Join(dir, ".claude.json"))
	case "openclaw":
		return openclawMachineMcpServers()
	default:
		return nil, nil
	}
}

// mcpEntry is the minimal shape we classify across all three config formats:
// a `command` means stdio; any URL field means http. Both TOML and JSON decode
// into it via the struct tags.
type mcpEntry struct {
	Command   string `json:"command" toml:"command"`
	URL       string `json:"url" toml:"url"`
	HTTPURL   string `json:"httpUrl" toml:"httpUrl"`
	ServerURL string `json:"serverUrl" toml:"serverUrl"`
}

func (e mcpEntry) transport() string {
	switch {
	case strings.TrimSpace(e.Command) != "":
		return "stdio"
	case strings.TrimSpace(e.URL) != "", strings.TrimSpace(e.HTTPURL) != "", strings.TrimSpace(e.ServerURL) != "":
		return "http"
	default:
		return "stdio" // a server with neither is unusual; default to stdio rather than drop it
	}
}

// toServerInfos turns a name→entry map into a name-sorted slice so the reported
// pool is stable across runs (map iteration order is otherwise random).
func toServerInfos(entries map[string]mcpEntry) []protocol.McpServerInfo {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]protocol.McpServerInfo, 0, len(names))
	for _, name := range names {
		out = append(out, protocol.McpServerInfo{Name: name, Transport: entries[name].transport()})
	}
	return out
}

func codexMachineMcpServers(path string) ([]protocol.McpServerInfo, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg struct {
		McpServers map[string]mcpEntry `toml:"mcp_servers"`
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return toServerInfos(cfg.McpServers), nil
}

func claudeMachineMcpServers(path string) ([]protocol.McpServerInfo, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Only the top-level mcpServers is the machine-wide pool. Project-scoped
	// servers (projects.<path>.mcpServers) are per-workdir and out of scope.
	var cfg struct {
		McpServers map[string]mcpEntry `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return toServerInfos(cfg.McpServers), nil
}

func openclawMachineMcpServers() ([]protocol.McpServerInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), openclawCLITimeout)
	defer cancel()
	out, err := openclawExec(ctx, "openclaw", "config", "get", "mcp.servers", "--json")
	if err != nil {
		if isOpenclawKeyMissing(err) {
			return nil, nil
		}
		return nil, err
	}
	trimmed := strings.TrimSpace(out)
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var servers map[string]mcpEntry
	if err := json.Unmarshal([]byte(trimmed), &servers); err != nil {
		return nil, err
	}
	return toServerInfos(servers), nil
}
