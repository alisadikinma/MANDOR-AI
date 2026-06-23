package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/mcpoauth"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// mcpTokenRefreshWindow refreshes a token this long before it actually expires,
// so the runtime never receives an access token that dies mid-connection.
const mcpTokenRefreshWindow = 60 * time.Second

// injectMcpOauthHeaders adds an `Authorization: Bearer <token>` header to every
// server in the effective mcp_config whose URL matches a stored OAuth token for
// this workspace. This is how the tokens MANDOR obtained via the in-app
// Authenticate flow reach the runtime — the runtime CLI never does its own
// OAuth. Tokens near expiry are refreshed first. Best-effort: any failure
// leaves the config unchanged rather than blocking the probe / task dispatch.
func (h *Handler) injectMcpOauthHeaders(ctx context.Context, wsID pgtype.UUID, effective json.RawMessage) json.RawMessage {
	if h.McpOAuthBox == nil || len(effective) == 0 {
		return effective
	}
	tokens, err := h.Queries.ListMcpOauthTokensByWorkspace(ctx, wsID)
	if err != nil || len(tokens) == 0 {
		return effective
	}

	bearer := make(map[string]string, len(tokens))
	for _, tok := range tokens {
		if access := h.resolveMcpAccessToken(ctx, tok); access != "" {
			bearer[normalizeResource(tok.Resource)] = access
		}
	}
	return applyBearerHeaders(effective, bearer)
}

// applyBearerHeaders is the pure config transform: for each server whose URL
// matches a resource in bearer, add `Authorization: Bearer <token>` unless the
// user already set an Authorization header. Returns the input unchanged on any
// parse failure or when nothing matched.
func applyBearerHeaders(effective json.RawMessage, bearer map[string]string) json.RawMessage {
	if len(bearer) == 0 || len(effective) == 0 {
		return effective
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(effective, &root); err != nil {
		return effective
	}
	serversRaw, ok := root["mcpServers"]
	if !ok {
		return effective
	}
	var servers map[string]map[string]json.RawMessage
	if err := json.Unmarshal(serversRaw, &servers); err != nil {
		return effective
	}

	changed := false
	for name, entry := range servers {
		url := entryURL(entry)
		if url == "" {
			continue
		}
		access, ok := bearer[normalizeResource(url)]
		if !ok {
			continue
		}
		headers := map[string]string{}
		if raw, ok := entry["headers"]; ok {
			_ = json.Unmarshal(raw, &headers)
		}
		// Respect an explicit Authorization the user set themselves.
		if _, exists := headers["Authorization"]; exists {
			continue
		}
		headers["Authorization"] = "Bearer " + access
		if hb, err := json.Marshal(headers); err == nil {
			entry["headers"] = hb
			servers[name] = entry
			changed = true
		}
	}
	if !changed {
		return effective
	}
	if sb, err := json.Marshal(servers); err == nil {
		root["mcpServers"] = sb
	}
	if out, err := json.Marshal(root); err == nil {
		return out
	}
	return effective
}

// resolveMcpAccessToken returns the usable access token for a row, refreshing it
// first when it is within mcpTokenRefreshWindow of expiry and a refresh token is
// present. Returns "" when the token can't be decrypted (e.g. key rotated).
func (h *Handler) resolveMcpAccessToken(ctx context.Context, tok db.McpOauthToken) string {
	if tok.ExpiresAt.Valid && time.Until(tok.ExpiresAt.Time) < mcpTokenRefreshWindow && len(tok.RefreshTokenEnc) > 0 {
		if access := h.refreshMcpToken(ctx, tok); access != "" {
			return access
		}
		// Refresh failed — fall through to the (possibly expired) access token;
		// the runtime gets one last chance and a clear 401 if it's truly dead.
	}
	plain, err := h.McpOAuthBox.Open(tok.AccessTokenEnc)
	if err != nil {
		slog.Warn("mcp oauth: failed to open access token", "resource", tok.Resource, "err", err)
		return ""
	}
	return string(plain)
}

// refreshMcpToken runs the refresh_token grant and persists the rotated token.
// Re-discovers the token endpoint from the resource (we store the issuer, not
// the endpoint) — refresh is infrequent, so the extra round trip is cheap.
func (h *Handler) refreshMcpToken(ctx context.Context, tok db.McpOauthToken) string {
	refreshPlain, err := h.McpOAuthBox.Open(tok.RefreshTokenEnc)
	if err != nil {
		return ""
	}
	disc, err := mcpoauth.Discover(ctx, tok.Resource)
	if err != nil {
		slog.Warn("mcp oauth: refresh discovery failed", "resource", tok.Resource, "err", err)
		return ""
	}
	client := mcpoauth.Client{ClientID: tok.ClientID}
	if len(tok.ClientSecretEnc) > 0 {
		if cs, err := h.McpOAuthBox.Open(tok.ClientSecretEnc); err == nil {
			client.ClientSecret = string(cs)
		}
	}
	newTok, err := mcpoauth.Refresh(ctx, disc, client, string(refreshPlain))
	if err != nil {
		slog.Warn("mcp oauth: token refresh failed", "resource", tok.Resource, "err", err)
		return ""
	}
	accessEnc, err := h.McpOAuthBox.Seal([]byte(newTok.AccessToken))
	if err != nil {
		return ""
	}
	refreshEnc := tok.RefreshTokenEnc // keep prior refresh token if none rotated in
	if newTok.RefreshToken != "" {
		if enc, err := h.McpOAuthBox.Seal([]byte(newTok.RefreshToken)); err == nil {
			refreshEnc = enc
		}
	}
	if _, err := h.Queries.UpdateMcpOauthTokenAfterRefresh(ctx, db.UpdateMcpOauthTokenAfterRefreshParams{
		ID:              tok.ID,
		AccessTokenEnc:  accessEnc,
		RefreshTokenEnc: refreshEnc,
		ExpiresAt:       pgtype.Timestamptz{Time: newTok.ExpiresAt, Valid: !newTok.ExpiresAt.IsZero()},
	}); err != nil {
		slog.Warn("mcp oauth: persist refreshed token failed", "resource", tok.Resource, "err", err)
		// The new access token is still valid in memory even if persistence
		// failed; use it for this dispatch.
	}
	return newTok.AccessToken
}

// entryURL reads the remote endpoint from one mcpServers entry, accepting the
// three URL spellings. Empty for stdio servers.
func entryURL(entry map[string]json.RawMessage) string {
	for _, key := range []string{"url", "httpUrl", "serverUrl"} {
		if raw, ok := entry[key]; ok {
			var s string
			if json.Unmarshal(raw, &s) == nil && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// normalizeResource canonicalizes a resource/server URL for matching: trimmed
// and without a trailing slash. (A token stored for ".../mcp" matches a config
// entry written as ".../mcp/".)
func normalizeResource(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), "/")
}
