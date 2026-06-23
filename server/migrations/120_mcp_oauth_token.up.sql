-- OAuth tokens MANDOR obtains on the user's behalf for remote MCP servers
-- (Figma, GitHub, …) via the in-app "Authenticate" flow. The token is injected
-- as an Authorization: Bearer header into the effective mcp_config forwarded to
-- the runtime, so the runtime CLI never performs its own OAuth.
--
-- Scope is (workspace_id, resource): whoever authenticates is already a
-- secret-viewer/admin, and every agent in the workspace reuses the token.
--
-- Secret material (access/refresh token, any issued client_secret) is stored
-- sealed with util/secretbox (key MULTICA_MCP_SECRET_KEY), unlike the plaintext
-- mcp_config columns — these are long-lived, refreshable credentials.
CREATE TABLE mcp_oauth_token (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id         uuid NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    resource             text NOT NULL,
    authorization_server text NOT NULL,
    scope                text NOT NULL DEFAULT '',
    client_id            text NOT NULL,
    client_secret_enc    bytea,
    access_token_enc     bytea NOT NULL,
    refresh_token_enc    bytea,
    expires_at           timestamptz,
    created_by           uuid,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

-- One token per (workspace, MCP resource); the auth flow upserts on this key.
CREATE UNIQUE INDEX idx_mcp_oauth_token_ws_resource
    ON mcp_oauth_token (workspace_id, resource);
