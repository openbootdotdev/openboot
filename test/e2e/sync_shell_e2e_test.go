//go:build e2e && vm

package e2e

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/testutil"
)

// TestVM_Sync_Shell_CaptureShell verifies that CaptureShell works correctly
// in a real macOS environment: detects Oh-My-Zsh and reads theme/plugins from .zshrc.
func TestVM_Sync_Shell_CaptureShell(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewMacHost(t)
	// Remove any pre-existing oh-my-zsh so installOhMyZsh actually runs the
	// install script rather than skipping via the idempotency guard.
	vm.Run("rm -rf ~/.oh-my-zsh")
	installOhMyZsh(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Set a known theme and plugins so we can assert on them
	_, err := vm.Run(`sed -i '' 's/ZSH_THEME="[^"]*"/ZSH_THEME="agnoster"/' ~/.zshrc`)
	require.NoError(t, err)
	_, err = vm.Run(`sed -i '' 's/plugins=(git)/plugins=(git docker)/' ~/.zshrc`)
	require.NoError(t, err)

	zshrc, err := vm.Run("cat ~/.zshrc")
	require.NoError(t, err)
	assert.Contains(t, zshrc, `ZSH_THEME="agnoster"`, ".zshrc should have agnoster theme")
	assert.Contains(t, zshrc, "docker", ".zshrc should have docker plugin")

	// Run snapshot --json to exercise the full capture path (including CaptureShell indirectly)
	snapshotOut, err := vmRunDevBinary(t, vm, bin, "snapshot --json")
	require.NoError(t, err, "snapshot should succeed, output: %s", snapshotOut)
	assert.NotEmpty(t, snapshotOut)
	// Snapshot must capture the shell config we set — not just return empty JSON.
	assert.Contains(t, snapshotOut, "agnoster", "snapshot JSON should capture the zsh theme")
	assert.Contains(t, snapshotOut, "docker", "snapshot JSON should capture the zsh plugins")

	_, err = vm.Run("test -d ~/.oh-my-zsh")
	assert.NoError(t, err, "~/.oh-my-zsh should exist after install")
}

// TestVM_Sync_Shell_NoPanic verifies that the binary handles a remote config
// with shell settings without panicking.
func TestVM_Sync_Shell_NoPanic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewMacHost(t)
	// Remove any pre-existing oh-my-zsh so installOhMyZsh actually runs the
	// install script rather than skipping via the idempotency guard.
	vm.Run("rm -rf ~/.oh-my-zsh")
	installOhMyZsh(t, vm)
	bin := vmCopyDevBinary(t, vm)

	cfg := `{"username":"testuser","slug":"default","packages":[],"casks":[],"taps":[],"npm":[],"shell":{"oh_my_zsh":true,"theme":"agnoster","plugins":["git","docker"]}}`
	escaped := strings.ReplaceAll(cfg, "'", "'\\''")
	_, err := vm.Run("printf '%s' '" + escaped + "' > /tmp/shell-config.json")
	require.NoError(t, err)

	out, _ := vmRunDevBinaryWithGit(t, vm, bin, "install --from /tmp/shell-config.json --silent --dry-run")
	t.Logf("dry-run output:\n%s", out)
	assert.NotContains(t, out, "panic", "binary should not panic with shell config")
}
