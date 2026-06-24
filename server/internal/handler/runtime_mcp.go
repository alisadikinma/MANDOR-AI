package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// RuntimeMcpResponse is the read-only mirror of a runtime's machine MCP pool,
// served to workspace members on the Runtime page. Servers carries name +
// transport only (no secrets); ProbeResults is the latest connection-test
// outcome for the runtime, empty until a probe has run.
type RuntimeMcpResponse struct {
	Servers      []protocol.McpServerInfo        `json:"servers"`
	ProbeResults []protocol.McpProbeServerResult `json:"probe_results"`
}

// GetRuntimeMcp returns the runtime's reported MCP pool plus its latest probe
// results. Member-gated via the runtime's workspace, mirroring GetRuntimeUsage.
func (h *Handler) GetRuntimeMcp(w http.ResponseWriter, r *http.Request) {
	runtimeUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "runtimeId"), "runtimeId")
	if !ok {
		return
	}
	rt, err := h.Queries.GetAgentRuntime(r.Context(), runtimeUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "runtime not found")
		return
	}
	if _, ok := h.requireWorkspaceMember(w, r, uuidToString(rt.WorkspaceID), "runtime not found"); !ok {
		return
	}

	// Default to a non-nil empty slice so the JSON is `[]`, never `null` — the
	// FE parses defensively but an explicit empty list is clearer than null.
	servers := []protocol.McpServerInfo{}
	if len(rt.ReportedMcpServers) > 0 {
		if err := json.Unmarshal(rt.ReportedMcpServers, &servers); err != nil {
			// A corrupt stored value shouldn't 500 the page; log and show empty.
			slog.Warn("decode reported_mcp_servers", "runtime_id", uuidToString(rt.ID), "error", err)
			servers = []protocol.McpServerInfo{}
		}
	}

	results := []protocol.McpProbeServerResult{}
	if h.McpProbeStore != nil {
		if latest := h.McpProbeStore.LatestResultsForRuntime(uuidToString(rt.ID)); latest != nil {
			results = latest
		}
	}

	writeJSON(w, http.StatusOK, RuntimeMcpResponse{Servers: servers, ProbeResults: results})
}

// persistReportedMcpPool stores the daemon-reported machine MCP pool on the
// runtime row, but only when it differs from what's already stored — the
// daemon sends the same cached set on every 15s heartbeat, so an unconditional
// write would be pure churn. A nil servers slice means the daemon did not
// report (old daemon) — we never clobber a known pool with that.
func (h *Handler) persistReportedMcpPool(ctx context.Context, rt db.AgentRuntime, servers []protocol.McpServerInfo) {
	if servers == nil {
		return
	}
	encoded, err := json.Marshal(servers)
	if err != nil {
		return
	}
	if bytes.Equal(rt.ReportedMcpServers, encoded) {
		return
	}
	if err := h.Queries.UpdateRuntimeReportedMcpServers(ctx, db.UpdateRuntimeReportedMcpServersParams{
		ID:                 rt.ID,
		ReportedMcpServers: encoded,
	}); err != nil {
		slog.Warn("persist reported mcp pool", "runtime_id", uuidToString(rt.ID), "error", err)
	}
}
