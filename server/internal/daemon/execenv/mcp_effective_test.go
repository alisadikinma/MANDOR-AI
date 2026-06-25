package execenv

import (
	"encoding/json"
	"testing"
)

func parseServers(t *testing.T, raw []byte) map[string]map[string]any {
	t.Helper()
	var root struct {
		McpServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("not a {mcpServers} doc: %v (%s)", err, raw)
	}
	return root.McpServers
}

func TestBuildEffectiveMcpConfig(t *testing.T) {
	machine := json.RawMessage(`{"mcpServers":{
		"github":{"command":"npx"},
		"figma":{"url":"https://figma.example/mcp"},
		"internal":{"url":"https://internal.example/mcp"}
	}}`)

	// Disable "github", inject a bearer for "figma" only.
	out := BuildEffectiveMcpConfig(machine, []string{"github"}, map[string]string{"figma": "tok-123"})
	servers := parseServers(t, out)

	if _, ok := servers["github"]; ok {
		t.Fatal("disabled server github must be removed")
	}
	if len(servers) != 2 {
		t.Fatalf("expected figma + internal, got %v", servers)
	}
	figmaHeaders, _ := servers["figma"]["headers"].(map[string]any)
	if figmaHeaders["Authorization"] != "Bearer tok-123" {
		t.Fatalf("figma should carry the bearer header, got %v", servers["figma"])
	}
	if _, ok := servers["internal"]["headers"]; ok {
		t.Fatalf("internal has no token, must not get a header: %v", servers["internal"])
	}
}

func TestBuildEffectiveMcpConfigRespectsExistingAuth(t *testing.T) {
	machine := json.RawMessage(`{"mcpServers":{"x":{"url":"https://x.example/mcp","headers":{"Authorization":"Bearer user-set"}}}}`)
	out := BuildEffectiveMcpConfig(machine, nil, map[string]string{"x": "override"})
	servers := parseServers(t, out)
	h, _ := servers["x"]["headers"].(map[string]any)
	if h["Authorization"] != "Bearer user-set" {
		t.Fatalf("must not overwrite a user-set Authorization, got %v", h)
	}
}

func TestBuildEffectiveMcpConfigEmptyMachine(t *testing.T) {
	if got := BuildEffectiveMcpConfig(nil, []string{"a"}, nil); got != nil {
		t.Fatalf("nil machine yields nil, got %s", got)
	}
}
