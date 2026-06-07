-- MCP connector directory. A visual marketplace layered over the existing
-- per-agent mcp_config JSONB column. Rows are either:
--   * GLOBAL curated connectors (workspace_id IS NULL) seeded from
--     server/internal/handler/mcp_connectors_seed.json, OR
--   * WORKSPACE-CUSTOM connectors authored by an admin (workspace_id set).
--
-- Adding a connector to an agent renders mcp_template against the user's
-- input_schema answers and DEEP-MERGES the result into the agent's existing
-- mcp_config.mcpServers — it never replaces the raw config.
CREATE TABLE mcp_connector (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  uuid REFERENCES workspace(id) ON DELETE CASCADE,
    slug          text NOT NULL,
    name          text NOT NULL,
    icon          text,
    description   text,
    popularity    int NOT NULL DEFAULT 0,
    input_schema  jsonb NOT NULL DEFAULT '{}'::jsonb,
    mcp_template  jsonb NOT NULL,
    created_by    uuid,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Unique slug per scope. NULL workspace_id (global) collapses to a single
-- partial unique index because NULLs are distinct under a plain UNIQUE
-- constraint (so two global "github" rows would otherwise be allowed).
CREATE UNIQUE INDEX idx_mcp_connector_workspace_slug
    ON mcp_connector (workspace_id, slug)
    WHERE workspace_id IS NOT NULL;

CREATE UNIQUE INDEX idx_mcp_connector_global_slug
    ON mcp_connector (slug)
    WHERE workspace_id IS NULL;

-- List query filters on (workspace_id IS NULL OR workspace_id = $1).
CREATE INDEX idx_mcp_connector_workspace ON mcp_connector (workspace_id);
