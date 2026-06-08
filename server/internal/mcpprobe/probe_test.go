package mcpprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"
)

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return c
}

// requireSh skips on platforms without a POSIX shell (the stdio fake server).
func requireSh(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stdio fake server needs a POSIX shell")
	}
}

func TestProbeStdioConnected(t *testing.T) {
	requireSh(t)
	// A fake MCP server: prints the initialize and tools/list responses up
	// front, then idles briefly so the prober can read both before exit.
	script := `printf '{"jsonrpc":"2.0","id":1,"result":{}}\n` +
		`{"jsonrpc":"2.0","id":2,"result":{"tools":[{},{},{}]}}\n'; sleep 0.3`
	raw := json.RawMessage(fmt.Sprintf(
		`{"command":"sh","args":["-c",%q]}`, script))

	got := probeOne(ctx(t), "fake", raw)
	if got.Status != StatusConnected {
		t.Fatalf("status = %q (%s), want connected", got.Status, got.Error)
	}
	if got.ToolCount != 3 {
		t.Fatalf("tool count = %d, want 3", got.ToolCount)
	}
}

func TestProbeStdioSpawnFailure(t *testing.T) {
	requireSh(t)
	raw := json.RawMessage(`{"command":"this-binary-does-not-exist-multica"}`)
	got := probeOne(ctx(t), "missing", raw)
	if got.Status != StatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
}

func TestProbeStdioNoInitializeResponse(t *testing.T) {
	requireSh(t)
	// Emits only non-JSON chatter — the prober must time out / fail rather than
	// report a phantom connection.
	raw := json.RawMessage(`{"command":"sh","args":["-c","echo not-json; sleep 0.2"]}`)
	c, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	got := probeOne(c, "chatty", raw)
	if got.Status != StatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
}

func TestProbeStdioDeadlineExceeded(t *testing.T) {
	requireSh(t)
	// A server that stays alive but never answers, so the per-server context
	// deadline (not a closed stream) is what ends the wait — the slow-start case
	// a cold `npx` hits. The error must say so, not "no initialize response".
	raw := json.RawMessage(`{"command":"sh","args":["-c","sleep 5"]}`)
	c, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	got := probeOne(c, "slow", raw)
	if got.Status != StatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if !strings.Contains(got.Error, "did not respond before timeout") {
		t.Fatalf("error = %q, want the slow-start/deadline message", got.Error)
	}
}

func mcpJSONServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Mcp-Session-Id", "sess-123")
		switch req.Method {
		case "initialize":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{}}}`))
		case "tools/list":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{},{}]}}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
}

func TestProbeHTTPConnected(t *testing.T) {
	srv := mcpJSONServer(t)
	defer srv.Close()

	raw := json.RawMessage(fmt.Sprintf(`{"url":%q}`, srv.URL))
	got := probeOne(ctx(t), "remote", raw)
	if got.Status != StatusConnected {
		t.Fatalf("status = %q (%s), want connected", got.Status, got.Error)
	}
	if got.ToolCount != 2 {
		t.Fatalf("tool count = %d, want 2", got.ToolCount)
	}
}

func TestProbeHTTPNeedsAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	raw := json.RawMessage(fmt.Sprintf(`{"url":%q}`, srv.URL))
	got := probeOne(ctx(t), "oauth", raw)
	if got.Status != StatusNeedsAuth {
		t.Fatalf("status = %q, want needs_auth", got.Status)
	}
}

func TestProbeHTTPServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	raw := json.RawMessage(fmt.Sprintf(`{"url":%q}`, srv.URL))
	got := probeOne(ctx(t), "broken", raw)
	if got.Status != StatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
}

func TestProbeHTTPViaSSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "text/event-stream")
		if req.Method == "initialize" {
			_, _ = w.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n\n"))
		} else {
			_, _ = w.Write([]byte("data: {\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"tools\":[{}]}}\n\n"))
		}
	}))
	defer srv.Close()

	raw := json.RawMessage(fmt.Sprintf(`{"url":%q,"transport":"sse"}`, srv.URL))
	got := probeOne(ctx(t), "sse", raw)
	if got.Status != StatusConnected {
		t.Fatalf("status = %q (%s), want connected", got.Status, got.Error)
	}
	if got.ToolCount != 1 {
		t.Fatalf("tool count = %d, want 1", got.ToolCount)
	}
}

func TestProbeConfigOnlyActiveServers(t *testing.T) {
	srv := mcpJSONServer(t)
	defer srv.Close()

	raw := json.RawMessage(fmt.Sprintf(`{
		"mcpServers": {"remote": {"url": %q}},
		"disabledMcpServers": {"off": {"url": %q}}
	}`, srv.URL, srv.URL))

	results := ProbeConfig(ctx(t), raw)
	if len(results) != 1 {
		t.Fatalf("probed %d servers, want 1 (disabled must be skipped)", len(results))
	}
	if results[0].Name != "remote" || results[0].Status != StatusConnected {
		t.Fatalf("unexpected result: %+v", results[0])
	}
}

func TestProbeConfigEmpty(t *testing.T) {
	if got := ProbeConfig(ctx(t), nil); got != nil {
		t.Fatalf("nil config → %v, want nil", got)
	}
	if got := ProbeConfig(ctx(t), json.RawMessage(`{}`)); got != nil {
		t.Fatalf("empty object → %v, want nil", got)
	}
}

func TestProbeSkippedUnknownTransport(t *testing.T) {
	raw := json.RawMessage(`{"foo":"bar"}`)
	got := probeOne(ctx(t), "weird", raw)
	if got.Status != StatusSkipped {
		t.Fatalf("status = %q, want skipped", got.Status)
	}
}
