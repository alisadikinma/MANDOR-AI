package execenv

import "encoding/json"

// BuildEffectiveMcpConfig assembles the config a task run (or a probe) actually
// uses, entirely on the runtime host: start from the machine's own MCP pool,
// remove the servers the agent disabled, and inject `Authorization: Bearer`
// headers for the servers the control plane holds an OAuth/token for (keyed by
// server name, so no URL normalization is needed here).
//
// The control plane sends only the deny-list and the bearer tokens — never the
// server definitions — so this is where the host's own commands/env/urls and
// the platform's stored tokens come together. A user-set Authorization header
// is respected (never overwritten). Returns the input unchanged on any parse
// failure so a malformed pool never blocks a run.
func BuildEffectiveMcpConfig(machine json.RawMessage, disabled []string, bearerByName map[string]string) json.RawMessage {
	if len(machine) == 0 {
		return machine
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(machine, &root); err != nil {
		return machine
	}
	serversRaw, ok := root["mcpServers"]
	if !ok {
		return machine
	}
	var servers map[string]map[string]json.RawMessage
	if err := json.Unmarshal(serversRaw, &servers); err != nil {
		return machine
	}

	for _, name := range disabled {
		delete(servers, name)
	}

	for name, token := range bearerByName {
		entry, ok := servers[name]
		if !ok || token == "" {
			continue
		}
		headers := map[string]string{}
		if raw, ok := entry["headers"]; ok {
			_ = json.Unmarshal(raw, &headers)
		}
		if _, exists := headers["Authorization"]; exists {
			continue // respect a user-set Authorization
		}
		headers["Authorization"] = "Bearer " + token
		if hb, err := json.Marshal(headers); err == nil {
			entry["headers"] = hb
			servers[name] = entry
		}
	}

	if sb, err := json.Marshal(servers); err == nil {
		root["mcpServers"] = sb
	}
	if out, err := json.Marshal(root); err == nil {
		return out
	}
	return machine
}
