package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// putReq builds a PUT /mcp-config request with the workspace id wired into the
// chi route context, so workspaceIDFromURL resolves without middleware.
func putReq(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "11111111-1111-1111-1111-111111111111")
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestUpdateWorkspaceMcpConfigValidation(t *testing.T) {
	h := &Handler{}

	tests := []struct {
		name string
		body string
		want int
	}{
		{name: "invalid JSON body", body: "not json", want: http.StatusBadRequest},
		{name: "missing mcp_config field", body: `{}`, want: http.StatusBadRequest},
		{name: "non-object mcp_config", body: `{"mcp_config": 5}`, want: http.StatusBadRequest},
		{name: "array mcp_config", body: `{"mcp_config": []}`, want: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.UpdateWorkspaceMcpConfig(w, putReq(tt.body))
			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d (body: %s)", w.Code, tt.want, w.Body.String())
			}
		})
	}
}

// normalize re-marshals JSON so key ordering does not affect comparison.
func normalize(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("not valid JSON: %v (%s)", err, raw)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func TestMergeWorkspaceAgentMcpConfig(t *testing.T) {
	tests := []struct {
		name      string
		workspace string
		agent     string
		want      string // "" → nil/empty result
	}{
		{
			name:      "no workspace config returns the agent config unchanged",
			workspace: "",
			agent:     `{"mcpServers":{"a":{"command":"x"}}}`,
			want:      `{"mcpServers":{"a":{"command":"x"}}}`,
		},
		{
			name:      "no agent config returns the workspace config",
			workspace: `{"mcpServers":{"shared":{"command":"w"}}}`,
			agent:     "",
			want:      `{"mcpServers":{"shared":{"command":"w"}}}`,
		},
		{
			name:      "agent inherits workspace servers and adds its own",
			workspace: `{"mcpServers":{"shared":{"command":"w"}}}`,
			agent:     `{"mcpServers":{"own":{"command":"a"}}}`,
			want:      `{"mcpServers":{"shared":{"command":"w"},"own":{"command":"a"}}}`,
		},
		{
			name:      "agent overrides a workspace server of the same name",
			workspace: `{"mcpServers":{"shared":{"command":"workspace"}}}`,
			agent:     `{"mcpServers":{"shared":{"command":"agent"}}}`,
			want:      `{"mcpServers":{"shared":{"command":"agent"}}}`,
		},
		{
			name:      "disabled sidecars from both sides survive the merge",
			workspace: `{"disabledMcpServers":{"w":{"command":"w"}}}`,
			agent:     `{"mcpServers":{"a":{"command":"a"}},"disabledMcpServers":{"x":{"command":"x"}}}`,
			want:      `{"mcpServers":{"a":{"command":"a"}},"disabledMcpServers":{"w":{"command":"w"},"x":{"command":"x"}}}`,
		},
		{
			name:      "both empty → empty",
			workspace: "",
			agent:     "",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ws, ag json.RawMessage
			if tt.workspace != "" {
				ws = json.RawMessage(tt.workspace)
			}
			if tt.agent != "" {
				ag = json.RawMessage(tt.agent)
			}
			got := normalize(t, mergeWorkspaceAgentMcpConfig(ws, ag))
			want := normalize(t, json.RawMessage(tt.want))
			if got != want {
				t.Fatalf("mergeWorkspaceAgentMcpConfig() = %s, want %s", got, want)
			}
		})
	}
}
