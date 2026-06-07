// Package mcpprobe performs a real, short-lived MCP handshake against the
// servers in an agent's effective mcp_config and reports whether each one
// actually connects. It is meant to run on the daemon/runtime host (where
// stdio servers physically exist and remote OAuth tokens are reachable), then
// report results back to the control plane — the app server cannot probe stdio
// servers itself because the commands only exist on the runtime machine.
//
// "Connected" means the server completed the JSON-RPC `initialize` handshake;
// the tool count comes from a best-effort `tools/list`. This is a health check,
// not a session — every probe spawns/connects, handshakes, and tears down.
package mcpprobe

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

// Status is the outcome of probing one MCP server.
type Status string

const (
	// StatusConnected — the server completed the `initialize` handshake.
	StatusConnected Status = "connected"
	// StatusFailed — spawn/connect error, timeout, or a protocol error.
	StatusFailed Status = "failed"
	// StatusNeedsAuth — a remote server rejected the handshake with 401/403;
	// the runtime CLI's OAuth login is required before it will connect.
	StatusNeedsAuth Status = "needs_auth"
	// StatusSkipped — the entry's transport could not be classified, so there
	// was nothing concrete to probe (reported truthfully rather than guessed).
	StatusSkipped Status = "skipped"
)

// Result is the per-server probe outcome. JSON tags match the wire shape the
// daemon reports back and the UI renders.
type Result struct {
	Name      string `json:"name"`
	Status    Status `json:"status"`
	ToolCount int    `json:"tool_count"`
	Error     string `json:"error,omitempty"`
}

// defaultPerServerTimeout bounds a single server probe. Callers may pass a
// shorter context deadline; whichever fires first wins.
const defaultPerServerTimeout = 8 * time.Second

// serverEntry is the subset of an `mcpServers` entry the prober understands.
// Unknown fields are ignored. Both Claude-style (`type`) and OpenClaw-style
// (`transport`) transport hints are accepted.
type serverEntry struct {
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env"`
	URL       string            `json:"url"`
	HTTPURL   string            `json:"httpUrl"`
	ServerURL string            `json:"serverUrl"`
	Type      string            `json:"type"`
	Transport string            `json:"transport"`
	Headers   map[string]string `json:"headers"`
}

func (e serverEntry) endpoint() string {
	for _, u := range []string{e.URL, e.HTTPURL, e.ServerURL} {
		if strings.TrimSpace(u) != "" {
			return u
		}
	}
	return ""
}

// ProbeConfig probes every active server in `rawConfig` concurrently and
// returns one Result per server, sorted by name. Only the `mcpServers` map is
// probed; `disabledMcpServers` and any other keys are ignored (the caller is
// expected to pass the effective, already-merged config). A malformed or empty
// config yields an empty slice, never an error.
func ProbeConfig(ctx context.Context, rawConfig []byte) []Result {
	var cfg struct {
		McpServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if len(bytes.TrimSpace(rawConfig)) == 0 {
		return nil
	}
	if err := json.Unmarshal(rawConfig, &cfg); err != nil || len(cfg.McpServers) == 0 {
		return nil
	}

	results := make([]Result, 0, len(cfg.McpServers))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for name, raw := range cfg.McpServers {
		wg.Add(1)
		go func(name string, raw json.RawMessage) {
			defer wg.Done()
			res := probeOne(ctx, name, raw)
			mu.Lock()
			results = append(results, res)
			mu.Unlock()
		}(name, raw)
	}
	wg.Wait()

	sort.Slice(results, func(i, j int) bool { return results[i].Name < results[j].Name })
	return results
}

func probeOne(ctx context.Context, name string, raw json.RawMessage) Result {
	var entry serverEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return Result{Name: name, Status: StatusSkipped, Error: "unrecognized server entry"}
	}

	pctx, cancel := context.WithTimeout(ctx, defaultPerServerTimeout)
	defer cancel()

	switch {
	case strings.TrimSpace(entry.Command) != "":
		return probeStdio(pctx, name, entry)
	case entry.endpoint() != "":
		return probeHTTP(pctx, name, entry)
	default:
		return Result{Name: name, Status: StatusSkipped, Error: "no command or url"}
	}
}

// --- JSON-RPC helpers shared by both transports ---

func initializeRequest() []byte {
	b, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "multica-probe", "version": "1"},
		},
	})
	return b
}

func toolsListRequest() []byte {
	b, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/list",
	})
	return b
}

func initializedNotification() []byte {
	b, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "method": "notifications/initialized",
	})
	return b
}

// rpcResponse is a tolerant view of a JSON-RPC response: we only care whether
// the id matches, whether it errored, and (for tools/list) the tool count.
type rpcResponse struct {
	ID     json.RawMessage `json:"id"`
	Result struct {
		Tools []json.RawMessage `json:"tools"`
	} `json:"result"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (r rpcResponse) idEquals(want string) bool {
	return strings.TrimSpace(string(r.ID)) == want
}

// --- stdio transport ---

func probeStdio(ctx context.Context, name string, entry serverEntry) Result {
	cmd := exec.CommandContext(ctx, entry.Command, entry.Args...)
	cmd.Env = os.Environ()
	for k, v := range entry.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Result{Name: name, Status: StatusFailed, Error: "stdin pipe: " + err.Error()}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{Name: name, Status: StatusFailed, Error: "stdout pipe: " + err.Error()}
	}
	if err := cmd.Start(); err != nil {
		return Result{Name: name, Status: StatusFailed, Error: "spawn: " + err.Error()}
	}
	// Always reap the process so a probe never leaks a child.
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	// Writes are best-effort: a server that responds and exits immediately can
	// close stdin under us (EPIPE), which is not a probe failure.
	writeLine := func(b []byte) { _, _ = stdin.Write(append(b, '\n')) }

	reader := bufio.NewReaderSize(stdout, 1<<20)
	lines := make(chan []byte, 16)
	go func() {
		defer close(lines)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				select {
				case lines <- line:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	writeLine(initializeRequest())

	// Read until the initialize (id:1) response, ignoring log lines / non-JSON
	// chatter some servers emit on stdout.
	if _, ok := awaitResponse(ctx, lines, "1"); !ok {
		return Result{Name: name, Status: StatusFailed, Error: "no initialize response"}
	}

	writeLine(initializedNotification())
	writeLine(toolsListRequest())

	toolCount := 0
	if resp, ok := awaitResponse(ctx, lines, "2"); ok && resp.Error == nil {
		toolCount = len(resp.Result.Tools)
	}
	return Result{Name: name, Status: StatusConnected, ToolCount: toolCount}
}

// awaitResponse drains lines until one parses as a JSON-RPC response with the
// wanted id, the context expires, or the stream closes.
func awaitResponse(ctx context.Context, lines <-chan []byte, wantID string) (rpcResponse, bool) {
	for {
		select {
		case <-ctx.Done():
			return rpcResponse{}, false
		case line, ok := <-lines:
			if !ok {
				return rpcResponse{}, false
			}
			var resp rpcResponse
			if err := json.Unmarshal(bytes.TrimSpace(line), &resp); err != nil {
				continue // log line / partial / non-JSON — keep reading
			}
			if resp.idEquals(wantID) {
				return resp, true
			}
		}
	}
}

// --- http / SSE transport ---

func probeHTTP(ctx context.Context, name string, entry serverEntry) Result {
	endpoint := entry.endpoint()
	client := &http.Client{}

	rpc := httpRPC(ctx, client, endpoint, entry.Headers, "", initializeRequest())
	switch rpc.status {
	case statusAuth:
		return Result{Name: name, Status: StatusNeedsAuth, Error: "authentication required"}
	case statusError:
		return Result{Name: name, Status: StatusFailed, Error: rpc.errMsg}
	}

	// Connected. tools/list is best-effort — a server may gate it behind the
	// captured session id; a failure here doesn't downgrade the handshake.
	toolCount := 0
	if list := httpRPC(ctx, client, endpoint, entry.Headers, rpc.sessionID, toolsListRequest()); list.parsed != nil {
		toolCount = len(list.parsed.Result.Tools)
	}
	return Result{Name: name, Status: StatusConnected, ToolCount: toolCount}
}

type httpStatus int

const (
	statusOK httpStatus = iota
	statusAuth
	statusError
)

type httpRPCResult struct {
	status    httpStatus
	sessionID string
	parsed    *rpcResponse
	errMsg    string
}

// httpRPC sends one JSON-RPC message over Streamable HTTP and inspects the
// response: classified status, any `Mcp-Session-Id` from the response header,
// the parsed JSON-RPC body (nil if unparseable), and an error string when the
// status is statusError. Returning the parsed body keeps the probe
// concurrency-safe (no shared state across the parallel per-server probes).
func httpRPC(ctx context.Context, client *http.Client, endpoint string, headers map[string]string, sessionID string, payload []byte) httpRPCResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return httpRPCResult{status: statusError, errMsg: "bad url"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := client.Do(req)
	if err != nil {
		return httpRPCResult{status: statusError, errMsg: "connect: " + err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return httpRPCResult{status: statusAuth}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return httpRPCResult{status: statusError, errMsg: fmt.Sprintf("http %d", resp.StatusCode)}
	}

	out := httpRPCResult{
		status:    statusOK,
		sessionID: resp.Header.Get("Mcp-Session-Id"),
		parsed:    parseRPCBody(resp),
	}
	if out.parsed != nil && out.parsed.Error != nil {
		out.status = statusError
		out.errMsg = out.parsed.Error.Message
	}
	return out
}

// parseRPCBody extracts the first JSON-RPC response from either a plain JSON
// body or an SSE (`text/event-stream`) body.
func parseRPCBody(resp *http.Response) *rpcResponse {
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			data, found := strings.CutPrefix(line, "data:")
			if !found {
				continue
			}
			var r rpcResponse
			if json.Unmarshal([]byte(strings.TrimSpace(data)), &r) == nil && len(r.ID) > 0 {
				return &r
			}
		}
		return nil
	}
	var r rpcResponse
	dec := json.NewDecoder(resp.Body)
	if dec.Decode(&r) == nil {
		return &r
	}
	return nil
}
