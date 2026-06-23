# MCP OAuth from the FE (Approach A — MANDOR as OAuth client)

## Design

Today remote MCP servers (figma/github/notion/atlassian) are `type:http` with no
secret; auth is delegated to the runtime CLI on first connect. The FE shows
`Needs auth` with no way to act, and the Claude CLI has **no headless OAuth
command**, so the daemon can't broker it. Goal: authenticate 100% from the FE.

MANDOR becomes the OAuth client per the MCP auth spec (verified live against
Figma 2026-06-22):

- `POST mcp.figma.com/mcp` → `401 WWW-Authenticate: Bearer resource_metadata=…`
- protected-resource metadata → `authorization_servers: [api.figma.com]`, scope `mcp:connect`
- AS metadata → `registration_endpoint` (DCR ✅), PKCE ✅, authorize `www.figma.com/oauth/mcp`, token `api.figma.com/v1/oauth/token`

So: discover → dynamic client registration → PKCE auth-code → store token →
inject `Authorization: Bearer <token>` into the effective mcp_config forwarded to
the runtime. Generic + spec-compliant → same path works for any compliant MCP.

**Token scope (default, override if you object): workspace + resource.** The
person who clicks Authenticate is already a secret-viewer/admin; all agents in
the workspace reuse the token. Per-agent tokens deferred (YAGNI).

**Storage:** new table, tokens encrypted with existing `util/secretbox` (as Lark
does). Discovery/DCR results cached per AS. In-flight auth state (state +
PKCE verifier) kept in an in-memory store with TTL (mirrors `McpProbeStore`).

## Implementation Plan

### Phase 1 — backend OAuth core (`server/internal/mcpoauth/`)
- `discover(resourceURL)`: fetch protected-resource + AS metadata (cache).
- `register(as)`: DCR POST → client_id (+secret if issued). Cache per AS.
- `authorizeURL(...)`: build PKCE auth-code URL (state, code_challenge, scope, resource, redirect_uri).
- `exchange(code, verifier)` / `refresh(refreshToken)`: token endpoint calls.
- Self-check: `demo()` asserting authorizeURL contains state+challenge+resource.

### Phase 2 — persistence
- Migration `120_mcp_oauth_token`: `(id, workspace_id, resource, authorization_server, client_id, client_secret_enc, access_token_enc, refresh_token_enc, expires_at, scope, created_by, created_at, updated_at)`, unique `(workspace_id, resource)`.
- sqlc queries: upsert, get-by-(workspace,resource), list-by-workspace.
- In-memory `oauthFlowStore` for pending `(state → {verifier, workspace, resource, redirect})`, 10-min TTL.

### Phase 3 — endpoints (`mcp_oauth.go`) + router
- `POST /api/agents/{id}/mcp/oauth/start` body `{server}` → resolves server URL from effective config, runs discover+register, returns `{authorize_url}`. Gated to `canViewAgentSecrets`.
- (workspace variant `POST /api/workspaces/{id}/mcp/oauth/start`, admin-gated.)
- `GET /api/mcp/oauth/callback?code&state` → validate state, exchange, encrypt+store, return tiny HTML that `postMessage`s success and closes the popup. Parse, never trust (schema + fallbacks per CLAUDE.md).

### Phase 4 — inject token into effective config
- Helper `injectOAuthHeaders(cfg, workspaceID)`: for each server whose url matches a stored token resource, set `headers.Authorization = "Bearer "+token`; refresh if `expires_at` near.
- Call it in **both** sites that build effective config: `InitiateAgentMcpProbe` (so probe shows Connected) and daemon task dispatch (`ClaimTask` runtimeMcpConfig).

### Phase 5 — FE (`installed-connector-list.tsx` + core api)
- Replace the `needs_auth` help block with an **Authenticate** button.
- `api.startMcpOauth(agentId, server)` → open `authorize_url` in a popup; listen for `postMessage` success → auto re-probe. Schema-validated envelope; popup-blocked fallback = show the URL as a link.
- Keep a one-line hint for the manual fallback.

### Phase 6 — tests + verify
- Go: discovery/authorizeURL/exchange with a stubbed AS (httptest); callback with malformed body (fails closed); token injection matches by resource URL.
- `make check`; manual: FE button → Figma login → `Connected · N tools`.

## Data Integration Map
| Component | Data source | Existing? | Notes |
|---|---|---|---|
| discover/register | Figma AS metadata (live) | new | cached per AS |
| token store | `mcp_oauth_token` (new) | new | secretbox-encrypted |
| pending flow | in-memory store | new | mirrors McpProbeStore TTL |
| effective config | agent+workspace mcp_config | exists | inject Bearer header |
| FE Authenticate | start endpoint + popup | new | replaces needs_auth help text |
