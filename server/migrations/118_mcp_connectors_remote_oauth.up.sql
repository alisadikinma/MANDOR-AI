-- Migrate the curated connectors that ship an official hosted (remote) MCP
-- server from per-token stdio entries to OAuth/login-connect remote entries.
-- The lazy seed (ensureGlobalMcpConnectorsSeeded) is insert-once — it
-- short-circuits when any global row exists — so editing the embedded JSON only
-- reaches FRESH installs. This statement reconciles already-seeded installs to
-- the same definitions. Rows that don't exist yet match nothing and are later
-- inserted from the updated JSON, so both paths converge.
--
-- These remote servers carry no secret: the runtime CLI performs the OAuth
-- handshake on first connect, so input_schema drops to no fields.
--
-- Atlassian uses the SSE endpoint, which Atlassian sunsets 2026-06-30 in favour
-- of the streamable-HTTP endpoint (https://mcp.atlassian.com/v1/mcp); update the
-- template before then.

UPDATE mcp_connector
SET description = 'Manage issues, pull requests, repositories, and code search across your GitHub organization. Sign in with GitHub on first connect — no token to paste.',
    input_schema = '{"fields": []}'::jsonb,
    mcp_template = '{"mcpServers": {"github": {"type": "http", "url": "https://api.githubcopilot.com/mcp/"}}}'::jsonb,
    updated_at = now()
WHERE slug = 'github' AND workspace_id IS NULL;

UPDATE mcp_connector
SET description = 'Search, read, and update pages and databases in your Notion workspace. Sign in with Notion on first connect — no token to paste.',
    input_schema = '{"fields": []}'::jsonb,
    mcp_template = '{"mcpServers": {"notion": {"type": "http", "url": "https://mcp.notion.com/mcp"}}}'::jsonb,
    updated_at = now()
WHERE slug = 'notion' AND workspace_id IS NULL;

UPDATE mcp_connector
SET description = 'Read design context, components, and variables from your Figma files. Sign in with Figma on first connect — no token to paste.',
    input_schema = '{"fields": []}'::jsonb,
    mcp_template = '{"mcpServers": {"figma": {"type": "http", "url": "https://mcp.figma.com/mcp"}}}'::jsonb,
    updated_at = now()
WHERE slug = 'figma' AND workspace_id IS NULL;

UPDATE mcp_connector
SET description = 'Work with Jira issues and Confluence pages across your Atlassian Cloud site. Sign in with Atlassian on first connect — no token to paste.',
    input_schema = '{"fields": []}'::jsonb,
    mcp_template = '{"mcpServers": {"atlassian": {"type": "sse", "url": "https://mcp.atlassian.com/v1/sse"}}}'::jsonb,
    updated_at = now()
WHERE slug = 'atlassian' AND workspace_id IS NULL;
