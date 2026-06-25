package daemon

import (
	"encoding/json"

	"github.com/multica-ai/multica/server/internal/daemon/execenv"
)

// effectiveMcpConfig assembles the MCP config a task run uses, on the host:
// the runtime's own machine pool (for this provider) minus the servers the
// agent disabled, with OAuth bearer headers injected for the servers the
// control plane holds a token for. The control plane sends only the deny-list
// and the tokens (agent.McpConfig / agent.McpOauthHeaders); the server
// definitions come from the machine. Returns nil when there is no agent or no
// machine config, so the CLI falls back to its own defaults.
func (d *Daemon) effectiveMcpConfig(provider string, agent *AgentData) json.RawMessage {
	if agent == nil {
		return nil
	}
	machine, err := execenv.MachineMcpConfigJSON(provider, "")
	if err != nil {
		d.logger.Warn("effective mcp config: resolve machine config", "provider", provider, "error", err)
	}
	if len(machine) == 0 {
		return nil
	}
	return execenv.BuildEffectiveMcpConfig(machine, parseDisabledMcpServers(agent.McpConfig), agent.McpOauthHeaders)
}

// parseDisabledMcpServers extracts the agent's deny-list of server names from
// its stored mcp_config (`{"disabledMcpServers":[...]}`). Returns nil for an
// absent / null / malformed value (inherit the whole pool).
func parseDisabledMcpServers(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var obj struct {
		Disabled []string `json:"disabledMcpServers"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	return obj.Disabled
}
