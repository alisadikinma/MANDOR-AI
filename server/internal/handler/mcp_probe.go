package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/pkg/protocol"
)

// ---------------------------------------------------------------------------
// MCP connection probe
// ---------------------------------------------------------------------------
//
// "Test connections" asks the runtime to actually handshake each MCP server in
// an agent's (or workspace's) effective config and report whether it connects.
// The server can't reach the daemon directly, so this reuses the same
// pending-request-on-heartbeat pattern as model-list discovery: a frontend POST
// enqueues a request carrying the effective config, the daemon pops it on its
// next heartbeat, runs mcpprobe locally (where stdio servers exist and OAuth
// tokens are reachable), and reports per-server status back. The UI polls the
// request until it reaches a terminal state.

type McpProbeStatus string

const (
	McpProbePending   McpProbeStatus = "pending"
	McpProbeRunning   McpProbeStatus = "running"
	McpProbeCompleted McpProbeStatus = "completed"
	McpProbeTimeout   McpProbeStatus = "timeout"
)

// McpProbeRequest is a pending or completed probe. Config carries secrets and
// is `json:"-"` so the polling UI never receives it — only per-server Results.
type McpProbeRequest struct {
	ID           string                          `json:"id"`
	RuntimeID    string                          `json:"runtime_id"`
	WorkspaceID  string                          `json:"-"`
	Config       json.RawMessage                 `json:"-"`
	Status       McpProbeStatus                  `json:"status"`
	Results      []protocol.McpProbeServerResult `json:"results,omitempty"`
	Error        string                          `json:"error,omitempty"`
	CreatedAt    time.Time                       `json:"created_at"`
	UpdatedAt    time.Time                       `json:"updated_at"`
	RunStartedAt *time.Time                      `json:"-"`
}

const (
	mcpProbePendingTimeout = 30 * time.Second
	mcpProbeRunningTimeout = 30 * time.Second
	mcpProbeRetention      = 3 * time.Minute
)

func mcpProbeTerminal(s McpProbeStatus) bool {
	return s == McpProbeCompleted || s == McpProbeTimeout
}

// applyMcpProbeTimeout transitions a stuck request to timeout. Mirrors the
// model-list timeouts: pending catches "daemon never picked it up"; running
// catches "picked up but the report was lost".
func applyMcpProbeTimeout(req *McpProbeRequest, now time.Time) {
	switch req.Status {
	case McpProbePending:
		if now.Sub(req.CreatedAt) > mcpProbePendingTimeout {
			req.Status = McpProbeTimeout
			req.Error = "runtime did not pick up the probe in time"
			req.UpdatedAt = now
		}
	case McpProbeRunning:
		if req.RunStartedAt != nil && now.Sub(*req.RunStartedAt) > mcpProbeRunningTimeout {
			req.Status = McpProbeTimeout
			req.Error = "runtime did not finish the probe in time"
			req.UpdatedAt = now
		}
	}
}

// InMemoryMcpProbeStore is the single-node store. Like the model-list store it
// is unsafe across replicas (each gets its own map); a multi-node deploy would
// need a Redis-backed variant. Adequate for self-hosted and the test suite.
type InMemoryMcpProbeStore struct {
	mu       sync.Mutex
	requests map[string]*McpProbeRequest
}

func NewInMemoryMcpProbeStore() *InMemoryMcpProbeStore {
	return &InMemoryMcpProbeStore{requests: make(map[string]*McpProbeRequest)}
}

func (s *InMemoryMcpProbeStore) Create(runtimeID, workspaceID string, config json.RawMessage) *McpProbeRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, req := range s.requests {
		if time.Since(req.CreatedAt) > mcpProbeRetention {
			delete(s.requests, id)
		}
	}

	now := time.Now()
	req := &McpProbeRequest{
		ID:          randomID(),
		RuntimeID:   runtimeID,
		WorkspaceID: workspaceID,
		Config:      config,
		Status:      McpProbePending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.requests[req.ID] = req
	return req
}

func (s *InMemoryMcpProbeStore) Get(id string) *McpProbeRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	req, ok := s.requests[id]
	if !ok {
		return nil
	}
	applyMcpProbeTimeout(req, time.Now())
	return req
}

func (s *InMemoryMcpProbeStore) HasPending(runtimeID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, req := range s.requests {
		applyMcpProbeTimeout(req, now)
		if req.RuntimeID == runtimeID && req.Status == McpProbePending {
			return true
		}
	}
	return false
}

// PopPending claims the oldest pending request for a runtime and hands back its
// config so the heartbeat can carry it to the daemon.
func (s *InMemoryMcpProbeStore) PopPending(runtimeID string) *McpProbeRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var oldest *McpProbeRequest
	for _, req := range s.requests {
		applyMcpProbeTimeout(req, now)
		if req.RuntimeID == runtimeID && req.Status == McpProbePending {
			if oldest == nil || req.CreatedAt.Before(oldest.CreatedAt) {
				oldest = req
			}
		}
	}
	if oldest != nil {
		oldest.Status = McpProbeRunning
		started := now
		oldest.RunStartedAt = &started
		oldest.UpdatedAt = now
	}
	return oldest
}

func (s *InMemoryMcpProbeStore) Complete(id string, results []protocol.McpProbeServerResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req, ok := s.requests[id]; ok {
		req.Status = McpProbeCompleted
		req.Results = results
		req.UpdatedAt = time.Now()
	}
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// InitiateAgentMcpProbe enqueues a probe of an agent's effective MCP config.
// Gated to callers allowed to see the agent's secrets (probing connects with
// them). Returns the pending request; the UI polls GET /api/mcp-probe/{id}.
func (h *Handler) InitiateAgentMcpProbe(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agent, ok := h.loadAgentForUser(w, r, id)
	if !ok {
		return
	}
	member, ok := ctxMember(r.Context())
	if !ok || !canViewAgentSecrets(agent, requestUserID(r), member.Role) {
		writeError(w, http.StatusForbidden, "you are not allowed to test this agent's MCP servers")
		return
	}
	if !agent.RuntimeID.Valid {
		writeError(w, http.StatusBadRequest, "agent has no runtime to test on")
		return
	}
	runtimeID := uuidToString(agent.RuntimeID)
	if !h.runtimeOnline(r.Context(), runtimeID) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "runtime_offline"})
		return
	}

	var workspaceCfg json.RawMessage
	if raw, err := h.Queries.GetWorkspaceMcpConfig(r.Context(), agent.WorkspaceID); err == nil && len(raw) > 0 {
		workspaceCfg = json.RawMessage(raw)
	}
	effective := mergeWorkspaceAgentMcpConfig(workspaceCfg, agent.McpConfig)

	req := h.McpProbeStore.Create(runtimeID, uuidToString(agent.WorkspaceID), effective)
	writeJSON(w, http.StatusOK, req)
}

// InitiateWorkspaceMcpProbe enqueues a probe of the workspace-level MCP config.
// Mounted under the owner/admin route group. Runs on the first online runtime
// in the workspace (the servers are workspace-wide, so any runtime is a valid
// place to test them).
func (h *Handler) InitiateWorkspaceMcpProbe(w http.ResponseWriter, r *http.Request) {
	wsID := workspaceIDFromURL(r, "id")
	wsUUID, ok := parseUUIDOrBadRequest(w, wsID, "workspace id")
	if !ok {
		return
	}
	runtimes, err := h.Queries.ListAgentRuntimes(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to resolve a runtime to test on")
		return
	}
	runtimeID := ""
	for _, rt := range runtimes {
		if rt.Status == "online" && h.runtimeOnline(r.Context(), uuidToString(rt.ID)) {
			runtimeID = uuidToString(rt.ID)
			break
		}
	}
	if runtimeID == "" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "runtime_offline"})
		return
	}

	var cfg json.RawMessage
	if raw, err := h.Queries.GetWorkspaceMcpConfig(r.Context(), wsUUID); err == nil && len(raw) > 0 {
		cfg = json.RawMessage(raw)
	}
	req := h.McpProbeStore.Create(runtimeID, wsID, cfg)
	writeJSON(w, http.StatusOK, req)
}

// GetMcpProbeRequest returns the status/results of a probe. Authorized to any
// member of the probe's workspace.
func (h *Handler) GetMcpProbeRequest(w http.ResponseWriter, r *http.Request) {
	req := h.McpProbeStore.Get(chi.URLParam(r, "requestId"))
	if req == nil {
		writeError(w, http.StatusNotFound, "probe not found")
		return
	}
	if _, ok := h.requireWorkspaceMember(w, r, req.WorkspaceID, "probe not found"); !ok {
		return
	}
	writeJSON(w, http.StatusOK, req)
}

// ReportMcpProbeResult receives per-server probe results from the daemon.
func (h *Handler) ReportMcpProbeResult(w http.ResponseWriter, r *http.Request) {
	runtimeID := chi.URLParam(r, "runtimeId")
	if _, ok := h.requireDaemonRuntimeAccess(w, r, runtimeID); !ok {
		return
	}
	requestID := chi.URLParam(r, "requestId")

	existing := h.McpProbeStore.Get(requestID)
	if existing == nil || existing.RuntimeID != runtimeID {
		writeError(w, http.StatusNotFound, "probe not found")
		return
	}
	if mcpProbeTerminal(existing.Status) {
		slog.Debug("ignoring stale mcp probe report", "runtime_id", runtimeID, "request_id", requestID, "status", existing.Status)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	var body protocol.McpProbeResultPayload
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	h.McpProbeStore.Complete(requestID, body.Results)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// runtimeOnline reports whether the runtime has a live daemon connection (so a
// probe can actually be picked up). Falls back to the persisted runtime status
// when the in-process hub is unavailable (e.g. multi-node without a shared hub).
func (h *Handler) runtimeOnline(ctx context.Context, runtimeID string) bool {
	if h.DaemonHub != nil && h.DaemonHub.RuntimeConnectionCount(runtimeID) > 0 {
		return true
	}
	// runtimeID is always a trusted DB UUID round-trip (callers pass
	// uuidToString of a loaded row), so parseUUID's panic-on-invalid contract
	// is the right strictness here.
	rt, err := h.Queries.GetAgentRuntime(ctx, parseUUID(runtimeID))
	if err != nil {
		return false
	}
	return rt.Status == "online"
}
