package daemon

import (
	"log/slog"
	"testing"

	"github.com/multica-ai/multica/server/pkg/protocol"
)

func TestMachineMcpServersResolvesByProviderAndCaches(t *testing.T) {
	const rid = "rt-1"
	calls := 0
	d := &Daemon{
		logger:       slog.Default(),
		runtimeIndex: map[string]Runtime{rid: {ID: rid, Provider: "codex"}},
		mcpPoolCache: make(map[string]cachedMcpPool),
		resolveMcpServers: func(runtimeType, _ string) ([]protocol.McpServerInfo, error) {
			calls++
			if runtimeType != "codex" {
				t.Fatalf("expected provider codex, got %q", runtimeType)
			}
			return []protocol.McpServerInfo{{Name: "github", Transport: "stdio"}}, nil
		},
	}

	got := d.machineMcpServers(rid)
	if len(got) != 1 || got[0].Name != "github" {
		t.Fatalf("unexpected servers: %v", got)
	}
	// Second call within TTL must hit the cache, not re-resolve (openclaw would
	// spawn a CLI on every heartbeat otherwise).
	d.machineMcpServers(rid)
	if calls != 1 {
		t.Fatalf("expected 1 resolve call (cached), got %d", calls)
	}
}

func TestMachineMcpServersUnknownRuntimeIsNil(t *testing.T) {
	d := &Daemon{
		logger:       slog.Default(),
		runtimeIndex: make(map[string]Runtime),
		mcpPoolCache: make(map[string]cachedMcpPool),
		resolveMcpServers: func(string, string) ([]protocol.McpServerInfo, error) {
			t.Fatal("resolver must not be called for unknown runtime")
			return nil, nil
		},
	}
	if got := d.machineMcpServers("nope"); got != nil {
		t.Fatalf("expected nil for unknown runtime, got %v", got)
	}
}

func TestHeartbeatPayloadCarriesMcpServers(t *testing.T) {
	// Compile-level guard: the wire payload must have the field so both
	// heartbeat transports can attach the pool.
	p := protocol.DaemonHeartbeatRequestPayload{
		RuntimeID:  "x",
		McpServers: []protocol.McpServerInfo{{Name: "a", Transport: "http"}},
	}
	if len(p.McpServers) != 1 {
		t.Fatal("McpServers not carried")
	}
}
