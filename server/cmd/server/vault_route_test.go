package main

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestVaultRoutesRegistered asserts the /vault subtree is reachable under the
// workspace member group and that non-members are rejected. The test server's
// Config leaves VAULT_PATH unset, so status returns enabled:false (no FS needed).
func TestVaultRoutesRegistered(t *testing.T) {
	if testServer == nil {
		t.Skip("no database connection")
	}

	t.Run("status reachable for member", func(t *testing.T) {
		resp := authRequest(t, http.MethodGet, "/api/workspaces/"+testWorkspaceID+"/vault/status", nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200 (route must be registered)", resp.StatusCode)
		}
		var got struct {
			Enabled bool `json:"enabled"`
		}
		readJSON(t, resp, &got)
		if got.Enabled {
			t.Fatal("enabled = true, want false (VAULT_PATH unset in test config)")
		}
	})

	t.Run("non-member is rejected", func(t *testing.T) {
		foreign := uuid.NewString()
		resp := authRequest(t, http.MethodGet, "/api/workspaces/"+foreign+"/vault/status", nil)
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("non-member got 200; vault routes must sit inside the member gate")
		}
	})
}
