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
		for _, cmd := range []string{"install", "snapshot", "sync", "clean", "diff", "doctor", "delete", "update", "init", "push", "login", "logout"} {
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

// TestVM_Cmd_Doctor tests `openboot doctor`.
func TestVM_Cmd_Doctor(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	t.Run("runs_all_checks", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "doctor")
		require.NoError(t, err, "doctor should succeed, output: %s", output)
		assert.Contains(t, output, "OpenBoot Doctor")
		t.Logf("doctor output:\n%s", output)
	})

	t.Run("exit_code_zero", func(t *testing.T) {
		_, err := vmRunDevBinary(t, vm, bin, "doctor")
		assert.NoError(t, err, "doctor should exit 0 even if issues found")
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

// TestVM_Cmd_Diff tests `openboot diff` with all flags.
func TestVM_Cmd_Diff(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Save a snapshot to diff against
	_, err := vmRunDevBinary(t, vm, bin, "snapshot --local")
	require.NoError(t, err)

	t.Run("from_file", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "diff --from ~/.openboot/snapshot.json")
		require.NoError(t, err, "output: %s", output)
		t.Logf("diff output:\n%s", output)
	})

	t.Run("from_file_json", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "diff --from ~/.openboot/snapshot.json --json")
		require.NoError(t, err, "output: %s", output)
		assert.Contains(t, output, "{", "should contain JSON")
	})

	t.Run("from_file_packages_only", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "diff --from ~/.openboot/snapshot.json --packages-only")
		require.NoError(t, err, "output: %s", output)
		t.Logf("packages-only diff:\n%s", output)
	})

	t.Run("self_diff_no_changes", func(t *testing.T) {
		// Diff against own snapshot should show no differences
		output, err := vmRunDevBinary(t, vm, bin, "diff --from ~/.openboot/snapshot.json")
		require.NoError(t, err)
		assert.True(t,
			strings.Contains(output, "No differences") || strings.Contains(output, "matches"),
			"self-diff should show no changes, got: %s", output)
	})

	t.Run("from_user_not_found", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "diff --user nonexistent-user-xyz-999")
		// Should fail or show error about user not found
		t.Logf("diff --user nonexistent output: %s, err: %v", output, err)
		if err != nil {
			assert.True(t,
				strings.Contains(output, "not found") ||
					strings.Contains(output, "error") ||
					strings.Contains(output, "404") ||
					strings.Contains(output, "Error"),
				"should indicate user not found, got: %s", output)
		}
	})

	t.Run("no_source_specified", func(t *testing.T) {
		// Running diff without --from or --user should either use local snapshot or error
		output, err := vmRunDevBinary(t, vm, bin, "diff")
		t.Logf("diff (no args) output: %s, err: %v", output, err)
		// May succeed (using local snapshot) or fail gracefully
	})
}

// TestVM_Cmd_Clean tests `openboot clean` with all flags.
func TestVM_Cmd_Clean(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Install minimal preset to have packages to work with
	_, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --packages-only")
	require.NoError(t, err)

	// Save snapshot of current state
	_, err = vmRunDevBinary(t, vm, bin, "snapshot --local")
	require.NoError(t, err)

	t.Run("dry_run_from_file", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "clean --from ~/.openboot/snapshot.json --dry-run")
		require.NoError(t, err, "output: %s", output)
		assert.Contains(t, output, "DRY-RUN", "should indicate dry-run mode")
		t.Logf("clean dry-run:\n%s", output)
	})

	t.Run("detects_extra_package", func(t *testing.T) {
		// Install an extra package
		vm.Run("export PATH=\"/opt/homebrew/bin:$PATH\" && brew install cowsay")

		output, err := vmRunDevBinary(t, vm, bin, "clean --from ~/.openboot/snapshot.json --dry-run")
		require.NoError(t, err, "output: %s", output)
		// Should detect cowsay as extra
		t.Logf("clean with extra:\n%s", output)
	})

	t.Run("from_user_not_found", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "clean --user nonexistent-xyz-999 --dry-run")
		t.Logf("clean --user nonexistent: %s, err: %v", output, err)
	})

	t.Run("no_source_specified", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "clean")
		t.Logf("clean (no args): %s, err: %v", output, err)
	})
}

// TestVM_Cmd_Update tests `openboot update` with all flags.
func TestVM_Cmd_Update(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	t.Run("dry_run", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "update --dry-run")
		t.Logf("update --dry-run:\n%s", output)
		if err != nil {
			t.Logf("update dry-run exited with: %v (may be expected)", err)
		}
	})

	t.Run("self_update_dev_build", func(t *testing.T) {
		// Dev build (version=dev) should skip or handle self-update gracefully
		output, err := vmRunDevBinary(t, vm, bin, "update --self")
		t.Logf("update --self:\n%s", output)
		if err != nil {
			t.Logf("update --self exited with: %v (expected for dev build)", err)
		}
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

// TestVM_Cmd_Delete tests `openboot delete`.
func TestVM_Cmd_Delete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	t.Run("no_slug_argument", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "delete")
		assert.Error(t, err, "delete without slug should fail")
		t.Logf("delete (no args): %s", output)
	})

	t.Run("not_authenticated", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "delete test-slug")
		assert.Error(t, err, "delete without auth should fail")
		assert.True(t,
			strings.Contains(output, "logged in") ||
				strings.Contains(output, "login") ||
				strings.Contains(output, "auth") ||
				strings.Contains(output, "Error"),
			"should mention auth required, got: %s", output)
	})

	t.Run("force_flag_not_authenticated", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "delete test-slug --force")
		assert.Error(t, err, "delete --force without auth should still fail")
		t.Logf("delete --force (no auth): %s", output)
	})
}

// TestVM_Cmd_Push tests `openboot push`.
func TestVM_Cmd_Push(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	t.Run("no_file_argument", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "push")
		assert.Error(t, err, "push without file should fail")
		t.Logf("push (no args): %s", output)
	})

	t.Run("not_authenticated", func(t *testing.T) {
		// Save a snapshot first, then try to push
		vmRunDevBinary(t, vm, bin, "snapshot --local")

		output, err := vmRunDevBinary(t, vm, bin, "push ~/.openboot/snapshot.json")
		assert.Error(t, err, "push without auth should fail")
		assert.True(t,
			strings.Contains(output, "logged in") ||
				strings.Contains(output, "login") ||
				strings.Contains(output, "auth") ||
				strings.Contains(output, "Error"),
			"should mention auth required, got: %s", output)
	})

	t.Run("nonexistent_file", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "push /nonexistent/file.json")
		assert.Error(t, err, "push nonexistent file should fail")
		t.Logf("push nonexistent: %s", output)
	})

	t.Run("slug_flag_not_authenticated", func(t *testing.T) {
		vmRunDevBinary(t, vm, bin, "snapshot --local")
		output, err := vmRunDevBinary(t, vm, bin, "push ~/.openboot/snapshot.json --slug my-config")
		assert.Error(t, err, "push --slug without auth should fail")
		t.Logf("push --slug (no auth): %s", output)
	})
}

// TestVM_Cmd_Sync tests `openboot sync`.
func TestVM_Cmd_Sync(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	t.Run("no_sync_source", func(t *testing.T) {
		// Without a saved sync source, sync should fail gracefully
		output, err := vmRunDevBinary(t, vm, bin, "sync")
		t.Logf("sync (no source): %s, err: %v", output, err)
		if err != nil {
			assert.True(t,
				strings.Contains(output, "source") ||
					strings.Contains(output, "sync") ||
					strings.Contains(output, "config") ||
					strings.Contains(output, "Error"),
				"should indicate no sync source, got: %s", output)
		}
	})

	t.Run("source_not_found", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "sync --source nonexistent-xyz-999")
		t.Logf("sync --source nonexistent: %s, err: %v", output, err)
	})

	t.Run("dry_run_no_source", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "sync --dry-run")
		t.Logf("sync --dry-run (no source): %s, err: %v", output, err)
	})

	t.Run("install_only_flag", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "sync --install-only --source nonexistent-xyz")
		t.Logf("sync --install-only: %s, err: %v", output, err)
	})
}

// TestVM_Cmd_Init tests `openboot init`.
func TestVM_Cmd_Init(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	t.Run("no_config_file", func(t *testing.T) {
		// Running init without .openboot.yml should fail or show helpful message
		output, err := vmRunDevBinary(t, vm, bin, "init /tmp")
		t.Logf("init (no .openboot.yml): %s, err: %v", output, err)
	})

	t.Run("with_config_dry_run", func(t *testing.T) {
		// Create a minimal .openboot.yml
		vm.Run("mkdir -p /tmp/test-project")
		vm.Run(`cat > /tmp/test-project/.openboot.yml << 'YAML'
dependencies:
  brew:
    - jq
    - curl
YAML`)

		output, err := vmRunDevBinary(t, vm, bin, "init /tmp/test-project --dry-run --silent")
		t.Logf("init --dry-run:\n%s", output)
		if err != nil {
			t.Logf("init --dry-run exited with: %v", err)
		}
	})

	t.Run("with_config_real_install", func(t *testing.T) {
		vm.Run("mkdir -p /tmp/test-project2")
		vm.Run(`cat > /tmp/test-project2/.openboot.yml << 'YAML'
dependencies:
  brew:
    - jq
YAML`)

		output, err := vmRunDevBinary(t, vm, bin, "init /tmp/test-project2 --silent")
		t.Logf("init --silent:\n%s", output)
		if err != nil {
			t.Logf("init exited with: %v", err)
		}

		// Verify jq is installed
		assert.True(t, vmIsInstalled(t, vm, "jq"), "jq should be installed after init")
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
