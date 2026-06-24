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

// MachineMcpConfigJSON returns the runtime host's own MCP servers as a full
// `{"mcpServers":{"<name>":{...}}}` JSON document — commands, args, env, urls
// and all. This is what mcpprobe needs to actually connect, and what the
// task-time path filters by an agent's deny-list. It stays on the runtime host
// (the daemon); secrets in it never travel to the control plane.
//
// Each runtime stores config differently:
//   - codex    → ~/.codex/config.toml `[mcp_servers]` (homeDir/config.toml in tests)
//   - claude   → ~/.claude.json top-level `mcpServers` (homeDir/.claude.json)
//   - openclaw → `openclaw config get mcp.servers --json`
//
// homeDir is the config directory (codex home, or user home for claude); empty
// resolves the default. Ignored for openclaw. An unknown runtime or a missing /
// empty config returns nil (no servers), never an error.
func MachineMcpConfigJSON(runtimeType, homeDir string) (json.RawMessage, error) {
	switch strings.ToLower(strings.TrimSpace(runtimeType)) {
	case "codex":
		dir := homeDir
		if dir == "" {
			dir = resolveSharedCodexHome()
		}
		return codexMachineConfig(filepath.Join(dir, "config.toml"))
	case "claude":
		dir := homeDir
		if dir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, nil
			}
			dir = home
		}
		return claudeMachineConfig(filepath.Join(dir, ".claude.json"))
	case "openclaw":
		return openclawMachineConfig()
	default:
		return nil, nil
	}
}

// ResolveMachineMcpServers returns the runtime host's MCP servers as name +
// transport only — no secrets — for the pool the daemon reports and agents
// reuse. Derived from MachineMcpConfigJSON so there is one source of truth for
// reading each runtime's config.
func ResolveMachineMcpServers(runtimeType, homeDir string) ([]protocol.McpServerInfo, error) {
	raw, err := MachineMcpConfigJSON(runtimeType, homeDir)
	if err != nil || len(raw) == 0 {
		return nil, err
	}
	var cfg struct {
		McpServers map[string]mcpEntry `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	return toServerInfos(cfg.McpServers), nil
}

// mcpEntry is the minimal shape we classify for transport: a `command` means
// stdio; any URL field means http.
type mcpEntry struct {
	Command   string `json:"command"`
	URL       string `json:"url"`
	HTTPURL   string `json:"httpUrl"`
	ServerURL string `json:"serverUrl"`
}

func (e mcpEntry) transport() string {
	switch {
	case strings.TrimSpace(e.Command) != "":
		return "stdio"
	case strings.TrimSpace(e.URL) != "", strings.TrimSpace(e.HTTPURL) != "", strings.TrimSpace(e.ServerURL) != "":
		return "http"
	default:
		return "stdio"
	}
}

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

// wrapServers marshals a name→entry map as `{"mcpServers":{...}}`, or returns
// nil when there are no servers (so callers treat "no config" uniformly).
func wrapServers(servers map[string]json.RawMessage) (json.RawMessage, error) {
	if len(servers) == 0 {
		return nil, nil
	}
	return json.Marshal(struct {
		McpServers map[string]json.RawMessage `json:"mcpServers"`
	}{servers})
}

func codexMachineConfig(path string) (json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Codex stores servers as a TOML `[mcp_servers.<name>]` table. Decode that
	// sub-table into generic JSON-able values; the keys (command/args/env/url)
	// already match what mcpprobe and the CLIs expect.
	var cfg struct {
		McpServers map[string]map[string]any `toml:"mcp_servers"`
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	servers := make(map[string]json.RawMessage, len(cfg.McpServers))
	for name, entry := range cfg.McpServers {
		b, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		servers[name] = b
	}
	return wrapServers(servers)
}

func claudeMachineConfig(path string) (json.RawMessage, error) {
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
		McpServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return wrapServers(cfg.McpServers)
}

func openclawMachineConfig() (json.RawMessage, error) {
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
	var servers map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &servers); err != nil {
		return nil, err
	}
	return wrapServers(servers)
}
