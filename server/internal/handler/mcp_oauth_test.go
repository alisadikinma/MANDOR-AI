package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/mcpoauth"
	"github.com/multica-ai/multica/server/internal/util/secretbox"
)

func testBox(t *testing.T) *secretbox.Box {
	t.Helper()
	box, err := secretbox.New(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return box
}

func TestInitiateRuntimeMcpOauthNotConfigured(t *testing.T) {
	// McpOAuthBox nil → 400 before any DB / runtime load.
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/api/runtimes/x/mcp/oauth/start", strings.NewReader(`{"server":"figma","origin":"http://localhost:3000"}`))
	w := httptest.NewRecorder()
	h.InitiateRuntimeMcpOauth(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "not configured") {
		t.Errorf("body = %q, want 'not configured'", w.Body.String())
	}
}

func TestCompleteMcpOauthNotConfigured(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/api/mcp/oauth/callback?code=c&state=s", nil)
	w := httptest.NewRecorder()
	h.CompleteMcpOauth(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestCompleteMcpOauthFailClosed(t *testing.T) {
	h := &Handler{McpOAuthBox: testBox(t), McpOAuthFlows: mcpoauth.NewFlowStore()}
	cases := map[string]string{
		"missing code+state": "/api/mcp/oauth/callback",
		"missing state":      "/api/mcp/oauth/callback?code=abc",
		"unknown/used state": "/api/mcp/oauth/callback?code=abc&state=neverseen",
		"provider error":     "/api/mcp/oauth/callback?error=access_denied",
	}
	for name, target := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			w := httptest.NewRecorder()
			h.CompleteMcpOauth(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (fail closed)", w.Code)
			}
			// The popup page tells the opener it failed.
			if !strings.Contains(w.Body.String(), "mcp-oauth-result") ||
				!strings.Contains(w.Body.String(), `"success":false`) {
				t.Errorf("body should post a failure result, got %q", w.Body.String())
			}
		})
	}
}

func TestNormalizeOAuthOrigin(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"http://localhost:3000", "http://localhost:3000", true},
		{"https://app.multica.ai/", "https://app.multica.ai", true},
		{"https://app.multica.ai/some/path", "https://app.multica.ai", true},
		{"  http://localhost:3000  ", "http://localhost:3000", true},
		{"ftp://x", "", false},
		{"not a url", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := normalizeOAuthOrigin(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("normalizeOAuthOrigin(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

