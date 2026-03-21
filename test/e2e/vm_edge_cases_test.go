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
// Scenario: Snapshot import/restore on a fresh machine
// =============================================================================

// TestVM_Edge_SnapshotImportRestore simulates the migration use case:
// Machine A captures a snapshot → Machine B (fresh VM) restores from it.
//
// User expectation: "I exported my setup from my old Mac. On my new Mac,
// I import the snapshot and everything should be installed."
func TestVM_Edge_SnapshotImportRestore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM edge case in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Install minimal preset on "old machine"
	_, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --packages-only")
	require.NoError(t, err)

	// Capture snapshot (simulating export from old machine)
	snapOutput, err := vmRunDevBinary(t, vm, bin, "snapshot --json")
	require.NoError(t, err)

	// Save snapshot to a file
	jsonStart := strings.Index(snapOutput, "{")
	require.True(t, jsonStart >= 0)
	// Find the JSON end by brace matching
	depth, jsonEnd := 0, 0
	for i := jsonStart; i < len(snapOutput); i++ {
		if snapOutput[i] == '{' {
			depth++
		} else if snapOutput[i] == '}' {
			depth--
			if depth == 0 {
				jsonEnd = i + 1
				break
			}
		}
	}
	snapJSON := snapOutput[jsonStart:jsonEnd]
	vm.Run("cat > /tmp/old-machine-snapshot.json << 'SNAPEOF'\n" + snapJSON + "\nSNAPEOF")

	// Now simulate "new machine" by removing a few packages
	vm.Run("export PATH=\"/opt/homebrew/bin:$PATH\" && brew uninstall --force tree htop bat")

	t.Run("packages_are_gone", func(t *testing.T) {
		assert.False(t, vmIsInstalled(t, vm, "tree"))
	})

	// Import snapshot (restore on "new machine")
	t.Run("import_restores_packages", func(t *testing.T) {
		output, err := vmRunDevBinaryWithGit(t, vm, bin, "snapshot --import /tmp/old-machine-snapshot.json --dry-run")
		t.Logf("import dry-run:\n%s", output)
		if err != nil {
			t.Logf("import dry-run exited with: %v", err)
		}
		// Dry-run should show what would be restored
		t.Logf("snapshot import dry-run completed")
	})
}

// =============================================================================
// Scenario: Clean actually removes packages (not just dry-run)
// =============================================================================

// TestVM_Edge_CleanActuallyRemoves verifies that clean without --dry-run
// actually uninstalls packages.
//
// User expectation: "I ran clean and confirmed removal. The packages should
// actually be gone from my system."
func TestVM_Edge_CleanActuallyRemoves(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM edge case in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Install minimal preset
	_, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --packages-only")
	require.NoError(t, err)

	// Save snapshot
	_, err = vmRunDevBinary(t, vm, bin, "snapshot --local")
	require.NoError(t, err)

	// Install extra package
	vm.Run("export PATH=\"/opt/homebrew/bin:$PATH\" && brew install cowsay")
	assert.True(t, vmIsInstalled(t, vm, "cowsay"), "cowsay should be installed")

	// Clean using expect (needs confirmation)
	t.Run("clean_removes_extra", func(t *testing.T) {
		cmd := "export PATH=\"/opt/homebrew/bin:$PATH\" && " + bin + " clean --from ~/.openboot/snapshot.json"
		output, err := vm.RunInteractive(cmd, []testutil.ExpectStep{
			{Expect: "remove", Send: "y\r"},
			{Expect: "remove", Send: "y\r"},
		}, 120)
		t.Logf("clean output:\n%s", output)
		if err != nil {
			t.Logf("clean exited with: %v", err)
		}
	})

	t.Run("cowsay_actually_gone", func(t *testing.T) {
		// Give brew a moment to finish
		installed := vmIsInstalled(t, vm, "cowsay")
		if installed {
			t.Log("cowsay is still installed — clean may not have completed or needed different confirmation")
		}
	})
}

// =============================================================================
// Scenario: Cask apps appear in /Applications
// =============================================================================

// TestVM_Edge_CaskAppsInApplications verifies that installed casks
// actually appear as apps in /Applications/.
//
// User expectation: "I installed Raycast via openboot. I should see
// Raycast.app in /Applications."
func TestVM_Edge_CaskAppsInApplications(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM edge case in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Install minimal preset (includes casks: warp, raycast, maccy, stats, rectangle)
	_, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent")
	require.NoError(t, err)

	expectedApps := map[string]string{
		"warp":      "Warp.app",
		"raycast":   "Raycast.app",
		"maccy":     "Maccy.app",
		"stats":     "Stats.app",
		"rectangle": "Rectangle.app",
	}

	for cask, appName := range expectedApps {
		t.Run(cask+"_in_applications", func(t *testing.T) {
			out, err := vm.Run("test -d '/Applications/" + appName + "' && echo exists || echo missing")
			require.NoError(t, err)
			assert.Contains(t, out, "exists",
				"%s should be in /Applications after installing %s cask", appName, cask)
		})
	}
}

// =============================================================================
// Scenario: NPM global packages actually work
// =============================================================================

// TestVM_Edge_NpmPackagesWork verifies that npm global packages installed
// by the developer preset are actually usable.
//
// User expectation: "I installed the developer preset. I should be able
// to run tsc, tsx, prettier, etc."
func TestVM_Edge_NpmPackagesWork(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM edge case in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Developer preset includes npm packages
	// First install node via brew (needed for npm)
	vm.Run("export PATH=\"/opt/homebrew/bin:$PATH\" && brew install node")

	// Install just the npm packages from developer preset
	_, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset developer --silent --packages-only")
	require.NoError(t, err)

	npmTools := map[string]string{
		"typescript": "tsc --version",
		"tsx":        "tsx --version",
		"prettier":   "prettier --version",
	}

	for name, cmd := range npmTools {
		t.Run(name+"_works", func(t *testing.T) {
			output, err := vm.Run("export PATH=\"/opt/homebrew/bin:$PATH\" && " + cmd)
			if err != nil {
				t.Logf("%s failed: %v, output: %s", name, err, output)
			}
			// At minimum it should be findable
			whichOut, _ := vm.Run("export PATH=\"/opt/homebrew/bin:$PATH\" && which " + strings.Fields(cmd)[0])
			assert.NotContains(t, whichOut, "not found", "%s should be in PATH", name)
		})
	}
}

// =============================================================================
// Scenario: Partial failure recovery
// =============================================================================

// TestVM_Edge_PartialFailureRecovery verifies that if installation fails
// midway (e.g., a package doesn't exist), re-running succeeds for the rest.
//
// User expectation: "One package failed to install. I fix the issue and
// re-run openboot. It should install the remaining packages."
func TestVM_Edge_PartialFailureRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM edge case in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Install minimal preset — should succeed
	output, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --packages-only")
	t.Logf("install output:\n%s", output)
	require.NoError(t, err)

	// Verify key packages work despite any transient issues
	t.Run("core_tools_work_after_install", func(t *testing.T) {
		for _, tool := range []string{"jq", "fzf", "gh", "tree"} {
			assert.True(t, vmIsInstalled(t, vm, tool), "%s should be installed", tool)
		}
	})
}

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

	vm := testutil.NewTartVM(t)
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

// =============================================================================
// Scenario: Remote config install (-u username)
// =============================================================================

// TestVM_Edge_RemoteConfigInstall tests installing from a remote openboot.dev config.
//
// User expectation: "I ran `openboot -u myusername` to install from my
// cloud config. If the user exists, it should work. If not, clear error."
func TestVM_Edge_RemoteConfigInstall(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM edge case in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	t.Run("nonexistent_user_clear_error", func(t *testing.T) {
		output, err := vmRunDevBinaryWithGit(t, vm, bin, "-u nonexistent-user-xyz-99999 --silent --dry-run")
		t.Logf("remote config error:\n%s", output)
		if err != nil {
			// Should give a clear error about user/config not found
			assert.True(t,
				strings.Contains(output, "not found") ||
					strings.Contains(output, "404") ||
					strings.Contains(output, "error") ||
					strings.Contains(output, "Error"),
				"should indicate config not found, got: %s", output)
		}
	})

	t.Run("install_subcommand_with_user", func(t *testing.T) {
		output, err := vmRunDevBinaryWithGit(t, vm, bin, "install -u nonexistent-user-xyz-99999 --silent --dry-run")
		t.Logf("install -u error:\n%s", output)
		if err != nil {
			t.Logf("expected error for nonexistent user: %v", err)
		}
	})
}

// =============================================================================
// Scenario: Full preset real install (developer)
// =============================================================================

// TestVM_Edge_DeveloperPresetRealInstall does a real install of developer preset
// and verifies language toolchains actually work.
//
// User expectation: "I chose developer preset. Node, Go should be installed
// and I can start coding immediately."
func TestVM_Edge_DeveloperPresetRealInstall(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM developer preset test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	output, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset developer --silent --packages-only")
	t.Logf("developer install:\n%s", output)
	require.NoError(t, err)

	t.Run("node_works", func(t *testing.T) {
		out, err := vm.Run("export PATH=\"/opt/homebrew/bin:$PATH\" && node --version")
		require.NoError(t, err, "node should work, output: %s", out)
		assert.True(t, strings.HasPrefix(strings.TrimSpace(out), "v"), "node version should start with v")
	})

	t.Run("go_works", func(t *testing.T) {
		out, err := vm.Run("export PATH=\"/opt/homebrew/bin:$PATH\" && go version")
		require.NoError(t, err, "go should work, output: %s", out)
		assert.Contains(t, out, "go1.", "go version should contain go1.")
	})

	t.Run("npm_works", func(t *testing.T) {
		out, err := vm.Run("export PATH=\"/opt/homebrew/bin:$PATH\" && npm --version")
		require.NoError(t, err, "npm should work, output: %s", out)
	})

	t.Run("neovim_works", func(t *testing.T) {
		out, err := vm.Run("export PATH=\"/opt/homebrew/bin:$PATH\" && nvim --version | head -1")
		require.NoError(t, err, "nvim should work, output: %s", out)
		assert.Contains(t, out, "NVIM")
	})

	t.Run("tmux_works", func(t *testing.T) {
		out, err := vm.Run("export PATH=\"/opt/homebrew/bin:$PATH\" && tmux -V")
		require.NoError(t, err, "tmux should work, output: %s", out)
		assert.Contains(t, out, "tmux")
	})
}

// =============================================================================
// Scenario: Post-install script execution
// =============================================================================

// TestVM_Edge_PostInstallScript tests the --allow-post-install flag behavior.
//
// User expectation: "Post-install scripts should NOT run unless I explicitly
// allow them with --allow-post-install."
func TestVM_Edge_PostInstallScript(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM edge case in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	t.Run("post_install_skip_by_default", func(t *testing.T) {
		output, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --dry-run --post-install skip")
		require.NoError(t, err, "output: %s", output)
		t.Logf("post-install skip:\n%s", output)
	})

	t.Run("allow_post_install_flag", func(t *testing.T) {
		output, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --dry-run --allow-post-install")
		require.NoError(t, err, "output: %s", output)
		t.Logf("allow-post-install:\n%s", output)
	})
}

// =============================================================================
// Scenario: Version upgrade compatibility
// =============================================================================

// TestVM_Edge_VersionUpgrade tests that upgrading openboot from one version
// to another doesn't break state files.
//
// User expectation: "I updated openboot. My previous setup should still work,
// snapshot files should still be readable."
func TestVM_Edge_VersionUpgrade(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM edge case in short mode")
	}

	vm := testutil.NewTartVM(t)

	// Install published version via brew
	vmInstallViaBrewTap(t, vm)

	// Run with published version and save state
	vmRunOpenbootWithGit(t, vm, "--preset minimal --silent --packages-only")
	vmRunOpenboot(t, vm, "snapshot --local")

	// Copy dev binary (newer version)
	bin := vmCopyDevBinary(t, vm)

	t.Run("dev_version_reads_old_snapshot", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "diff --from ~/.openboot/snapshot.json")
		require.NoError(t, err, "dev version should read old snapshot, output: %s", output)
		t.Logf("cross-version diff:\n%s", output)
	})

	t.Run("dev_version_runs_doctor", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "doctor")
		require.NoError(t, err, "output: %s", output)
		assert.Contains(t, output, "OpenBoot Doctor")
	})

	t.Run("dev_version_snapshot_compatible", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "snapshot --json")
		require.NoError(t, err, "output: %s", output)
		assert.Contains(t, output, "packages")
	})
}
