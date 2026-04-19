//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/testutil"
)

// ── request capture ──────────────────────────────────────────────────────────

type publishImportReq struct {
	Method string
	Path   string
	Auth   string
	Body   map[string]interface{}
}

// reqLog records every request received by a mock server.
type reqLog struct {
	mu   sync.Mutex
	reqs []publishImportReq
}

func (l *reqLog) record(r *http.Request) {
	pr := publishImportReq{
		Method: r.Method,
		Path:   r.URL.Path,
		Auth:   r.Header.Get("Authorization"),
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&pr.Body)
	}
	l.mu.Lock()
	l.reqs = append(l.reqs, pr)
	l.mu.Unlock()
}

// firstMatch returns the first recorded request whose path equals target.
func (l *reqLog) firstMatch(path string) (publishImportReq, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.reqs {
		if r.Path == path {
			return r, true
		}
	}
	return publishImportReq{}, false
}

// ── mock server ───────────────────────────────────────────────────────────────

// newMockServer starts an httptest.Server that:
//   - records all incoming requests in the returned *reqLog
//   - responds with the JSON body registered for each path (200 OK)
//   - falls back to 200 + empty JSON object for unregistered paths
func newMockServer(t *testing.T, routes map[string]interface{}) (*httptest.Server, *reqLog) {
	t.Helper()
	log := &reqLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		body, ok := routes[r.URL.Path]
		if !ok {
			body = map[string]interface{}{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv, log
}

// ── filesystem helpers ────────────────────────────────────────────────────────

func writeJSONFile(t *testing.T, path string, v interface{}) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))
}

// seedAuth writes an unexpired auth token to <homeDir>/.openboot/auth.json.
func seedAuth(t *testing.T, homeDir, token, username string) {
	t.Helper()
	dir := filepath.Join(homeDir, ".openboot")
	require.NoError(t, os.MkdirAll(dir, 0700))
	writeJSONFile(t, filepath.Join(dir, "auth.json"), map[string]interface{}{
		"token":      token,
		"username":   username,
		"expires_at": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		"created_at": time.Now().Format(time.RFC3339),
	})
}

// seedSyncSource writes a sync source to <homeDir>/.openboot/sync_source.json.
// This simulates a machine that has previously installed a cloud config, so
// `snapshot --publish` (without --slug) resolves to an update (PUT) rather
// than an interactive create (POST).
func seedSyncSource(t *testing.T, homeDir, username, slug string) {
	t.Helper()
	dir := filepath.Join(homeDir, ".openboot")
	require.NoError(t, os.MkdirAll(dir, 0700))
	writeJSONFile(t, filepath.Join(dir, "sync_source.json"), map[string]interface{}{
		"user_slug":    username + "/" + slug,
		"username":     username,
		"slug":         slug,
		"synced_at":    time.Now().Format(time.RFC3339),
		"installed_at": time.Now().Format(time.RFC3339),
	})
}

// ── process helpers ───────────────────────────────────────────────────────────

// isolatedEnv returns an environment slice suitable for test binary invocations:
//   - HOME is replaced with an isolated temp directory
//   - All OPENBOOT_* vars from the parent process are stripped
//   - OPENBOOT_API_URL is pointed at the mock server
//   - OPENBOOT_DISABLE_AUTOUPDATE suppresses the GitHub version check
func isolatedEnv(homeDir, apiURL string) []string {
	var env []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "HOME=") || strings.HasPrefix(e, "OPENBOOT_") {
			continue
		}
		env = append(env, e)
	}
	return append(env,
		"HOME="+homeDir,
		"OPENBOOT_API_URL="+apiURL,
		"OPENBOOT_DISABLE_AUTOUPDATE=1",
	)
}

// runBinary executes the openboot binary with the given args and environment,
// returning stdout, stderr, and the process exit error.
func runBinary(t *testing.T, binary string, env []string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = env
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	return outBuf.String(), errBuf.String(), cmd.Run()
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestE2E_Publish_UpdateViaExplicitSlug verifies the P7 invariant:
// `snapshot --publish --slug X` must send a PUT (not POST) to
// /api/configs/from-snapshot carrying the target slug in the body and the
// auth token in the Authorization header.
func TestE2E_Publish_UpdateViaExplicitSlug(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	home := t.TempDir()

	const (
		token    = "e2e-test-bearer-token"
		username = "alice"
		slug     = "dev-setup"
	)
	seedAuth(t, home, token, username)

	srv, log := newMockServer(t, map[string]interface{}{
		"/api/configs/from-snapshot": map[string]string{"slug": slug},
	})

	_, stderr, err := runBinary(t, binary, isolatedEnv(home, srv.URL),
		"snapshot", "--publish", "--slug", slug)
	t.Logf("stderr:\n%s", stderr)
	require.NoError(t, err, "publish --slug should succeed against mock server")

	req, ok := log.firstMatch("/api/configs/from-snapshot")
	require.True(t, ok, "binary must call /api/configs/from-snapshot")

	assert.Equal(t, http.MethodPut, req.Method,
		"updating an existing config must use PUT, not POST")
	assert.Equal(t, "Bearer "+token, req.Auth,
		"Authorization header must carry the stored Bearer token")
	require.NotNil(t, req.Body, "request body must be present")
	assert.Equal(t, slug, req.Body["config_slug"],
		"body must contain config_slug so the server knows which config to update")
	assert.Contains(t, req.Body, "snapshot",
		"body must embed the captured snapshot object")
}

// TestE2E_Publish_UpdateViaSyncSource verifies the P7 invariant:
// when no --slug flag is given but a sync source exists on disk,
// `snapshot --publish` resolves to an update (PUT) using that source's slug,
// and the output names the config being updated.
func TestE2E_Publish_UpdateViaSyncSource(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	home := t.TempDir()

	const (
		token    = "e2e-sync-source-token"
		username = "bob"
		slug     = "my-env"
	)
	seedAuth(t, home, token, username)
	seedSyncSource(t, home, username, slug)

	srv, log := newMockServer(t, map[string]interface{}{
		"/api/configs/from-snapshot": map[string]string{"slug": slug},
	})

	_, stderr, err := runBinary(t, binary, isolatedEnv(home, srv.URL), "snapshot", "--publish")
	t.Logf("stderr:\n%s", stderr)
	require.NoError(t, err, "publish with a saved sync source should succeed")

	req, ok := log.firstMatch("/api/configs/from-snapshot")
	require.True(t, ok, "binary must call /api/configs/from-snapshot")

	assert.Equal(t, http.MethodPut, req.Method,
		"sync-source update must use PUT")
	assert.Equal(t, "Bearer "+token, req.Auth)
	require.NotNil(t, req.Body)
	assert.Equal(t, slug, req.Body["config_slug"],
		"body must carry the sync source's slug")

	// P7: output must identify the config being updated ("Publishing to @user/slug").
	assert.Contains(t, stderr, username+"/"+slug,
		"output must name the config being updated")
}

// TestE2E_Install_FetchesCloudConfig verifies that
// `install user/slug --dry-run --silent` makes exactly a
// GET /{user}/{slug}/config request with the stored Bearer token and exits 0.
// The installer runs in dry-run mode so no packages are installed.
func TestE2E_Install_FetchesCloudConfig(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	home := t.TempDir()

	const (
		token    = "e2e-install-bearer-token"
		username = "carol"
		slug     = "team-config"
	)
	seedAuth(t, home, token, username)

	configPath := "/" + username + "/" + slug + "/config"
	srv, log := newMockServer(t, map[string]interface{}{
		configPath: map[string]interface{}{
			"packages": []string{"git"},
			"casks":    []string{},
			"taps":     []string{},
			"npm":      []string{},
			"preset":   "minimal",
		},
	})

	_, stderr, err := runBinary(t, binary, isolatedEnv(home, srv.URL),
		"install", username+"/"+slug, "--dry-run", "--silent")
	t.Logf("stderr:\n%s", stderr)
	require.NoError(t, err, "install --dry-run --silent should exit 0")

	req, ok := log.firstMatch(configPath)
	require.True(t, ok, "binary must fetch %s", configPath)

	assert.Equal(t, http.MethodGet, req.Method)
	assert.Equal(t, "Bearer "+token, req.Auth,
		"install must forward the stored Bearer token when fetching a cloud config")
}
