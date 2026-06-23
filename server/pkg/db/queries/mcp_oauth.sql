-- name: UpsertMcpOauthToken :one
-- Stores (or replaces) the OAuth token for one (workspace, MCP resource).
-- Re-authenticating the same server overwrites the prior token.
INSERT INTO mcp_oauth_token (
    workspace_id, resource, authorization_server, scope,
    client_id, client_secret_enc, access_token_enc, refresh_token_enc,
    expires_at, created_by
) VALUES (
    @workspace_id, @resource, @authorization_server, @scope,
    @client_id, @client_secret_enc, @access_token_enc, @refresh_token_enc,
    @expires_at, @created_by
)
ON CONFLICT (workspace_id, resource) DO UPDATE SET
    authorization_server = EXCLUDED.authorization_server,
    scope                = EXCLUDED.scope,
    client_id            = EXCLUDED.client_id,
    client_secret_enc    = EXCLUDED.client_secret_enc,
    access_token_enc     = EXCLUDED.access_token_enc,
    refresh_token_enc    = EXCLUDED.refresh_token_enc,
    expires_at           = EXCLUDED.expires_at,
    updated_at           = now()
RETURNING *;

-- name: GetMcpOauthToken :one
SELECT * FROM mcp_oauth_token
WHERE workspace_id = @workspace_id AND resource = @resource;

-- name: ListMcpOauthTokensByWorkspace :many
-- All tokens for a workspace — used to inject Bearer headers into the
-- effective mcp_config (match each server URL against a token resource).
SELECT * FROM mcp_oauth_token
WHERE workspace_id = @workspace_id;

-- name: UpdateMcpOauthTokenAfterRefresh :one
-- Persists rotated access/refresh tokens after a refresh_token grant.
UPDATE mcp_oauth_token
SET access_token_enc  = @access_token_enc,
    refresh_token_enc = @refresh_token_enc,
    expires_at        = @expires_at,
    updated_at        = now()
WHERE id = @id
RETURNING *;

-- name: DeleteMcpOauthToken :exec
DELETE FROM mcp_oauth_token
WHERE workspace_id = @workspace_id AND resource = @resource;
