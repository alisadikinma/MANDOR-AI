package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGetRuntimeMcpReturnsReportedPool seeds the runtime's reported pool and
// asserts the member-facing endpoint mirrors it back.
func TestGetRuntimeMcpReturnsReportedPool(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	runtimeID := handlerTestRuntimeID(t)

	if _, err := testPool.Exec(ctx,
		`UPDATE agent_runtime SET reported_mcp_servers = $2 WHERE id = $1`,
		runtimeID, []byte(`[{"name":"github","transport":"stdio"},{"name":"figma","transport":"http"}]`),
	); err != nil {
		t.Fatalf("seed pool: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(ctx, `UPDATE agent_runtime SET reported_mcp_servers = NULL WHERE id = $1`, runtimeID)
	})

	w := httptest.NewRecorder()
	req := withURLParam(newRequest("GET", "/api/runtimes/"+runtimeID+"/mcp", nil), "runtimeId", runtimeID)
	testHandler.GetRuntimeMcp(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp RuntimeMcpResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Servers) != 2 || resp.Servers[0].Name != "figma" && resp.Servers[0].Name != "github" {
		t.Fatalf("unexpected servers: %+v", resp.Servers)
	}
	names := map[string]string{}
	for _, s := range resp.Servers {
		names[s.Name] = s.Transport
	}
	if names["github"] != "stdio" || names["figma"] != "http" {
		t.Fatalf("unexpected transports: %v", names)
	}
}

// TestGetRuntimeMcpEmptyPool: a runtime that has reported nothing returns an
// empty list, not an error or null crash.
func TestGetRuntimeMcpEmptyPool(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	runtimeID := handlerTestRuntimeID(t)
	w := httptest.NewRecorder()
	req := withURLParam(newRequest("GET", "/api/runtimes/"+runtimeID+"/mcp", nil), "runtimeId", runtimeID)
	testHandler.GetRuntimeMcp(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp RuntimeMcpResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Servers) != 0 {
		t.Fatalf("expected empty pool, got %+v", resp.Servers)
	}
}
