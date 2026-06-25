// Package mcpoauth makes MANDOR an OAuth 2.1 client for remote MCP servers
// (Figma, GitHub, Notion, …) so the control plane can authenticate a server on
// the user's behalf and inject the resulting bearer token into the effective
// mcp_config forwarded to the runtime — no CLI-side OAuth, no token to paste.
//
// The flow follows the MCP authorization spec: discover the protected-resource
// metadata, discover the authorization-server metadata, dynamically register a
// client (RFC 7591), run a PKCE authorization-code grant, then exchange/refresh
// tokens at the token endpoint. Everything here is transport (HTTP + JSON);
// persistence and the HTTP endpoints live in the handler/db layers.
package mcpoauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
)

// httpTimeout bounds every metadata/registration/token call. These are quick
// JSON round-trips against the provider's auth server.
const httpTimeout = 15 * time.Second

// ResourceMetadata is the subset of RFC 9728 protected-resource metadata we use.
type ResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
	ScopesSupported      []string `json:"scopes_supported"`
}

// ServerMetadata is the subset of RFC 8414 authorization-server metadata we use.
type ServerMetadata struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	RegistrationEndpoint  string   `json:"registration_endpoint"`
	ScopesSupported       []string `json:"scopes_supported"`
	CodeChallengeMethods  []string `json:"code_challenge_methods_supported"`
}

// Discovery is the resolved endpoint set for one MCP resource: which scopes to
// request, where to register, authorize, and redeem codes.
type Discovery struct {
	Resource string
	Scope    string
	Server   ServerMetadata
}

// Client is a registered (or static) OAuth client for one authorization server.
type Client struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
}

// Token is the credential set returned by the token endpoint.
type Token struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	Scope        string
	ExpiresAt    time.Time // zero when the server returns no expires_in
}

// PKCE holds a generated verifier/challenge pair for one authorization request.
type PKCE struct {
	Verifier  string
	Challenge string
}

// httpClient returns the client used for every metadata/registration/token
// call. Its dialer rejects private, loopback, and link-local targets so a
// workspace admin's mcp_config URL (or a malicious server's resource_metadata
// hint) can't turn these server-side fetches into an SSRF against internal
// services or the cloud metadata endpoint (169.254.169.254). The guard runs on
// the resolved IP, so DNS rebinding is covered too. Set
// MULTICA_MCP_OAUTH_ALLOW_PRIVATE=1 to allow private targets for local dev.
func httpClient() *http.Client {
	d := &net.Dialer{Timeout: httpTimeout, Control: guardDial}
	return &http.Client{
		Timeout:   httpTimeout,
		Transport: &http.Transport{DialContext: d.DialContext},
	}
}

func guardDial(network, address string, _ syscall.RawConn) error {
	if os.Getenv("MULTICA_MCP_OAUTH_ALLOW_PRIVATE") == "1" {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("blocked: could not resolve %q to an IP", host)
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("blocked private/internal address %s", ip)
	}
	return nil
}

// Discover resolves the OAuth endpoints for an MCP resource URL. It reads the
// protected-resource metadata (preferring the URL advertised in the server's
// 401 WWW-Authenticate header, falling back to the well-known path), then the
// authorization-server metadata. The chosen scope is the resource's
// scopes_supported joined, falling back to the AS scopes.
func Discover(ctx context.Context, resourceURL string) (Discovery, error) {
	rm, err := fetchResourceMetadata(ctx, resourceURL)
	if err != nil {
		return Discovery{}, err
	}
	if len(rm.AuthorizationServers) == 0 {
		return Discovery{}, fmt.Errorf("no authorization_servers advertised by %s", resourceURL)
	}
	sm, err := fetchServerMetadata(ctx, rm.AuthorizationServers[0])
	if err != nil {
		return Discovery{}, err
	}
	scope := strings.Join(rm.ScopesSupported, " ")
	if scope == "" {
		scope = strings.Join(sm.ScopesSupported, " ")
	}
	resource := rm.Resource
	if resource == "" {
		resource = resourceURL
	}
	return Discovery{Resource: resource, Scope: scope, Server: sm}, nil
}

// fetchResourceMetadata probes the resource for its 401 WWW-Authenticate
// resource_metadata pointer, then GETs that document. If the probe yields no
// pointer it falls back to the conventional well-known path on the resource's
// origin.
func fetchResourceMetadata(ctx context.Context, resourceURL string) (ResourceMetadata, error) {
	metaURL := resourceMetadataURL(ctx, resourceURL)
	var rm ResourceMetadata
	if err := getJSON(ctx, metaURL, &rm); err != nil {
		return ResourceMetadata{}, fmt.Errorf("resource metadata: %w", err)
	}
	return rm, nil
}

// resourceMetadataURL returns the protected-resource metadata URL, preferring
// the one advertised in the unauthenticated probe's WWW-Authenticate header.
func resourceMetadataURL(ctx context.Context, resourceURL string) string {
	if adv := probeResourceMetadataHint(ctx, resourceURL); adv != "" {
		return adv
	}
	if u, err := url.Parse(resourceURL); err == nil {
		u.Path = "/.well-known/oauth-protected-resource"
		u.RawQuery = ""
		return u.String()
	}
	return strings.TrimRight(resourceURL, "/") + "/.well-known/oauth-protected-resource"
}

// probeResourceMetadataHint sends an unauthenticated MCP initialize and parses
// resource_metadata="…" out of a 401's WWW-Authenticate header. Best-effort:
// any failure returns "" so the caller falls back to the well-known path.
func probeResourceMetadataHint(ctx context.Context, resourceURL string) string {
	body := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"mandor-oauth","version":"1"}}}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resourceURL, body)
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := httpClient().Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	return parseResourceMetadataHeader(resp.Header.Get("WWW-Authenticate"))
}

// parseResourceMetadataHeader extracts the resource_metadata="…" value from a
// WWW-Authenticate header value. Split out for testability.
func parseResourceMetadataHeader(h string) string {
	for _, part := range strings.Split(h, ",") {
		part = strings.TrimSpace(part)
		if v, ok := strings.CutPrefix(part, "resource_metadata="); ok {
			return strings.Trim(v, `"`)
		}
		// Some servers prefix the first param with the scheme: Bearer resource_metadata="…"
		if i := strings.Index(part, "resource_metadata="); i >= 0 {
			return strings.Trim(part[i+len("resource_metadata="):], `"`)
		}
	}
	return ""
}

// fetchServerMetadata GETs RFC 8414 metadata for an authorization server,
// trying the oauth-authorization-server well-known then openid-configuration.
func fetchServerMetadata(ctx context.Context, issuer string) (ServerMetadata, error) {
	base := strings.TrimRight(issuer, "/")
	var lastErr error
	for _, suffix := range []string{"/.well-known/oauth-authorization-server", "/.well-known/openid-configuration"} {
		var sm ServerMetadata
		if err := getJSON(ctx, base+suffix, &sm); err == nil && sm.AuthorizationEndpoint != "" && sm.TokenEndpoint != "" {
			return sm, nil
		} else if err != nil {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("incomplete authorization-server metadata at %s", issuer)
	}
	return ServerMetadata{}, fmt.Errorf("server metadata: %w", lastErr)
}

// ErrRegistrationNotPermitted means the authorization server won't let us
// register an OAuth client automatically — either it advertises no registration
// endpoint, or it rejects anonymous registration (e.g. Figma requires the client
// to be allowlisted in its MCP Catalog, returning 401/403). This is a permanent,
// server-side condition, not a transient failure: in-app OAuth can't proceed.
var ErrRegistrationNotPermitted = fmt.Errorf("server does not allow third-party OAuth clients to register")

// Register performs RFC 7591 dynamic client registration as a public PKCE
// client (token_endpoint_auth_method=none) with the given redirect URI. If the
// AS advertises no registration endpoint the caller must supply a pre-registered
// client instead.
func Register(ctx context.Context, sm ServerMetadata, redirectURI, clientName string) (Client, error) {
	if strings.TrimSpace(sm.RegistrationEndpoint) == "" {
		return Client{}, ErrRegistrationNotPermitted
	}
	reqBody, _ := json.Marshal(map[string]any{
		"client_name":                clientName,
		"redirect_uris":              []string{redirectURI},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sm.RegistrationEndpoint, strings.NewReader(string(reqBody)))
	if err != nil {
		return Client{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient().Do(req)
	if err != nil {
		return Client{}, fmt.Errorf("client registration: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return Client{}, ErrRegistrationNotPermitted
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Client{}, fmt.Errorf("client registration: http %d", resp.StatusCode)
	}
	var c Client
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		return Client{}, fmt.Errorf("client registration: decode: %w", err)
	}
	if c.ClientID == "" {
		return Client{}, fmt.Errorf("client registration: no client_id returned")
	}
	return c, nil
}

// NewPKCE generates a high-entropy verifier and its S256 challenge.
func NewPKCE() (PKCE, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return PKCE{}, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	return PKCE{Verifier: verifier, Challenge: base64.RawURLEncoding.EncodeToString(sum[:])}, nil
}

// RandomState returns a URL-safe opaque CSRF state value.
func RandomState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// AuthorizeURL builds the authorization-code request URL with PKCE. The resource
// parameter (RFC 8707) scopes the token to this specific MCP server.
func AuthorizeURL(d Discovery, client Client, redirectURI, state string, pkce PKCE) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", client.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	q.Set("code_challenge", pkce.Challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("resource", d.Resource)
	if d.Scope != "" {
		q.Set("scope", d.Scope)
	}
	sep := "?"
	if strings.Contains(d.Server.AuthorizationEndpoint, "?") {
		sep = "&"
	}
	return d.Server.AuthorizationEndpoint + sep + q.Encode()
}

// Exchange redeems an authorization code for tokens (PKCE, public client).
func Exchange(ctx context.Context, d Discovery, client Client, redirectURI, code string, pkce PKCE) (Token, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", client.ClientID)
	form.Set("code_verifier", pkce.Verifier)
	form.Set("resource", d.Resource)
	return postToken(ctx, d.Server.TokenEndpoint, client, form)
}

// Refresh exchanges a refresh token for a fresh access token.
func Refresh(ctx context.Context, d Discovery, client Client, refreshToken string) (Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", client.ClientID)
	form.Set("resource", d.Resource)
	if d.Scope != "" {
		form.Set("scope", d.Scope)
	}
	return postToken(ctx, d.Server.TokenEndpoint, client, form)
}

// tokenResponse is the token endpoint's JSON body.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int64  `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func postToken(ctx context.Context, tokenEndpoint string, client Client, form url.Values) (Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	// Confidential clients (a client_secret was issued) authenticate via Basic.
	if client.ClientSecret != "" {
		req.SetBasicAuth(url.QueryEscape(client.ClientID), url.QueryEscape(client.ClientSecret))
	}
	resp, err := httpClient().Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("token endpoint: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return Token{}, fmt.Errorf("token endpoint: http %d: unparseable body", resp.StatusCode)
	}
	if tr.Error != "" {
		return Token{}, fmt.Errorf("token endpoint: %s: %s", tr.Error, tr.ErrorDesc)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || tr.AccessToken == "" {
		return Token{}, fmt.Errorf("token endpoint: http %d", resp.StatusCode)
	}
	tok := Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		TokenType:    tr.TokenType,
		Scope:        tr.Scope,
	}
	if tr.ExpiresIn > 0 {
		tok.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return tok, nil
}

// getJSON GETs a URL and decodes a JSON body, enforcing a 2xx status.
func getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s: http %d", u, resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return json.Unmarshal(body, out)
}
