//go:build e2e && vm

package e2e

import (
	"fmt"
	"strings"
	"testing"

	"github.com/openbootdotdev/openboot/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// revisionListJSON is the mock response for GET /api/configs/*/revisions.
const revisionListJSON = `{
  "revisions": [
    {
      "id": "rev_abc1234567890123",
      "message": "before adding rust",
      "created_at": "2026-01-10 10:00:00",
      "package_count": 2
    },
    {
      "id": "rev_def4567890123456",
      "message": null,
      "created_at": "2026-01-05 09:00:00",
      "package_count": 1
    }
  ]
}`

// revisionDetailJSON is the mock response for GET /api/configs/*/revisions/rev_abc1234567890123.
// Packages are deliberately minimal so dry-run shows simple diff.
const revisionDetailJSON = `{
  "id": "rev_abc1234567890123",
  "message": "before adding rust",
  "created_at": "2026-01-10 10:00:00",
  "packages": [
    {"name": "git", "type": "formula"},
    {"name": "curl", "type": "formula"}
  ]
}`

// restoreResponseJSON is the mock response for POST /api/configs/*/revisions/*/restore.
const restoreResponseJSON = `{
  "restored": true,
  "revision_id": "rev_abc1234567890123",
  "packages": [
    {"name": "git", "type": "formula"},
    {"name": "curl", "type": "formula"}
  ]
}`

// startMockAPIServerForLog starts a Python HTTP server that handles
// revision list and single revision fetch endpoints.
func startMockAPIServerForLog(t *testing.T, vm *testutil.TartVM, port int) string {
	t.Helper()

	writeCmd := func(varName, content, path string) {
		cmd := fmt.Sprintf(`printf '%%s' %s > %s`, shellescape(content), path)
		_, err := vm.Run(cmd)
		require.NoError(t, err, "write "+varName)
	}

	writeCmd("revisions list", revisionListJSON, "/tmp/mock-revisions.json")
	writeCmd("revision detail", revisionDetailJSON, "/tmp/mock-revision-detail.json")
	writeCmd("restore response", restoreResponseJSON, "/tmp/mock-restore-resp.json")

	// Also write the standard remote config so sync diff works after restore
	writeCmd("remote config", mockAPIConfig, "/tmp/mock-config.json")

	pyServer := fmt.Sprintf(`import http.server, pathlib, re
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        path = self.path.split('?')[0]
        # GET /api/configs/*/revisions/<id>  →  single revision
        if re.match(r'^/api/configs/[^/]+/revisions/[^/]+$', path):
            body = pathlib.Path('/tmp/mock-revision-detail.json').read_bytes()
            self.respond(200, body)
        # GET /api/configs/*/revisions  →  list
        elif re.match(r'^/api/configs/[^/]+/revisions$', path):
            body = pathlib.Path('/tmp/mock-revisions.json').read_bytes()
            self.respond(200, body)
        # Anything else (remote config fetch for sync)
        else:
            body = pathlib.Path('/tmp/mock-config.json').read_bytes()
            self.respond(200, body)
    def do_POST(self):
        path = self.path.split('?')[0]
        # POST /api/configs/*/revisions/*/restore
        if re.match(r'^/api/configs/[^/]+/revisions/[^/]+/restore$', path):
            length = int(self.headers.get('Content-Length', 0))
            self.rfile.read(length)
            pathlib.Path('/tmp/restore-called').write_bytes(b'1')
            body = pathlib.Path('/tmp/mock-restore-resp.json').read_bytes()
            self.respond(200, body)
        else:
            self.respond(404, b'{}')
    def respond(self, status, body):
        self.send_response(status)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *a): pass
http.server.HTTPServer(('127.0.0.1', %d), H).serve_forever()
`, port)

	_, err := vm.Run(fmt.Sprintf("cat > /tmp/mock-server-log.py << 'PYEOF'\n%sPYEOF", pyServer))
	require.NoError(t, err, "write mock log/restore server script")

	pidOut, err := vm.Run(fmt.Sprintf("nohup python3 /tmp/mock-server-log.py >/tmp/mock-log-api.log 2>&1 & echo $!"))
	require.NoError(t, err, "start mock log/restore API server")
	pid := strings.TrimSpace(pidOut)
	t.Logf("mock log/restore API server started (pid=%s) on port %d", pid, port)

	// Poll until server responds
	for i := 0; i < 10; i++ {
		_, _ = vm.Run("sleep 1")
		out, curlErr := vm.Run(fmt.Sprintf("curl -s http://localhost:%d/api/configs/x/revisions", port))
		if curlErr == nil && strings.Contains(out, "revisions") {
			break
		}
		t.Logf("waiting for log mock server (attempt %d)", i+1)
	}

	t.Cleanup(func() {
		if pid != "" {
			_, _ = vm.Run("kill " + pid + " 2>/dev/null || true")
		}
	})

	return fmt.Sprintf("http://localhost:%d", port)
}

// TestE2E_Log_ShowsRevisionHistory verifies that `openboot log` fetches the
// revision list from the API and displays revision IDs and messages.
func TestE2E_Log_ShowsRevisionHistory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	bin := vmCopyDevBinary(t, vm)

	apiURL := startMockAPIServerForLog(t, vm, 19895)
	writeFakeAuth(t, vm, "testuser", "fake-token-e2e")
	writeSyncSource(t, vm, "testuser/myconfig")

	// Save slug to sync source so `log` knows which config to query
	_, err := vm.Run(`echo '{"user_slug":"testuser/myconfig","username":"testuser","slug":"myconfig"}' > ~/.openboot/sync_source.json`)
	require.NoError(t, err, "write sync source with slug")

	env := map[string]string{
		"PATH":             brewPath,
		"OPENBOOT_API_URL": apiURL,
	}
	out, err := vm.RunWithEnv(env, bin+" log")
	t.Logf("log output:\n%s", out)
	require.NoError(t, err, "openboot log should exit 0")

	assert.Contains(t, out, "rev_abc1234567890123", "should show first revision ID")
	assert.Contains(t, out, "before adding rust", "should show revision message")
	assert.Contains(t, out, "rev_def4567890123456", "should show second revision ID")
	assert.Contains(t, out, "2 pkgs", "should show package count for first revision")
	assert.Contains(t, out, "1 pkgs", "should show package count for second revision")
	assert.Contains(t, out, "openboot restore", "should show restore hint")
}

// TestE2E_Log_NoSyncSource_ReturnsError verifies that `openboot log` without
// a saved sync source fails with a clear error message.
func TestE2E_Log_NoSyncSource_ReturnsError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	bin := vmCopyDevBinary(t, vm)

	apiURL := startMockAPIServerForLog(t, vm, 19896)
	writeFakeAuth(t, vm, "testuser", "fake-token-e2e")
	// No sync source written

	env := map[string]string{
		"PATH":             brewPath,
		"OPENBOOT_API_URL": apiURL,
	}
	out, err := vm.RunWithEnv(env, bin+" log")
	t.Logf("log no-source output:\n%s", out)
	// Should exit non-zero with a useful error
	assert.Error(t, err, "log without sync source should fail")
	assert.Contains(t, out, "slug", "error should mention slug")
}

// TestE2E_Restore_DryRun_ShowsDiff verifies that `openboot restore --dry-run`
// fetches the revision packages from the API, computes a local diff, and
// displays what would change — without calling the restore endpoint or
// modifying the system.
func TestE2E_Restore_DryRun_ShowsDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	bin := vmCopyDevBinary(t, vm)

	apiURL := startMockAPIServerForLog(t, vm, 19897)
	writeFakeAuth(t, vm, "testuser", "fake-token-e2e")
	_, err := vm.Run(`echo '{"user_slug":"testuser/myconfig","username":"testuser","slug":"myconfig"}' > ~/.openboot/sync_source.json`)
	require.NoError(t, err, "write sync source with slug")

	env := map[string]string{
		"PATH":             brewPath,
		"OPENBOOT_API_URL": apiURL,
	}
	out, err := vm.RunWithEnv(env, bin+" restore rev_abc1234567890123 --dry-run")
	t.Logf("restore --dry-run output:\n%s", out)
	// dry-run may exit 0 even if no changes; the key check is no error from restore endpoint
	t.Logf("restore exit: %v", err)

	// dry-run should NOT call the restore POST endpoint
	restoreCalled, _ := vm.Run("cat /tmp/restore-called 2>/dev/null")
	assert.Empty(t, strings.TrimSpace(restoreCalled), "restore endpoint should NOT be called in --dry-run")

	assert.Contains(t, out, "rev_abc1234567890123", "should mention the revision being restored")
	assert.Contains(t, out, "dry-run", "should indicate dry-run mode")
}

// TestE2E_Restore_ActualApply verifies that `openboot restore --yes` (non-dry-run)
// calls the server restore endpoint and then applies the resulting package diff
// to the local system via brew.
//
// Setup:  mock API returns a revision containing only "git" + "curl".
//         The VM's base image has many extra packages (awscli, cmake, etc.)
//         that should be flagged as extra but —since --install-only is not set—
//         we only verify that the restore endpoint was called and that the
//         command exits 0, not that all extras were removed (those prompts are
//         skipped via --yes).
func TestE2E_Restore_ActualApply(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	bin := vmCopyDevBinary(t, vm)

	apiURL := startMockAPIServerForLog(t, vm, 19899)
	writeFakeAuth(t, vm, "testuser", "fake-token-e2e")
	_, err := vm.Run(`echo '{"user_slug":"testuser/myconfig","username":"testuser","slug":"myconfig"}' > ~/.openboot/sync_source.json`)
	require.NoError(t, err, "write sync source with slug")

	env := map[string]string{
		"PATH":             brewPath,
		"OPENBOOT_API_URL": apiURL,
	}

	// Run actual restore (no --dry-run) with --yes to skip interactive prompts.
	out, err := vm.RunWithEnv(env, bin+" restore rev_abc1234567890123 --yes")
	t.Logf("restore --yes output:\n%s", out)
	t.Logf("restore --yes exit: %v", err)

	// The restore endpoint must have been called.
	restoreCalled, _ := vm.Run("cat /tmp/restore-called 2>/dev/null")
	assert.Equal(t, "1", strings.TrimSpace(restoreCalled), "restore endpoint should have been called")

	// Command should succeed.
	require.NoError(t, err, "openboot restore --yes should exit 0")

	// Output should confirm restoring to the revision.
	assert.Contains(t, out, "rev_abc1234567890123", "should mention the revision ID")
	assert.Contains(t, out, "Server config restored", "should confirm server-side restore")
}

// TestE2E_Restore_WithSlugFlag verifies that `openboot restore` uses the --slug
// flag to target the correct config when no sync source is saved.
func TestE2E_Restore_WithSlugFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	bin := vmCopyDevBinary(t, vm)

	apiURL := startMockAPIServerForLog(t, vm, 19898)
	writeFakeAuth(t, vm, "testuser", "fake-token-e2e")
	// No sync source — must use --slug

	env := map[string]string{
		"PATH":             brewPath,
		"OPENBOOT_API_URL": apiURL,
	}
	out, err := vm.RunWithEnv(env, bin+" restore rev_abc1234567890123 --dry-run --slug myconfig")
	t.Logf("restore --slug output:\n%s", out)
	t.Logf("restore --slug exit: %v", err)

	// Should reach the diff stage (not fail on missing slug)
	assert.Contains(t, out, "rev_abc1234567890123", "should mention the revision")
}
