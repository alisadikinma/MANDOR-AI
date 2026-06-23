package handler

import (
	"encoding/json"
	"testing"
)

func TestApplyBearerHeaders(t *testing.T) {
	cfg := json.RawMessage(`{"mcpServers":{
		"figma":{"url":"https://mcp.figma.com/mcp"},
		"slash":{"httpUrl":"https://api.example/mcp/"},
		"manual":{"url":"https://manual.example/mcp","headers":{"Authorization":"Bearer keepme"}},
		"stdio":{"command":"npx"},
		"untouched":{"url":"https://other.example/mcp"}
	}}`)
	bearer := map[string]string{
		"https://mcp.figma.com/mcp": "FIGTOKEN",
		"https://api.example/mcp":   "SLASHTOKEN", // stored without trailing slash
		"https://manual.example/mcp": "SHOULD_NOT_OVERRIDE",
	}

	out := applyBearerHeaders(cfg, bearer)
	var parsed struct {
		McpServers map[string]struct {
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}

	if got := parsed.McpServers["figma"].Headers["Authorization"]; got != "Bearer FIGTOKEN" {
		t.Errorf("figma auth = %q", got)
	}
	// Trailing-slash mismatch between config and stored resource still matches.
	if got := parsed.McpServers["slash"].Headers["Authorization"]; got != "Bearer SLASHTOKEN" {
		t.Errorf("slash auth = %q", got)
	}
	// Explicit user Authorization is preserved, never overridden.
	if got := parsed.McpServers["manual"].Headers["Authorization"]; got != "Bearer keepme" {
		t.Errorf("manual auth overridden: %q", got)
	}
	// stdio + servers with no matching token get no Authorization.
	if _, ok := parsed.McpServers["stdio"].Headers["Authorization"]; ok {
		t.Error("stdio should not get an Authorization header")
	}
	if _, ok := parsed.McpServers["untouched"].Headers["Authorization"]; ok {
		t.Error("server with no token should not get an Authorization header")
	}
}

func TestApplyBearerHeadersNoChange(t *testing.T) {
	cfg := json.RawMessage(`{"mcpServers":{"x":{"url":"https://x/mcp"}}}`)
	// No bearer entries → identical bytes back.
	if got := applyBearerHeaders(cfg, nil); string(got) != string(cfg) {
		t.Errorf("expected unchanged config, got %s", got)
	}
	// Malformed config → returned unchanged, never panics.
	bad := json.RawMessage(`not json`)
	if got := applyBearerHeaders(bad, map[string]string{"https://x/mcp": "t"}); string(got) != string(bad) {
		t.Errorf("malformed config should be unchanged")
	}
}

func TestEntryURL(t *testing.T) {
	mk := func(s string) map[string]json.RawMessage {
		var m map[string]json.RawMessage
		_ = json.Unmarshal([]byte(s), &m)
		return m
	}
	if got := entryURL(mk(`{"url":"https://a/mcp"}`)); got != "https://a/mcp" {
		t.Errorf("url = %q", got)
	}
	if got := entryURL(mk(`{"httpUrl":"  https://b/mcp  "}`)); got != "https://b/mcp" {
		t.Errorf("httpUrl trim = %q", got)
	}
	if got := entryURL(mk(`{"serverUrl":"https://c/sse"}`)); got != "https://c/sse" {
		t.Errorf("serverUrl = %q", got)
	}
	if got := entryURL(mk(`{"command":"npx"}`)); got != "" {
		t.Errorf("stdio = %q", got)
	}
}

func TestNormalizeResource(t *testing.T) {
	for in, want := range map[string]string{
		"https://x/mcp/":   "https://x/mcp",
		"  https://x/mcp ": "https://x/mcp",
		"https://x/mcp":    "https://x/mcp",
	} {
		if got := normalizeResource(in); got != want {
			t.Errorf("normalizeResource(%q) = %q, want %q", in, got, want)
		}
	}
}
