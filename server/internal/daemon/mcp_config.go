package daemon

import "encoding/json"

// runtimeMcpConfig returns the agent's stored mcp_config with the UI-only
// `disabledMcpServers` sidecar key removed, so servers a user disabled in the
// UI never reach the runtime CLI. The active `mcpServers` map and any other
// top-level keys pass through verbatim.
//
// The disabled servers are persisted (the UI keeps them so they can be
// re-enabled without re-entering config), but they are not valid runtime
// config — Claude/Codex/etc. would either error on or ignore the unknown key.
//
// Behaviour:
//   - empty / whitespace input → returned unchanged (no managed config)
//   - JSON that is not an object → returned unchanged (some runtime may accept
//     a shape we do not model; we never silently drop it)
//   - object without `disabledMcpServers` → returned unchanged
//   - object where stripping leaves `{}` (e.g. every server was disabled) →
//     nil, so the daemon falls back to the CLI's own default instead of
//     forcing a managed-empty config that would suppress the user's global
//     servers
func runtimeMcpConfig(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return raw
	}
	if _, ok := obj["disabledMcpServers"]; !ok {
		return raw
	}
	delete(obj, "disabledMcpServers")
	if len(obj) == 0 {
		return nil
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return raw
	}
	return out
}
