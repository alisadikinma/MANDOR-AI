package handler

import (
	"bytes"
	"encoding/json"
)

// validateAgentMcpConfig enforces the deny-list shape for an agent's mcp_config
// under the runtime-pool model. An agent no longer carries MCP server
// definitions — those live on the runtime host and are reported as the
// runtime's pool. The agent only records which servers it disables from that
// pool. Accepts:
//
//   - null / empty           → inherit the whole runtime pool
//   - {"disabledMcpServers": ["<name>", ...]}  → inherit minus these names
//
// Anything else — `mcpServers` definitions, an unknown key, a non-object, or a
// non-string-array disabled list — is rejected so a stale client can't write
// the old full-config shape. Returns (errorMessage, ok).
func validateAgentMcpConfig(raw []byte) (string, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", true
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return "mcp_config must be a JSON object", false
	}
	for key := range obj {
		if key != "disabledMcpServers" {
			return "mcp_config may only contain disabledMcpServers; MCP servers are configured on the runtime now", false
		}
	}
	if dis, ok := obj["disabledMcpServers"]; ok {
		var names []string
		if err := json.Unmarshal(dis, &names); err != nil {
			return "disabledMcpServers must be an array of server names", false
		}
	}
	return "", true
}
