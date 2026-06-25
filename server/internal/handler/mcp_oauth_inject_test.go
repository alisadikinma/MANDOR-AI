package handler

import "testing"

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
