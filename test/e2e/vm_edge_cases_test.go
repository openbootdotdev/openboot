//go:build e2e && vm

// Edge case and advanced scenario tests that complement the core journey tests.

package e2e

import (
	"strings"
	"testing"

	"github.com/openbootdotdev/openboot/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Scenario: Shell actually works (zsh -l can start)
// =============================================================================

// TestVM_Edge_ShellActuallyWorks verifies that after shell setup,
// zsh can actually start a login shell without errors.
//
// User expectation: "I opened a new terminal after openboot setup.
// It should work normally without errors."
func TestVM_Edge_ShellActuallyWorks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM edge case in short mode")
	}

	vm := testutil.NewMacHost(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Install with shell setup
	_, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --shell install --dotfiles skip --macos skip")
	require.NoError(t, err)

	t.Run("zsh_login_shell_starts", func(t *testing.T) {
		// Run a command through a login zsh — this sources .zshrc
		output, err := vm.Run("zsh -l -c 'echo zsh-login-ok'")
		require.NoError(t, err, "zsh login shell should start, output: %s", output)
		assert.Contains(t, output, "zsh-login-ok")
	})

	t.Run("brew_in_path_via_zshrc", func(t *testing.T) {
		// Verify that .zshrc sets up brew path correctly
		output, err := vm.Run("zsh -l -c 'which brew'")
		require.NoError(t, err, "brew should be in PATH via .zshrc, output: %s", output)
		assert.Contains(t, output, "brew")
	})

	t.Run("oh_my_zsh_loads", func(t *testing.T) {
		// Verify oh-my-zsh is sourced by checking .zshrc references it
		output, err := vm.Run("grep -c 'oh-my-zsh' ~/.zshrc")
		require.NoError(t, err, "output: %s", output)
		count := strings.TrimSpace(output)
		assert.NotEqual(t, "0", count, ".zshrc should reference oh-my-zsh")
	})
}
