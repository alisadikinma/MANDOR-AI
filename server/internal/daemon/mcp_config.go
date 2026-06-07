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
	disabledRaw, ok := obj["disabledMcpServers"]
	if !ok {
		return raw
	}
	delete(obj, "disabledMcpServers")

	// A disabled entry wins over an active one of the same name. This matters
	// after the workspace∪agent merge: a server the workspace enables can be
	// disabled at the agent level, which re-appears in the merged `mcpServers`
	// map — drop it so the disable actually takes effect.
	var disabled map[string]json.RawMessage
	if err := json.Unmarshal(disabledRaw, &disabled); err == nil && len(disabled) > 0 {
		if activeRaw, ok := obj["mcpServers"]; ok {
			var active map[string]json.RawMessage
			if json.Unmarshal(activeRaw, &active) == nil {
				for name := range disabled {
					delete(active, name)
				}
				if len(active) == 0 {
					delete(obj, "mcpServers")
				} else if b, err := json.Marshal(active); err == nil {
					obj["mcpServers"] = b
				}
			}
		}
	}

	if len(obj) == 0 {
		return nil
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return raw
	}
	return out
}
