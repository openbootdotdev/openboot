//go:build e2e && vm

// Package e2e contains VM-based E2E tests for the login/logout commands,
// exercising the full OAuth device flow via the compiled binary against a
// local mock HTTP server.
//
// Gap filled: the OAuth device flow was previously only tested at the unit
// level (internal/auth/login_test.go). These tests verify the compiled binary
// correctly reads/writes auth.json and surfaces meaningful errors to the user.

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/testutil"
)

// =============================================================================
// login
// =============================================================================

// TestE2E_Login_SuccessfulOAuthFlow runs `openboot login` against a local mock
// HTTP server that immediately approves the device-code request.
//
// User expectation: after running `openboot login`, a valid auth.json containing
// the token returned by the server should exist at ~/.openboot/auth.json.
func TestE2E_Login_SuccessfulOAuthFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	vm := testutil.NewMacHost(t)
	bin := vmCopyDevBinary(t, vm)
	tmpHome := t.TempDir()

	// Mock API: /start returns a code; /poll immediately approves.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/auth/cli/start":
			json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck // test helper
				"code_id": "e2e-code-id",
				"code":    "E2ETEST1",
			})
		case "/api/auth/cli/poll":
			json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck // test helper
				"status":     "approved",
				"token":      "obt_e2e_token",
				"username":   "e2etestuser",
				"expires_at": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Inline env overrides guarantee HOME and OPENBOOT_API_URL win over any
	// inherited values in the bash subprocess.
	cmd := fmt.Sprintf(
		"HOME=%q OPENBOOT_API_URL=%q OPENBOOT_DISABLE_AUTOUPDATE=1 PATH=%q %s login",
		tmpHome, srv.URL, brewPath, bin,
	)
	output, err := vm.Run(cmd)
	t.Logf("login output:\n%s", output)
	require.NoError(t, err, "login should succeed against mock server")

	// The binary must have written auth.json with the token from our server.
	authFile := filepath.Join(tmpHome, ".openboot", "auth.json")
	data, readErr := os.ReadFile(authFile)
	require.NoError(t, readErr, "auth.json should exist after successful login")

	var stored auth.StoredAuth
	require.NoError(t, json.Unmarshal(data, &stored))
	assert.Equal(t, "obt_e2e_token", stored.Token)
	assert.Equal(t, "e2etestuser", stored.Username)
	assert.True(t, stored.ExpiresAt.After(time.Now()), "stored token must not be expired")
}

// TestE2E_Login_AlreadyAuthenticated verifies that `openboot login` reports
// "already logged in" when a valid auth.json already exists, without hitting
// the OAuth flow at all.
func TestE2E_Login_AlreadyAuthenticated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	vm := testutil.NewMacHost(t)
	bin := vmCopyDevBinary(t, vm)
	tmpHome := t.TempDir()
	writeTestAuthFile(t, tmpHome, "obt_existing", "existinguser")

	cmd := fmt.Sprintf(
		"HOME=%q OPENBOOT_DISABLE_AUTOUPDATE=1 PATH=%q %s login",
		tmpHome, brewPath, bin,
	)
	output, err := vm.Run(cmd)
	t.Logf("login output:\n%s", output)
	require.NoError(t, err, "login should succeed when already authenticated")
	// loginCmd prints: ui.Success(fmt.Sprintf("Already logged in as %s", stored.Username))
	assert.Contains(t, output, "Already logged in as existinguser",
		"output should say already logged in with the username")
}

// TestE2E_Login_ServerUnavailable verifies that `openboot login` returns a
// non-zero exit code and a meaningful error when the auth API is unreachable.
func TestE2E_Login_ServerUnavailable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	vm := testutil.NewMacHost(t)
	bin := vmCopyDevBinary(t, vm)
	tmpHome := t.TempDir()

	// Port 19999 has nothing listening.
	cmd := fmt.Sprintf(
		"HOME=%q OPENBOOT_API_URL=%q OPENBOOT_DISABLE_AUTOUPDATE=1 PATH=%q %s login",
		tmpHome, "http://127.0.0.1:19999", brewPath, bin,
	)
	output, err := vm.Run(cmd)
	t.Logf("login output:\n%s", output)
	assert.Error(t, err, "login should fail when server is unreachable")
	// loginCmd returns: fmt.Errorf("login failed: %w", err)
	assert.Contains(t, output, "login failed",
		"error output should say 'login failed', got: %s", output)
}

// TestE2E_Login_ExpiredCodeRejected verifies that the binary surfaces the
// "authorization code expired" error from the poll endpoint so the user
// knows to run `openboot login` again — rather than hanging until timeout.
func TestE2E_Login_ExpiredCodeRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	vm := testutil.NewMacHost(t)
	bin := vmCopyDevBinary(t, vm)
	tmpHome := t.TempDir()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/auth/cli/start":
			json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck // test helper
				"code_id": "expired-id",
				"code":    "EXPD1234",
			})
		case "/api/auth/cli/poll":
			json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck // test helper
				"status": "expired",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cmd := fmt.Sprintf(
		"HOME=%q OPENBOOT_API_URL=%q OPENBOOT_DISABLE_AUTOUPDATE=1 PATH=%q %s login",
		tmpHome, srv.URL, brewPath, bin,
	)
	output, err := vm.Run(cmd)
	t.Logf("login output:\n%s", output)
	assert.Error(t, err, "login should fail when code is expired")
	// pollOnce returns: fmt.Errorf("authorization code expired; please run 'openboot login' again")
	assert.Contains(t, output, "expired",
		"error output should mention the expired code, got: %s", output)

	// auth.json must NOT have been written after a failed login.
	authFile := filepath.Join(tmpHome, ".openboot", "auth.json")
	assert.NoFileExists(t, authFile, "auth.json must not be created after failed login")
}

// =============================================================================
// logout
// =============================================================================

// TestE2E_Logout_WhenAuthenticated verifies that `openboot logout` removes the
// auth.json token file and confirms the username in its output.
func TestE2E_Logout_WhenAuthenticated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	vm := testutil.NewMacHost(t)
	bin := vmCopyDevBinary(t, vm)
	tmpHome := t.TempDir()
	writeTestAuthFile(t, tmpHome, "obt_logout_token", "logoutuser")

	cmd := fmt.Sprintf(
		"HOME=%q OPENBOOT_DISABLE_AUTOUPDATE=1 PATH=%q %s logout",
		tmpHome, brewPath, bin,
	)
	output, err := vm.Run(cmd)
	t.Logf("logout output:\n%s", output)
	require.NoError(t, err, "logout should succeed")
	// logoutCmd prints: ui.Success(fmt.Sprintf("Logged out of %s", stored.Username))
	assert.Contains(t, output, "Logged out of logoutuser",
		"output should confirm logout with username")

	authFile := filepath.Join(tmpHome, ".openboot", "auth.json")
	assert.NoFileExists(t, authFile, "auth.json should be deleted after logout")
}

// TestE2E_Logout_WhenNotAuthenticated verifies that `openboot logout` handles
// the "not logged in" state gracefully (exit 0, informative message, no crash).
func TestE2E_Logout_WhenNotAuthenticated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	vm := testutil.NewMacHost(t)
	bin := vmCopyDevBinary(t, vm)
	tmpHome := t.TempDir()

	cmd := fmt.Sprintf(
		"HOME=%q OPENBOOT_DISABLE_AUTOUPDATE=1 PATH=%q %s logout",
		tmpHome, brewPath, bin,
	)
	output, err := vm.Run(cmd)
	t.Logf("logout output:\n%s", output)
	require.NoError(t, err, "logout should not fail when not logged in")
	// logoutCmd prints: ui.Info("Not logged in.")
	assert.Contains(t, output, "Not logged in",
		"output should say 'Not logged in', got: %s", output)
}

// =============================================================================
// helpers
// =============================================================================

// writeTestAuthFile writes a non-expired auth.json under tmpHome/.openboot/.
func writeTestAuthFile(t *testing.T, tmpHome, token, username string) {
	t.Helper()
	authDir := filepath.Join(tmpHome, ".openboot")
	require.NoError(t, os.MkdirAll(authDir, 0700))

	stored := auth.StoredAuth{
		Token:     token,
		Username:  username,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(authDir, "auth.json"), data, 0600))
}
