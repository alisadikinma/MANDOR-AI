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
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// mcpTokenRefreshWindow refreshes a token this long before it actually expires,
// so the runtime never receives an access token that dies mid-connection.
const mcpTokenRefreshWindow = 60 * time.Second

// mcpOauthHeadersByServerName resolves the workspace's stored OAuth/manual
// tokens to a map of pool-server-name → access token, by matching each token's
// resource to a server URL in the runtime's reported pool. The daemon receives
// this (instead of an assembled config) and injects the bearer into its own
// machine config by name. Returns nil when nothing is configured or matched.
func (h *Handler) mcpOauthHeadersByServerName(ctx context.Context, wsID pgtype.UUID, reportedPool []byte) map[string]string {
	if h.McpOAuthBox == nil || len(reportedPool) == 0 {
		return nil
	}
	tokens, err := h.Queries.ListMcpOauthTokensByWorkspace(ctx, wsID)
	if err != nil || len(tokens) == 0 {
		return nil
	}
	byResource := make(map[string]string, len(tokens))
	for _, tok := range tokens {
		if access := h.resolveMcpAccessToken(ctx, tok); access != "" {
			byResource[normalizeResource(tok.Resource)] = access
		}
	}
	var pool []protocol.McpServerInfo
	if err := json.Unmarshal(reportedPool, &pool); err != nil {
		return nil
	}
	out := map[string]string{}
	for _, s := range pool {
		if s.URL == "" {
			continue
		}
		if access, ok := byResource[normalizeResource(s.URL)]; ok {
			out[s.Name] = access
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

// normalizeResource canonicalizes a resource/server URL for matching: trimmed
// and without a trailing slash. (A token stored for ".../mcp" matches a config
// entry written as ".../mcp/".)
func normalizeResource(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), "/")
}
