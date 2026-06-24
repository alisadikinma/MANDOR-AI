package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/mcpoauth"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// ---------------------------------------------------------------------------
// MCP OAuth (in-app "Authenticate" for remote MCP servers)
// ---------------------------------------------------------------------------
//
// Some remote MCP servers (Figma, GitHub, …) require OAuth. Rather than make
// the user run a CLI on the runtime host, MANDOR acts as the OAuth client: the
// frontend opens an authorize URL, the user signs in at the provider, the
// provider redirects back to our callback, and we store the issued token sealed
// at rest. Phase 4 injects it as an Authorization: Bearer header into the
// effective mcp_config forwarded to the runtime, so the runtime CLI never does
// its own OAuth.
//
// Flow:
//   POST /api/agents/{id}/mcp/oauth/start  -> {authorize_url}
//   GET  /api/mcp/oauth/callback?code&state -> HTML that messages the opener
//
// The (workspace, resource) pair scopes the token: whoever authenticates is
// already a secret-viewer/admin and every agent in the workspace reuses it.

type startMcpOauthRequest struct {
	// Server is the mcpServers key to authenticate.
	Server string `json:"server"`
	// Origin is the browser origin (window.location.origin) used to build the
	// redirect URI; the provider redirects there and the FE proxy forwards
	// /api/* to this backend, carrying the session cookie.
	Origin string `json:"origin"`
}

// runtimePoolServerURL resolves a server's remote endpoint from a runtime's
// reported MCP pool (reported_mcp_servers JSON). Empty for stdio or unknown
// servers — only remote http servers are OAuth-capable.
func runtimePoolServerURL(reportedPool []byte, name string) string {
	if len(reportedPool) == 0 {
		return ""
	}
	var pool []protocol.McpServerInfo
	if err := json.Unmarshal(reportedPool, &pool); err != nil {
		return ""
	}
	for _, s := range pool {
		if s.Name == name {
			return strings.TrimSpace(s.URL)
		}
	}
	return ""
}

// InitiateRuntimeMcpOauth begins an OAuth authorization for one server in a
// runtime's machine MCP pool. The issued token is stored per (workspace,
// resource), so every agent on the runtime reuses it. Gated to workspace
// owner/admin (it writes a shared credential). Returns the authorize URL for
// the FE to open in a popup.
func (h *Handler) InitiateRuntimeMcpOauth(w http.ResponseWriter, r *http.Request) {
	if h.McpOAuthBox == nil {
		writeError(w, http.StatusBadRequest, "MCP OAuth is not configured on this server (set MULTICA_MCP_SECRET_KEY)")
		return
	}
	runtimeUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "runtimeId"), "runtimeId")
	if !ok {
		return
	}
	rt, err := h.Queries.GetAgentRuntime(r.Context(), runtimeUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "runtime not found")
		return
	}
	member, ok := h.requireWorkspaceMember(w, r, uuidToString(rt.WorkspaceID), "runtime not found")
	if !ok {
		return
	}
	if !roleAllowed(member.Role, "owner", "admin") {
		writeError(w, http.StatusForbidden, "you are not allowed to authenticate this runtime's MCP servers")
		return
	}

	var body startMcpOauthRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	body.Server = strings.TrimSpace(body.Server)
	if body.Server == "" {
		writeError(w, http.StatusBadRequest, "server is required")
		return
	}
	origin, ok := normalizeOAuthOrigin(body.Origin)
	if !ok {
		writeError(w, http.StatusBadRequest, "origin must be a valid http(s) URL")
		return
	}

	resourceURL := runtimePoolServerURL(rt.ReportedMcpServers, body.Server)
	if resourceURL == "" {
		writeError(w, http.StatusBadRequest, "server has no URL to authenticate (only remote http MCP servers support OAuth)")
		return
	}

	redirectURI := origin + "/api/mcp/oauth/callback"
	disc, err := mcpoauth.Discover(r.Context(), resourceURL)
	if err != nil {
		slog.Warn("mcp oauth discovery failed", "server", body.Server, "resource", resourceURL, "err", err)
		writeError(w, http.StatusBadGateway, "could not discover OAuth configuration for this server")
		return
	}
	client, err := mcpoauth.Register(r.Context(), disc.Server, redirectURI, "MANDOR (Multica)")
	if err != nil {
		slog.Warn("mcp oauth client registration failed", "server", body.Server, "err", err)
		writeError(w, http.StatusBadGateway, "could not register an OAuth client with this server")
		return
	}
	pkce, err := mcpoauth.NewPKCE()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start authorization")
		return
	}
	state, err := mcpoauth.RandomState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start authorization")
		return
	}

	h.McpOAuthFlows.Put(state, mcpoauth.Flow{
		WorkspaceID: uuidToString(rt.WorkspaceID),
		Resource:    resourceURL,
		RedirectURI: redirectURI,
		CreatedBy:   requestUserID(r),
		PKCE:        pkce,
		Client:      client,
		Discovery:   disc,
	})

	writeJSON(w, http.StatusOK, map[string]string{
		"authorize_url": mcpoauth.AuthorizeURL(disc, client, redirectURI, state, pkce),
	})
}

// CompleteMcpOauth is the OAuth redirect target. It redeems the code, seals the
// token, and persists it for the flow's workspace+resource, then returns HTML
// that notifies the opener window and closes the popup.
func (h *Handler) CompleteMcpOauth(w http.ResponseWriter, r *http.Request) {
	if h.McpOAuthBox == nil {
		writeOAuthResult(w, false, "MCP OAuth is not configured on this server")
		return
	}
	q := r.URL.Query()
	if oauthErr := q.Get("error"); oauthErr != "" {
		writeOAuthResult(w, false, "authorization was denied: "+oauthErr)
		return
	}
	code := q.Get("code")
	state := q.Get("state")
	if code == "" || state == "" {
		writeOAuthResult(w, false, "missing authorization code or state")
		return
	}
	flow, ok := h.McpOAuthFlows.Take(state)
	if !ok {
		writeOAuthResult(w, false, "this authorization expired or was already used — please try again")
		return
	}
	// The callback runs inside the auth group, so the browser carries the
	// session cookie. Bind completion to the user who started the flow: a
	// leaked single-use state can't be redeemed from a different session.
	if flow.CreatedBy != "" && flow.CreatedBy != requestUserID(r) {
		writeOAuthResult(w, false, "this authorization was started by a different session")
		return
	}

	token, err := mcpoauth.Exchange(r.Context(), flow.Discovery, flow.Client, flow.RedirectURI, code, flow.PKCE)
	if err != nil {
		slog.Warn("mcp oauth token exchange failed", "resource", flow.Resource, "err", err)
		writeOAuthResult(w, false, "could not complete sign-in with the provider")
		return
	}

	accessEnc, err := h.McpOAuthBox.Seal([]byte(token.AccessToken))
	if err != nil {
		writeOAuthResult(w, false, "failed to store the token")
		return
	}
	var refreshEnc, clientSecretEnc []byte
	if token.RefreshToken != "" {
		if refreshEnc, err = h.McpOAuthBox.Seal([]byte(token.RefreshToken)); err != nil {
			writeOAuthResult(w, false, "failed to store the token")
			return
		}
	}
	if flow.Client.ClientSecret != "" {
		if clientSecretEnc, err = h.McpOAuthBox.Seal([]byte(flow.Client.ClientSecret)); err != nil {
			writeOAuthResult(w, false, "failed to store the token")
			return
		}
	}

	wsUUID, err := util.ParseUUID(flow.WorkspaceID)
	if err != nil {
		writeOAuthResult(w, false, "invalid workspace")
		return
	}
	createdByUUID, _ := util.ParseUUID(flow.CreatedBy)

	if _, err := h.Queries.UpsertMcpOauthToken(r.Context(), db.UpsertMcpOauthTokenParams{
		WorkspaceID:         wsUUID,
		Resource:            flow.Resource,
		AuthorizationServer: flow.Discovery.Server.Issuer,
		Scope:               flow.Discovery.Scope,
		ClientID:            flow.Client.ClientID,
		ClientSecretEnc:     clientSecretEnc,
		AccessTokenEnc:      accessEnc,
		RefreshTokenEnc:     refreshEnc,
		ExpiresAt:           pgtype.Timestamptz{Time: token.ExpiresAt, Valid: !token.ExpiresAt.IsZero()},
		CreatedBy:           createdByUUID,
	}); err != nil {
		slog.Error("mcp oauth token upsert failed", "resource", flow.Resource, "err", err)
		writeOAuthResult(w, false, "failed to store the token")
		return
	}

	writeOAuthResult(w, true, "")
}

type setMcpTokenRequest struct {
	// Server is the mcpServers key to attach the token to.
	Server string `json:"server"`
	// Token is the user-supplied access token (e.g. a GitHub PAT).
	Token string `json:"token"`
}

// SetMcpAccessToken stores a user-supplied access token for one remote MCP
// server, the manual fallback for providers whose OAuth server doesn't support
// dynamic client registration (e.g. GitHub) so the in-app Authenticate flow
// can't run. The token is sealed at rest and injected as `Authorization:
// Bearer <token>` exactly like an OAuth-obtained one — same (workspace,
// resource) keying, no refresh token, no expiry.
func (h *Handler) SetMcpAccessToken(w http.ResponseWriter, r *http.Request) {
	if h.McpOAuthBox == nil {
		writeError(w, http.StatusBadRequest, "MCP token storage is not configured on this server (set MULTICA_MCP_SECRET_KEY)")
		return
	}
	runtimeUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "runtimeId"), "runtimeId")
	if !ok {
		return
	}
	rt, err := h.Queries.GetAgentRuntime(r.Context(), runtimeUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "runtime not found")
		return
	}
	member, ok := h.requireWorkspaceMember(w, r, uuidToString(rt.WorkspaceID), "runtime not found")
	if !ok {
		return
	}
	if !roleAllowed(member.Role, "owner", "admin") {
		writeError(w, http.StatusForbidden, "you are not allowed to set tokens for this runtime's MCP servers")
		return
	}

	var body setMcpTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	body.Server = strings.TrimSpace(body.Server)
	body.Token = strings.TrimSpace(body.Token)
	if body.Server == "" || body.Token == "" {
		writeError(w, http.StatusBadRequest, "server and token are required")
		return
	}

	resourceURL := runtimePoolServerURL(rt.ReportedMcpServers, body.Server)
	if resourceURL == "" {
		writeError(w, http.StatusBadRequest, "server has no URL (only remote http MCP servers accept an access token)")
		return
	}

	accessEnc, err := h.McpOAuthBox.Seal([]byte(body.Token))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store the token")
		return
	}

	createdBy, _ := util.ParseUUID(requestUserID(r))
	if _, err := h.Queries.UpsertMcpOauthToken(r.Context(), db.UpsertMcpOauthTokenParams{
		WorkspaceID:    rt.WorkspaceID,
		Resource:       resourceURL,
		AccessTokenEnc: accessEnc,
		CreatedBy:      createdBy,
	}); err != nil {
		slog.Error("mcp access token upsert failed", "resource", resourceURL, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to store the token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// normalizeOAuthOrigin validates and normalizes a browser origin into a bare
// scheme://host[:port] string (no trailing slash, no path).
func normalizeOAuthOrigin(origin string) (string, bool) {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return "", false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	return u.Scheme + "://" + u.Host, true
}

// mcpServerURL extracts the remote endpoint of a named server from an effective
// mcp_config, accepting the three URL spellings the prober understands. Empty
// means stdio-only (not OAuth-capable) or unknown server.
func mcpServerURL(effective json.RawMessage, name string) string {
	var cfg struct {
		McpServers map[string]struct {
			URL       string `json:"url"`
			HTTPURL   string `json:"httpUrl"`
			ServerURL string `json:"serverUrl"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(effective, &cfg); err != nil {
		return ""
	}
	e := cfg.McpServers[name]
	for _, u := range []string{e.URL, e.HTTPURL, e.ServerURL} {
		if strings.TrimSpace(u) != "" {
			return strings.TrimSpace(u)
		}
	}
	return ""
}

// writeOAuthResult renders the popup-closing page. It posts a typed message to
// the opener so the FE can re-probe on success, then closes the window.
func writeOAuthResult(w http.ResponseWriter, success bool, errMsg string) {
	payload, _ := json.Marshal(map[string]any{
		"type":    "mcp-oauth-result",
		"success": success,
		"error":   errMsg,
	})
	heading := "Connected"
	if !success {
		heading = "Sign-in failed"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !success {
		w.WriteHeader(http.StatusBadRequest)
	}
	// The message is JSON-encoded above (HTML/JS-safe); origin "*" is acceptable
	// because the payload carries no secret — only success/error for the UI.
	body := `<!doctype html><meta charset="utf-8"><title>` + heading + `</title>` +
		`<body style="font:14px system-ui;padding:2rem;color:#0F172A"><p>` + heading + `. You can close this window.</p>` +
		`<script>try{window.opener&&window.opener.postMessage(` + string(payload) + `,"*")}catch(e){}window.close()</script>`
	_, _ = w.Write([]byte(body))
}
