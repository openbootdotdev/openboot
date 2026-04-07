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

// mockAPIConfig is the JSON response served by the in-VM mock API server.
// It describes a remote config with oh-my-zsh: agnoster theme and git+docker plugins.
const mockAPIConfig = `{
  "username": "testuser",
  "slug": "myconfig",
  "packages": [],
  "casks": [],
  "taps": [],
  "npm": [],
  "shell": {
    "oh_my_zsh": true,
    "theme": "agnoster",
    "plugins": ["git", "docker"]
  }
}`

// startMockAPIServer starts a Python HTTP server inside the VM that serves
// mockAPIConfig for any GET request. Returns the base URL (http://localhost:PORT).
// The server is killed via t.Cleanup.
func startMockAPIServer(t *testing.T, vm *testutil.TartVM, port int) string {
	t.Helper()

	// Write JSON to a file using printf (more reliable than heredoc over SSH)
	writeCmd := fmt.Sprintf(`printf '%%s' %s > /tmp/mock-config.json`, shellescape(mockAPIConfig))
	_, err := vm.Run(writeCmd)
	require.NoError(t, err, "write mock config JSON")

	// Verify the file was written
	content, err := vm.Run("cat /tmp/mock-config.json")
	require.NoError(t, err, "read mock config JSON")
	require.Contains(t, content, "agnoster", "mock JSON should contain agnoster theme")
	t.Logf("mock config written: %s", content)

	// Python HTTP server as a script file (avoids shell quoting issues with -c)
	pyServer := fmt.Sprintf(`import http.server, pathlib
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        body = pathlib.Path('/tmp/mock-config.json').read_bytes()
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *a): pass
http.server.HTTPServer(('127.0.0.1', %d), H).serve_forever()
`, port)

	_, err = vm.Run(fmt.Sprintf(`cat > /tmp/mock-server.py << 'PYEOF'
%sPYEOF`, pyServer))
	require.NoError(t, err, "write mock server script")

	// Start server with nohup so it persists after the SSH session exits
	pidOut, err := vm.Run(fmt.Sprintf("nohup python3 /tmp/mock-server.py >/tmp/mock-api.log 2>&1 & echo $!"))
	require.NoError(t, err, "start mock API server")
	pid := strings.TrimSpace(pidOut)
	t.Logf("mock API server started (pid=%s) on port %d", pid, port)

	// Poll until server responds (up to 10s)
	var curlOut string
	var curlErr error
	for i := 0; i < 10; i++ {
		_, _ = vm.Run("sleep 1")
		curlOut, curlErr = vm.Run(fmt.Sprintf("curl -s http://localhost:%d/testuser/myconfig/config", port))
		if curlErr == nil && strings.Contains(curlOut, "agnoster") {
			break
		}
		t.Logf("waiting for mock server (attempt %d): err=%v", i+1, curlErr)
	}
	t.Logf("curl verification: err=%v, body=%s", curlErr, curlOut)
	if logOut, _ := vm.Run("cat /tmp/mock-api.log 2>/dev/null"); logOut != "" {
		t.Logf("mock server log:\n%s", logOut)
	}
	require.NoError(t, curlErr, "mock server should respond to curl")
	require.Contains(t, curlOut, "agnoster", "mock server should return our JSON")

	t.Cleanup(func() {
		if pid != "" {
			_, _ = vm.Run("kill " + pid + " 2>/dev/null || true")
		}
	})

	return fmt.Sprintf("http://localhost:%d", port)
}

// shellescape wraps a string in single quotes for safe use in shell commands.
// It escapes any single quotes within the string.
func shellescape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// writeSyncSource writes ~/.openboot/sync_source.json in the VM.
func writeSyncSource(t *testing.T, vm *testutil.TartVM, userSlug string) {
	t.Helper()
	json := fmt.Sprintf(`{"user_slug":"%s"}`, userSlug)
	_, err := vm.Run(fmt.Sprintf("mkdir -p ~/.openboot && echo '%s' > ~/.openboot/sync_source.json", json))
	require.NoError(t, err, "write sync source")
}

// TestE2E_Sync_Shell_DryRunShowsDiff verifies that `openboot sync --dry-run`
// detects a shell config difference and displays it in the output.
//
// Setup: VM has Oh-My-Zsh with default theme (robbyrussell) and plugins (git).
// Remote config (served by a local mock API) specifies theme=agnoster, plugins=git+docker.
// Expected: diff output shows "Shell Changes" with theme and plugin arrows.
func TestE2E_Sync_Shell_DryRunShowsDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	installOhMyZsh(t, vm)
	bin := vmCopyDevBinary(t, vm)

	apiURL := startMockAPIServer(t, vm, 19876)
	writeSyncSource(t, vm, "testuser/myconfig")

	env := map[string]string{
		"PATH":             brewPath,
		"OPENBOOT_API_URL": apiURL,
	}
	out, err := vm.RunWithEnv(env, bin+" sync --dry-run")
	t.Logf("dry-run output:\n%s", out)
	// err is allowed (exit 1 if no changes applied), just verify output
	if err != nil {
		t.Logf("exit: %v", err)
	}

	assert.Contains(t, out, "Shell Changes", "should show Shell Changes section")
	assert.Contains(t, out, "agnoster", "should show target theme")
	// robbyrussell is the default Oh-My-Zsh theme
	assert.Contains(t, out, "→", "should show arrow between old and new values")
}

// TestE2E_Sync_Shell_AppliesTheme verifies that `openboot sync` (non-dry-run)
// actually patches ~/.zshrc with the new theme and plugins.
//
// Setup: Same as DryRun test above.
// Interaction: send Enter twice to accept both confirm prompts.
// Expected: ~/.zshrc has ZSH_THEME="agnoster" and plugins=(git docker) afterwards.
func TestE2E_Sync_Shell_AppliesTheme(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	installOhMyZsh(t, vm)
	bin := vmCopyDevBinary(t, vm)

	apiURL := startMockAPIServer(t, vm, 19877)
	writeSyncSource(t, vm, "testuser/myconfig")

	// Verify initial state
	zshrcBefore, err := vm.Run("cat ~/.zshrc")
	require.NoError(t, err)
	assert.Contains(t, zshrcBefore, "robbyrussell", "initial theme should be robbyrussell")

	// Write a shell script to avoid Tcl quoting issues in RunInteractive's expect script.
	// --install-only skips the "remove extra packages?" prompt from the base VM image.
	syncScript := fmt.Sprintf("#!/bin/sh\nexport PATH=%q\nexport OPENBOOT_API_URL=%q\nexec %s sync --install-only\n",
		brewPath, apiURL, bin)
	_, err = vm.Run(fmt.Sprintf("printf '%%s' %s > /tmp/run-sync.sh && chmod +x /tmp/run-sync.sh", shellescape(syncScript)))
	require.NoError(t, err, "write sync script")

	output, interactErr := vm.RunInteractive("/tmp/run-sync.sh", []testutil.ExpectStep{
		{Expect: "shell config", Send: "\r"}, // Update shell config (theme and plugins)? → Yes
		{Expect: "Apply",        Send: "\r"}, // Apply 1 changes? → Yes
	}, 60)
	t.Logf("interactive sync output:\n%s", output)
	if interactErr != nil {
		t.Logf("interactive exit: %v", interactErr)
	}

	// Verify .zshrc was updated
	zshrcAfter, err := vm.Run("cat ~/.zshrc")
	require.NoError(t, err)
	t.Logf("zshrc after sync:\n%s", zshrcAfter)

	assert.Contains(t, zshrcAfter, `ZSH_THEME="agnoster"`, "theme should be updated to agnoster")
	assert.Contains(t, zshrcAfter, "docker", "plugins should include docker")
}
