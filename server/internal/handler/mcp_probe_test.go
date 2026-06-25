package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/pkg/protocol"
)

func TestMcpProbeStoreLifecycle(t *testing.T) {
	s := NewInMemoryMcpProbeStore()
	headers := map[string]string{"figma": "secret-bearer"}
	req := s.Create("rt-1", "ws-1", headers)

	if req.Status != McpProbePending {
		t.Fatalf("new request status = %q, want pending", req.Status)
	}
	if !s.HasPending("rt-1") {
		t.Fatal("HasPending(rt-1) = false, want true")
	}
	if s.HasPending("rt-other") {
		t.Fatal("HasPending(rt-other) = true, want false")
	}

	popped := s.PopPending("rt-1")
	if popped == nil || popped.ID != req.ID {
		t.Fatalf("PopPending returned %v, want the created request", popped)
	}
	if popped.OauthHeaders["figma"] != "secret-bearer" {
		t.Fatalf("popped oauth headers = %v, want the figma bearer", popped.OauthHeaders)
	}
	if popped.Status != McpProbeRunning {
		t.Fatalf("popped status = %q, want running", popped.Status)
	}
	// A claimed request is no longer pending.
	if s.HasPending("rt-1") {
		t.Fatal("HasPending after pop = true, want false")
	}

	results := []protocol.McpProbeServerResult{
		{Name: "a", Status: "connected", ToolCount: 3},
	}
	s.Complete(req.ID, results)

	got := s.Get(req.ID)
	if got.Status != McpProbeCompleted {
		t.Fatalf("completed status = %q, want completed", got.Status)
	}
	if len(got.Results) != 1 || got.Results[0].ToolCount != 3 {
		t.Fatalf("results = %+v, want one connected server with 3 tools", got.Results)
	}
	// The polling response must never carry the secret-bearing OAuth headers.
	blob, _ := json.Marshal(got)
	if string(blob) == "" || strings.Contains(string(blob), "secret-bearer") || containsConfig(blob) {
		t.Fatalf("probe JSON leaked a secret: %s", blob)
	}
}

func containsConfig(blob []byte) bool {
	var m map[string]json.RawMessage
	_ = json.Unmarshal(blob, &m)
	_, hasConfig := m["config"]
	_, hasMcpServers := m["mcpServers"]
	_, hasOauth := m["oauth_headers"]
	_, hasOauth2 := m["OauthHeaders"]
	return hasConfig || hasMcpServers || hasOauth || hasOauth2
}

func TestMcpProbePendingTimeout(t *testing.T) {
	s := NewInMemoryMcpProbeStore()
	req := s.Create("rt-1", "ws-1", nil)
	// Backdate so the pending threshold has elapsed.
	s.requests[req.ID].CreatedAt = time.Now().Add(-mcpProbePendingTimeout - time.Second)

	got := s.Get(req.ID)
	if got.Status != McpProbeTimeout {
		t.Fatalf("status = %q, want timeout", got.Status)
	}
}

func TestMcpProbeTerminalHelper(t *testing.T) {
	if !mcpProbeTerminal(McpProbeCompleted) || !mcpProbeTerminal(McpProbeTimeout) {
		t.Fatal("completed/timeout must be terminal")
	}
	if mcpProbeTerminal(McpProbePending) || mcpProbeTerminal(McpProbeRunning) {
		t.Fatal("pending/running must not be terminal")
	}
}
