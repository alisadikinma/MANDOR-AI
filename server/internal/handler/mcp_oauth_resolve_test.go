package handler

import "testing"

func TestRuntimePoolServerURL(t *testing.T) {
	pool := []byte(`[{"name":"figma","transport":"http","url":"https://figma.example/mcp"},{"name":"gh","transport":"stdio"}]`)

	if got := runtimePoolServerURL(pool, "figma"); got != "https://figma.example/mcp" {
		t.Fatalf("figma url = %q, want the http endpoint", got)
	}
	if got := runtimePoolServerURL(pool, "gh"); got != "" {
		t.Fatalf("stdio server has no url, got %q", got)
	}
	if got := runtimePoolServerURL(pool, "missing"); got != "" {
		t.Fatalf("unknown server has no url, got %q", got)
	}
	if got := runtimePoolServerURL(nil, "figma"); got != "" {
		t.Fatalf("empty pool yields no url, got %q", got)
	}
}
