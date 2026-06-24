package daemon

import (
	"time"

	"github.com/multica-ai/multica/server/pkg/protocol"
)

// mcpPoolTTL bounds how stale the reported machine MCP pool may be. Heartbeats
// fire every 15s; re-resolving the pool (a CLI spawn for openclaw) that often
// is wasteful, and MCP config rarely changes mid-session. A 2-minute cache
// keeps the Runtime page's read-only mirror reasonably fresh after a config
// edit without spawning a CLI on every tick.
const mcpPoolTTL = 2 * time.Minute

type cachedMcpPool struct {
	servers []protocol.McpServerInfo
	at      time.Time
}

// machineMcpServers returns the runtime's machine MCP pool to attach to its
// heartbeat, resolved from the runtime's provider and cached per provider with
// mcpPoolTTL. On a resolver error it serves the last good cache (if any) rather
// than dropping the pool to empty — a transient CLI hiccup shouldn't make the
// Runtime page flash "no servers".
func (d *Daemon) machineMcpServers(rid string) []protocol.McpServerInfo {
	rt := d.findRuntime(rid)
	if rt == nil || rt.Provider == "" {
		return nil
	}
	provider := rt.Provider

	d.mcpPoolMu.Lock()
	defer d.mcpPoolMu.Unlock()
	if c, ok := d.mcpPoolCache[provider]; ok && time.Since(c.at) < mcpPoolTTL {
		return c.servers
	}
	servers, err := d.resolveMcpServers(provider, "")
	if err != nil {
		d.logger.Warn("resolve machine mcp servers", "provider", provider, "error", err)
		if c, ok := d.mcpPoolCache[provider]; ok {
			return c.servers // serve stale rather than empty
		}
		return nil
	}
	d.mcpPoolCache[provider] = cachedMcpPool{servers: servers, at: time.Now()}
	return servers
}
