package handler

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/internal/logger"
)

// workspaceMcpConfigResponse is the body for GET/PUT of the workspace-level MCP
// config. `mcp_config` is JSON null when no workspace config is set.
type workspaceMcpConfigResponse struct {
	McpConfig json.RawMessage `json:"mcp_config"`
}

// updateWorkspaceMcpConfigRequest carries the new config. The pointer makes the
// field tri-state on the wire (mirrors the agent path):
//   - field omitted (nil pointer) → 400, so the endpoint never silently no-ops
//   - explicit JSON null → clear the column (agents fall back to their own config)
//   - object → replace the stored JSON verbatim
type updateWorkspaceMcpConfigRequest struct {
	McpConfig *json.RawMessage `json:"mcp_config"`
}

// GetWorkspaceMcpConfig returns the workspace-level MCP server config. Mounted
// under the owner/admin route group: MCP configs carry secrets, so the route
// middleware (RequireWorkspaceRoleFromURL owner/admin) is the gate — members
// without that role are rejected before reaching this handler, mirroring the
// agent rule that only owner/admin may read mcp_config.
func (h *Handler) GetWorkspaceMcpConfig(w http.ResponseWriter, r *http.Request) {
	id := workspaceIDFromURL(r, "id")
	idUUID, ok := parseUUIDOrBadRequest(w, id, "workspace id")
	if !ok {
		return
	}
	raw, err := h.Queries.GetWorkspaceMcpConfig(r.Context(), idUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	// A nil RawMessage marshals to JSON null — exactly the "no config" signal.
	writeJSON(w, http.StatusOK, workspaceMcpConfigResponse{McpConfig: raw})
}

// UpdateWorkspaceMcpConfig replaces (or clears) the workspace-level MCP config.
// Same owner/admin route gate as the read.
func (h *Handler) UpdateWorkspaceMcpConfig(w http.ResponseWriter, r *http.Request) {
	id := workspaceIDFromURL(r, "id")
	idUUID, ok := parseUUIDOrBadRequest(w, id, "workspace id")
	if !ok {
		return
	}

	var req updateWorkspaceMcpConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.McpConfig == nil {
		writeError(w, http.StatusBadRequest, "mcp_config is required (send null to clear)")
		return
	}

	var stored []byte
	trimmed := bytes.TrimSpace(*req.McpConfig)
	if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
		// Must be a JSON object (e.g. {"mcpServers": …}); reject arrays/scalars
		// so a malformed save can't corrupt the column for every agent.
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &obj); err != nil {
			writeError(w, http.StatusBadRequest, `mcp_config must be a JSON object (e.g. {"mcpServers": …}) or null`)
			return
		}
		stored = append([]byte(nil), trimmed...)
	}

	raw, err := h.Queries.UpdateWorkspaceMcpConfig(r.Context(), db.UpdateWorkspaceMcpConfigParams{
		ID:        idUUID,
		McpConfig: stored,
	})
	if err != nil {
		slog.Warn("update workspace mcp_config failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", id)...)
		writeError(w, http.StatusInternalServerError, "failed to update workspace MCP config")
		return
	}

	writeJSON(w, http.StatusOK, workspaceMcpConfigResponse{McpConfig: raw})
}

// parseMcpConfigObject parses an mcp_config value into its top-level object map.
// Empty/whitespace or non-object JSON returns nil (treated as "no config").
func parseMcpConfigObject(raw json.RawMessage) map[string]json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	return obj
}

// mergeServerMaps merges two server maps (mcpServers / disabledMcpServers) with
// the agent winning per server name. Returns nil when both sides are empty.
func mergeServerMaps(workspace, agent json.RawMessage) json.RawMessage {
	wsM := parseMcpConfigObject(workspace)
	agM := parseMcpConfigObject(agent)
	if len(wsM) == 0 && len(agM) == 0 {
		return nil
	}
	out := make(map[string]json.RawMessage, len(wsM)+len(agM))
	for k, v := range wsM {
		out[k] = v
	}
	for k, v := range agM {
		out[k] = v // agent overrides a workspace server of the same name
	}
	merged, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	return merged
}

// mergeWorkspaceAgentMcpConfig folds the workspace-level MCP config into the
// agent's so every agent inherits workspace servers, with the agent's own
// config overriding by server name. The server maps (`mcpServers` and
// `disabledMcpServers`) are merged entry-by-entry; any other top-level key is
// taken from the agent when present, else the workspace. Either side may be
// empty/nil. The UI-only `disabledMcpServers` sidecar is preserved here and
// stripped later, at the daemon, by runtimeMcpConfig.
func mergeWorkspaceAgentMcpConfig(workspace, agent json.RawMessage) json.RawMessage {
	wsObj := parseMcpConfigObject(workspace)
	agObj := parseMcpConfigObject(agent)
	if wsObj == nil {
		return agent
	}
	if agObj == nil {
		return workspace
	}

	out := make(map[string]json.RawMessage, len(wsObj)+len(agObj))
	for k, v := range wsObj {
		out[k] = v
	}
	for k, v := range agObj {
		out[k] = v
	}
	for _, key := range []string{"mcpServers", "disabledMcpServers"} {
		if merged := mergeServerMaps(wsObj[key], agObj[key]); merged != nil {
			out[key] = merged
		} else {
			delete(out, key)
		}
	}

	res, err := json.Marshal(out)
	if err != nil {
		return agent
	}
	return res
}
