-- The runtime host's own MCP server pool (name + transport only, never
-- secrets), reported by the daemon on its heartbeat. Agents on the runtime
-- reuse this pool instead of carrying their own server definitions, so the
-- control plane mirrors it read-only on the Runtime page. Nullable: a runtime
-- that has never reported (old daemon, or no MCP servers) has NULL here, which
-- the API surfaces as an empty pool.
ALTER TABLE agent_runtime ADD COLUMN reported_mcp_servers jsonb;
