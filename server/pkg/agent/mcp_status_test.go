package agent

import "testing"

func TestParseClaudeMcpStatus(t *testing.T) {
	out := `Checking MCP server health…

claude.ai higgsfield: https://mcp.higgsfield.ai/mcp - ✔ Connected
plugin:github:github: https://api.githubcopilot.com/mcp/ (HTTP) - ✔ Connected
figma: https://mcp.figma.com/mcp (HTTP) - ✔ Connected
github: https://api.githubcopilot.com/mcp/ (HTTP) - ✘ Failed to connect
`
	got := parseClaudeMcpStatus(out)

	want := map[string]bool{
		"claude.ai higgsfield": true,
		"plugin:github:github": true, // name keeps its internal colons
		"figma":                true,
		"github":               false,
	}
	if len(got) != len(want) {
		t.Fatalf("parsed %d rows, want %d: %#v", len(got), len(want), got)
	}
	for name, ok := range want {
		if got[name] != ok {
			t.Errorf("%q: got connected=%v, want %v", name, got[name], ok)
		}
	}
}
