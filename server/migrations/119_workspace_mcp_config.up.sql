-- Workspace-level MCP server config. Mirrors the per-agent `agent.mcp_config`
-- JSONB but applies workspace-wide: every agent in the workspace inherits these
-- servers, and the agent's own `mcp_config` overrides by server name at task
-- dispatch (handler.daemon.ClaimTask). Carries secrets, so reads are gated to
-- workspace owner/admin via the dedicated /mcp-config endpoints — it is never
-- folded into the general workspace response.
ALTER TABLE workspace ADD COLUMN mcp_config jsonb;
