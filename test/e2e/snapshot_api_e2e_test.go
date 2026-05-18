//go:build e2e && !vm

// Package e2e contains E2E tests for the snapshot publish and import
// commands exercised via the compiled binary against a local mock HTTP server.
//
// Gaps filled:
//   - `snapshot --publish`: HTTP POST/PUT path was never run end-to-end;
//     slug conflicts and updates had no coverage.
//   - `snapshot --import URL`: the http:// (insecure) rejection was only
//     tested at the unit level; the binary error path was untested.

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/auth"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
	"github.com/openbootdotdev/openboot/testutil"
)

// =============================================================================
// snapshot --publish
// =============================================================================

// TestE2E_Snapshot_Publish_UpdatesExistingConfig runs
//
//	openboot snapshot --publish
//
// when a sync source is already saved (simulating a second publish).
// The binary should issue a PUT request and report "updated successfully".
//
// Gap: the PUT path (update existing config) was never exercised via the binary.
func TestE2E_Snapshot_Publish_UpdatesExistingConfig(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	home := t.TempDir()

	writePublishAuthFile(t, home, "obt_pub_token", "pubuser")
	writePublishSyncSource(t, home, "pubuser", "my-existing-config")

	var receivedMethod string
	var receivedAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/configs/from-snapshot" && r.Method == http.MethodPut:
			receivedMethod = r.Method
			receivedAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"slug": "my-existing-config"}) //nolint:errcheck // test helper
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"packages": []interface{}{}}) //nolint:errcheck // test helper
		}
	}))
	t.Cleanup(srv.Close)

	stdout, stderr, err := runBinary(t, binary, isolatedEnv(home, srv.URL), "snapshot", "--publish")
	output := stdout + stderr
	t.Logf("publish output:\n%s", output)
	require.NoError(t, err, "snapshot --publish should succeed")

	t.Run("output_confirms_update", func(t *testing.T) {
		assert.True(t,
			strings.Contains(output, "updated") || strings.Contains(output, "Updated") ||
				strings.Contains(output, "successfully"),
			"output should confirm successful update, got: %s", output)
	})

	t.Run("api_received_PUT_with_auth_header", func(t *testing.T) {
		assert.Equal(t, http.MethodPut, receivedMethod, "update should send PUT")
		assert.Equal(t, "Bearer obt_pub_token", receivedAuth, "should include Bearer token")
	})
}

// TestE2E_Snapshot_Publish_ExplicitSlugUpdate runs
//
//	openboot snapshot --publish --slug my-config
//
// verifying that an explicit --slug forces PUT even without a stored sync source.
func TestE2E_Snapshot_Publish_ExplicitSlugUpdate(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	home := t.TempDir()

	writePublishAuthFile(t, home, "obt_slug_token", "sluguser")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/configs/from-snapshot" && r.Method == http.MethodPut {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"slug": "my-config"}) //nolint:errcheck // test helper
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"packages": []interface{}{}}) //nolint:errcheck // test helper
	}))
	t.Cleanup(srv.Close)

	stdout, stderr, err := runBinary(t, binary, isolatedEnv(home, srv.URL), "snapshot", "--publish", "--slug", "my-config")
	output := stdout + stderr
	t.Logf("publish --slug output:\n%s", output)
	require.NoError(t, err, "snapshot --publish --slug should succeed")
	assert.True(t,
		strings.Contains(output, "updated") || strings.Contains(output, "Updated") ||
			strings.Contains(output, "successfully"),
		"output should confirm update, got: %s", output)
}

// TestE2E_Snapshot_Publish_ConflictError verifies that when the API returns a
// 409 conflict the binary surfaces the server's error message (not a generic
// "HTTP 409" string).
//
// Gap: slug conflicts were never exercised via the compiled binary.
func TestE2E_Snapshot_Publish_ConflictError(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	home := t.TempDir()

	writePublishAuthFile(t, home, "obt_conflict_token", "conflictuser")
	writePublishSyncSource(t, home, "conflictuser", "existing-slug")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/configs/from-snapshot" {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"message": "config slug already exists"}) //nolint:errcheck // test helper
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"packages": []interface{}{}}) //nolint:errcheck // test helper
	}))
	t.Cleanup(srv.Close)

	stdout, stderr, err := runBinary(t, binary, isolatedEnv(home, srv.URL), "snapshot", "--publish")
	output := stdout + stderr
	t.Logf("conflict output:\n%s", output)
	assert.Error(t, err, "snapshot --publish should fail on 409")
	assert.True(t,
		strings.Contains(output, "already exists") || strings.Contains(output, "conflict") ||
			strings.Contains(output, "slug"),
		"output should describe the conflict, got: %s", output)
}

// =============================================================================
// snapshot --import URL
// =============================================================================

// TestE2E_Snapshot_Import_InsecureHTTP_Rejected verifies that the binary
// refuses to download a snapshot from a plain http:// URL and returns an
// actionable error message.
//
// Gap: only the internal/cli unit test covered this; the binary's error path
// was never exercised.
func TestE2E_Snapshot_Import_InsecureHTTP_Rejected(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	home := t.TempDir()

	insecureURL := "http://127.0.0.1:19998/snap.json"
	stdout, stderr, err := runBinary(t, binary, isolatedEnv(home, ""),
		"snapshot", "--import", insecureURL, "--dry-run")
	output := stdout + stderr
	t.Logf("insecure import output:\n%s", output)
	assert.Error(t, err, "importing from http:// should fail")
	assert.True(t,
		strings.Contains(output, "insecure") || strings.Contains(output, "https") ||
			strings.Contains(output, "not allowed"),
		"error should tell the user to use https://, got: %s", output)
}

// TestE2E_Snapshot_Import_DownloadError verifies that the binary returns a
// meaningful error when an HTTPS download fails (e.g., server not found).
func TestE2E_Snapshot_Import_DownloadError(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	home := t.TempDir()

	// This host / port does not exist so the TLS handshake fails.
	badURL := "https://127.0.0.1:19997/snap.json"
	stdout, stderr, err := runBinary(t, binary, isolatedEnv(home, ""), "snapshot", "--import", badURL)
	output := stdout + stderr
	t.Logf("download error output:\n%s", output)
	assert.Error(t, err, "import from unreachable URL should fail")
	assert.True(t,
		strings.Contains(output, "download") || strings.Contains(output, "connect") ||
			strings.Contains(output, "failed") || strings.Contains(output, "refused"),
		"error should indicate download failure, got: %s", output)
}

// =============================================================================
// helpers
// =============================================================================

// writePublishAuthFile writes a valid non-expired auth.json for publish tests.
func writePublishAuthFile(t *testing.T, home, token, username string) {
	t.Helper()
	authDir := filepath.Join(home, ".openboot")
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

// writePublishSyncSource writes a sync_source.json so the binary can resolve a
// target slug without interactive prompts.
func writePublishSyncSource(t *testing.T, home, username, slug string) {
	t.Helper()
	dir := filepath.Join(home, ".openboot")
	require.NoError(t, os.MkdirAll(dir, 0700))

	src := syncpkg.SyncSource{
		UserSlug:    fmt.Sprintf("%s/%s", username, slug),
		Username:    username,
		Slug:        slug,
		InstalledAt: time.Now(),
		SyncedAt:    time.Now(),
	}
	data, err := json.MarshalIndent(src, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sync_source.json"), data, 0600))
}
