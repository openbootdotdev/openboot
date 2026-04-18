//go:build e2e && vm

package e2e

import (
	"strings"
	"testing"

	"github.com/openbootdotdev/openboot/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVM_Cmd_Version tests `openboot version`.
func TestVM_Cmd_Version(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	t.Run("prints_version", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "version")
		require.NoError(t, err)
		assert.Contains(t, output, "OpenBoot v")
	})
}

// TestVM_Cmd_Help tests `openboot help` and `--help` / `-h` flags.
func TestVM_Cmd_Help(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	t.Run("help_command", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "help")
		require.NoError(t, err)
		assert.Contains(t, output, "Usage:")
		assert.Contains(t, output, "Available Commands:")
	})

	t.Run("help_flag_long", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "--help")
		require.NoError(t, err)
		assert.Contains(t, output, "Usage:")
	})

	t.Run("help_flag_short", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "-h")
		require.NoError(t, err)
		assert.Contains(t, output, "Usage:")
	})

	t.Run("help_for_subcommand", func(t *testing.T) {
		for _, cmd := range []string{"install", "snapshot", "config", "login", "logout", "version"} {
			t.Run(cmd, func(t *testing.T) {
				output, err := vmRunDevBinary(t, vm, bin, cmd+" --help")
				require.NoError(t, err, "help for %s failed, output: %s", cmd, output)
				assert.Contains(t, output, "Usage:", "help for %s should show usage", cmd)
			})
		}
	})

	t.Run("unknown_command", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "nonexistent-xyz")
		assert.Error(t, err)
		assert.True(t,
			strings.Contains(output, "unknown command") || strings.Contains(output, "unknown"),
			"got: %s", output)
	})

	t.Run("unknown_flag", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "--nonexistent-flag")
		assert.Error(t, err)
		assert.True(t,
			strings.Contains(output, "unknown flag") || strings.Contains(output, "flag"),
			"got: %s", output)
	})
}

// TestVM_Cmd_Snapshot tests `openboot snapshot` with all flags.
func TestVM_Cmd_Snapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	t.Run("json_output", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "snapshot --json")
		require.NoError(t, err, "output: %s", output)
		assert.Contains(t, output, "{")
		assert.Contains(t, output, "packages")
		assert.Contains(t, output, "formulae")
		assert.Contains(t, output, "casks")
	})

	t.Run("local_save", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "snapshot --local")
		require.NoError(t, err, "output: %s", output)
		assert.True(t,
			strings.Contains(output, "saved") || strings.Contains(output, "✓"),
			"should confirm save, got: %s", output)

		// Verify file created
		out, err := vm.Run("test -f ~/.openboot/snapshot.json && echo exists || echo missing")
		require.NoError(t, err)
		assert.Contains(t, out, "exists")
	})

	t.Run("dry_run", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "snapshot --dry-run --json")
		// --dry-run with snapshot should still capture (dry-run affects import/restore)
		if err != nil {
			t.Logf("snapshot --dry-run exited with: %v, output: %s", err, output)
		}
		t.Logf("output: %s", output)
	})

	t.Run("import_from_file", func(t *testing.T) {
		// First save a snapshot, then import it
		_, _ = vmRunDevBinary(t, vm, bin, "snapshot --local")

		// Import with --dry-run to avoid side effects
		output, err := vmRunDevBinaryWithGit(t, vm, bin, "snapshot --import ~/.openboot/snapshot.json --dry-run")
		t.Logf("import dry-run output:\n%s", output)
		if err != nil {
			t.Logf("import dry-run exited with: %v (may be expected)", err)
		}
	})

	t.Run("import_invalid_file", func(t *testing.T) {
		_, err := vmRunDevBinary(t, vm, bin, "snapshot --import /nonexistent/file.json")
		assert.Error(t, err, "importing nonexistent file should fail")
	})
}

// TestVM_Cmd_Login tests `openboot login` and `openboot logout`.
func TestVM_Cmd_Login(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	t.Run("logout_when_not_logged_in", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "logout")
		require.NoError(t, err, "logout should succeed gracefully")
		assert.True(t,
			strings.Contains(output, "Not logged in") || strings.Contains(output, "not logged"),
			"got: %s", output)
	})

	t.Run("login_no_browser", func(t *testing.T) {
		// In headless VM, login can't open browser — should handle gracefully
		// We use expect with a timeout to avoid hanging
		output, err := vm.RunInteractive(
			"export PATH=\"/opt/homebrew/bin:$PATH\" && "+bin+" login",
			[]testutil.ExpectStep{},
			10, // 10 second timeout
		)
		t.Logf("login output:\n%s", output)
		if err != nil {
			t.Logf("login exited with: %v (expected in headless VM)", err)
		}
	})
}

// TestVM_Cmd_Completion tests `openboot completion`.
func TestVM_Cmd_Completion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			output, err := vmRunDevBinary(t, vm, bin, "completion "+shell)
			require.NoError(t, err, "completion %s should succeed, output: %s", shell, output)
			assert.Greater(t, len(output), 100, "completion script should have content")
		})
	}

	t.Run("powershell", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "completion powershell")
		// PowerShell may or may not be supported
		t.Logf("completion powershell: output=%d bytes, err=%v", len(output), err)
	})
}
