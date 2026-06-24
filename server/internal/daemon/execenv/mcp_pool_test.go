package execenv

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/multica-ai/multica/server/pkg/protocol"
)

// sortedNames is a readability helper for asserting the (name, transport) set
// the reader yields, independent of map iteration order.
func names(servers []protocol.McpServerInfo) map[string]string {
	out := make(map[string]string, len(servers))
	for _, s := range servers {
		out[s.Name] = s.Transport
	}
	return out
}

func TestResolveMachineMcpServersCodex(t *testing.T) {
	dir := t.TempDir()
	cfg := `
model = "gpt-5"

[mcp_servers.github]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-github"]

[mcp_servers.github.env]
GITHUB_TOKEN = "secret-should-not-leak"

[mcp_servers.figma]
url = "https://figma.example/mcp"
`
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveMachineMcpServers("codex", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{"github": "stdio", "figma": "http"}
	if g := names(got); !reflect.DeepEqual(g, want) {
		t.Fatalf("got %v, want %v", g, want)
	}
}

func TestResolveMachineMcpServersClaude(t *testing.T) {
	dir := t.TempDir()
	cfg := `{
	  "mcpServers": {
	    "obsidian": { "command": "npx", "args": ["-y", "obsidian-mcp"] },
	    "firecrawl": { "url": "https://firecrawl.example/mcp" }
	  },
	  "projects": { "/some/path": { "mcpServers": { "ignored": { "command": "x" } } } }
	}`
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveMachineMcpServers("claude", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Project-scoped servers are out of scope for the runtime pool — only the
	// global mcpServers map is reported.
	want := map[string]string{"obsidian": "stdio", "firecrawl": "http"}
	if g := names(got); !reflect.DeepEqual(g, want) {
		t.Fatalf("got %v, want %v", g, want)
	}
}

func TestResolveMachineMcpServersOpenclaw(t *testing.T) {
	// OpenClaw has no static file — its servers come from `openclaw config get
	// mcp.servers --json`. Stub the CLI hook the way the other openclaw tests do.
	orig := openclawExec
	t.Cleanup(func() { openclawExec = orig })
	openclawExec = func(_ context.Context, _ string, args ...string) (string, error) {
		return `{"github":{"command":"npx"},"stitch":{"transport":"streamable-http","url":"https://stitch.example/mcp"}}`, nil
	}

	got, err := ResolveMachineMcpServers("openclaw", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{"github": "stdio", "stitch": "http"}
	if g := names(got); !reflect.DeepEqual(g, want) {
		t.Fatalf("got %v, want %v", g, want)
	}
}

func TestResolveMachineMcpServersMissingFileIsEmpty(t *testing.T) {
	got, err := ResolveMachineMcpServers("codex", t.TempDir())
	if err != nil {
		t.Fatalf("missing config must not error, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no servers, got %v", got)
	}
}

func TestResolveMachineMcpServersUnknownRuntimeIsEmpty(t *testing.T) {
	got, err := ResolveMachineMcpServers("hermes", t.TempDir())
	if err != nil {
		t.Fatalf("unknown runtime must not error, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no servers for unknown runtime, got %v", got)
	}
}
