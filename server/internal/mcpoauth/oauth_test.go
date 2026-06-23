package mcpoauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestParseResourceMetadataHeader(t *testing.T) {
	cases := map[string]string{
		`Bearer resource_metadata="https://mcp.figma.com/.well-known/oauth-protected-resource",scope="mcp:connect"`: "https://mcp.figma.com/.well-known/oauth-protected-resource",
		`resource_metadata="https://x/.well-known/oauth-protected-resource"`:                                        "https://x/.well-known/oauth-protected-resource",
		`Bearer realm="x"`: "",
		``:                 "",
	}
	for header, want := range cases {
		if got := parseResourceMetadataHeader(header); got != want {
			t.Errorf("header %q: got %q want %q", header, got, want)
		}
	}
}

func TestGuardDialBlocksInternal(t *testing.T) {
	blocked := []string{
		"127.0.0.1:443", "10.0.0.5:443", "192.168.1.1:80",
		"169.254.169.254:80", "[::1]:443", "0.0.0.0:80",
	}
	for _, addr := range blocked {
		if err := guardDial("tcp", addr, nil); err == nil {
			t.Errorf("guardDial(%q) = nil, want blocked", addr)
		}
	}
	if err := guardDial("tcp", "93.184.216.34:443", nil); err != nil {
		t.Errorf("guardDial(public) = %v, want allowed", err)
	}
	t.Setenv("MULTICA_MCP_OAUTH_ALLOW_PRIVATE", "1")
	if err := guardDial("tcp", "127.0.0.1:443", nil); err != nil {
		t.Errorf("guardDial(loopback, allow-private) = %v, want allowed", err)
	}
}

func TestNewPKCE(t *testing.T) {
	p, err := NewPKCE()
	if err != nil {
		t.Fatal(err)
	}
	if p.Verifier == "" || p.Challenge == "" || p.Verifier == p.Challenge {
		t.Fatalf("bad PKCE pair: %+v", p)
	}
}

func TestAuthorizeURL(t *testing.T) {
	d := Discovery{
		Resource: "https://mcp.figma.com/mcp",
		Scope:    "mcp:connect",
		Server:   ServerMetadata{AuthorizationEndpoint: "https://www.figma.com/oauth/mcp"},
	}
	u := AuthorizeURL(d, Client{ClientID: "cid"}, "https://app/cb", "st4te", PKCE{Challenge: "chal"})
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	for k, want := range map[string]string{
		"response_type": "code", "client_id": "cid", "redirect_uri": "https://app/cb",
		"state": "st4te", "code_challenge": "chal", "code_challenge_method": "S256",
		"resource": "https://mcp.figma.com/mcp", "scope": "mcp:connect",
	} {
		if q.Get(k) != want {
			t.Errorf("query %s: got %q want %q", k, q.Get(k), want)
		}
	}
}

// TestFullFlow drives discovery -> DCR -> exchange -> refresh against a fake
// authorization server, the way the handler layer will.
func TestFullFlow(t *testing.T) {
	// httptest binds 127.0.0.1, which the SSRF dial guard blocks by default.
	t.Setenv("MULTICA_MCP_OAUTH_ALLOW_PRIVATE", "1")
	mux := http.NewServeMux()
	var srv *httptest.Server

	// Resource probe: unauthenticated initialize -> 401 with metadata pointer.
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+srv.URL+`/.well-known/oauth-protected-resource"`)
		w.WriteHeader(http.StatusUnauthorized)
	})
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ResourceMetadata{
			Resource: srv.URL + "/mcp", AuthorizationServers: []string{srv.URL}, ScopesSupported: []string{"mcp:connect"},
		})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ServerMetadata{
			Issuer: srv.URL, AuthorizationEndpoint: srv.URL + "/authorize", TokenEndpoint: srv.URL + "/token",
			RegistrationEndpoint: srv.URL + "/register", CodeChallengeMethods: []string{"S256"},
		})
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["token_endpoint_auth_method"] != "none" {
			t.Errorf("expected public client, got %v", body["token_endpoint_auth_method"])
		}
		_ = json.NewEncoder(w).Encode(Client{ClientID: "dyn-client-1"})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code_verifier") == "" || r.Form.Get("client_id") != "dyn-client-1" {
				t.Errorf("bad exchange form: %v", r.Form)
			}
			_ = json.NewEncoder(w).Encode(tokenResponse{AccessToken: "at-1", RefreshToken: "rt-1", TokenType: "Bearer", ExpiresIn: 3600})
		case "refresh_token":
			if r.Form.Get("refresh_token") != "rt-1" {
				t.Errorf("bad refresh form: %v", r.Form)
			}
			_ = json.NewEncoder(w).Encode(tokenResponse{AccessToken: "at-2", RefreshToken: "rt-2", TokenType: "Bearer", ExpiresIn: 3600})
		default:
			http.Error(w, "bad grant", http.StatusBadRequest)
		}
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	ctx := context.Background()

	d, err := Discover(ctx, srv.URL+"/mcp")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if d.Scope != "mcp:connect" || d.Server.TokenEndpoint != srv.URL+"/token" {
		t.Fatalf("bad discovery: %+v", d)
	}

	client, err := Register(ctx, d.Server, "https://app/cb", "MANDOR")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if client.ClientID != "dyn-client-1" {
		t.Fatalf("bad client: %+v", client)
	}

	pkce, _ := NewPKCE()
	if !strings.Contains(AuthorizeURL(d, client, "https://app/cb", "st", pkce), "code_challenge=") {
		t.Fatal("authorize URL missing PKCE challenge")
	}

	tok, err := Exchange(ctx, d, client, "https://app/cb", "the-code", pkce)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if tok.AccessToken != "at-1" || tok.RefreshToken != "rt-1" || tok.ExpiresAt.IsZero() {
		t.Fatalf("bad token: %+v", tok)
	}

	refreshed, err := Refresh(ctx, d, client, tok.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if refreshed.AccessToken != "at-2" {
		t.Fatalf("bad refreshed token: %+v", refreshed)
	}
}
