package mcpoauth

import (
	"testing"
	"time"
)

func TestFlowStoreTakeConsumesOnce(t *testing.T) {
	s := NewFlowStore()
	s.Put("st", Flow{WorkspaceID: "ws", Resource: "https://r/mcp"})

	got, ok := s.Take("st")
	if !ok || got.WorkspaceID != "ws" || got.Resource != "https://r/mcp" {
		t.Fatalf("first Take failed: %+v ok=%v", got, ok)
	}
	if _, ok := s.Take("st"); ok {
		t.Fatal("state should be single-use; second Take must fail")
	}
	if _, ok := s.Take("nope"); ok {
		t.Fatal("unknown state must fail")
	}
}

func TestFlowStoreExpiry(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	s := NewFlowStore()
	s.nowFunc = func() time.Time { return now }
	s.Put("st", Flow{WorkspaceID: "ws"})

	now = now.Add(flowTTL + time.Second) // advance past TTL
	if _, ok := s.Take("st"); ok {
		t.Fatal("expired flow must not be returned")
	}
}
