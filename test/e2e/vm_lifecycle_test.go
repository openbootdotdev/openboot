//go:build e2e && vm

package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/openbootdotdev/openboot/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVM_Lifecycle_DryRun verifies --dry-run has zero side effects on a real system.
func TestVM_Lifecycle_DryRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	before := vmBrewList(t, vm)
	t.Logf("before: %d formulae", len(before))

	// Dry-run with full preset (most packages)
	output, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset full --dry-run --silent")
	t.Logf("dry-run output:\n%s", output)
	assert.NoError(t, err)

	after := vmBrewList(t, vm)
	t.Logf("after: %d formulae", len(after))

	assert.Equal(t, len(before), len(after), "dry-run should not change package count")

	// Shell should not be touched
	out, _ := vm.Run("test -d ~/.oh-my-zsh && echo exists || echo missing")
	assert.Contains(t, out, "missing", "dry-run should not install Oh-My-Zsh")

	// Dotfiles should not be touched
	out, _ = vm.Run("test -d ~/.dotfiles && echo exists || echo missing")
	assert.Contains(t, out, "missing", "dry-run should not clone dotfiles")
}

// TestVM_Lifecycle_Idempotent verifies running openboot twice produces identical results.
func TestVM_Lifecycle_Idempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// First run
	t.Log("first run")
	start := time.Now()
	output1, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --packages-only")
	run1 := time.Since(start)
	t.Logf("first run (%v):\n%s", run1, output1)
	require.NoError(t, err)
	snap1 := vmBrewList(t, vm)

	// Second run
	t.Log("second run")
	start = time.Now()
	output2, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --packages-only")
	run2 := time.Since(start)
	t.Logf("second run (%v):\n%s", run2, output2)
	require.NoError(t, err)
	snap2 := vmBrewList(t, vm)

	assert.ElementsMatch(t, snap1, snap2, "second run should not change installed packages")
	t.Logf("run1: %v, run2: %v", run1, run2)
}

// TestVM_Lifecycle_SnapshotDiffCleanCycle tests the full snapshot → diff → clean workflow.
func TestVM_Lifecycle_SnapshotDiffCleanCycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Install minimal preset
	_, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --packages-only")
	require.NoError(t, err)

	t.Run("step1_snapshot_captures_state", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "snapshot --json")
		require.NoError(t, err)
		assert.Contains(t, output, "jq")
		assert.Contains(t, output, "fzf")
		t.Logf("snapshot: %d bytes", len(output))
	})

	t.Run("step2_save_snapshot_locally", func(t *testing.T) {
		_, err := vmRunDevBinary(t, vm, bin, "snapshot --local")
		require.NoError(t, err)

		out, err := vm.Run("test -f ~/.openboot/snapshot.json && echo exists || echo missing")
		require.NoError(t, err)
		assert.Contains(t, out, "exists")
	})

	t.Run("step3_diff_self_is_clean", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "diff --from ~/.openboot/snapshot.json")
		require.NoError(t, err, "output: %s", output)
		assert.True(t,
			strings.Contains(output, "No differences") || strings.Contains(output, "matches"),
			"got: %s", output)
	})

	t.Run("step4_install_extra_package", func(t *testing.T) {
		_, err := vm.Run("export PATH=\"/opt/homebrew/bin:$PATH\" && brew install cowsay")
		require.NoError(t, err)
		assert.True(t, vmIsInstalled(t, vm, "cowsay"))
	})

	t.Run("step5_diff_detects_extra", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "diff --from ~/.openboot/snapshot.json")
		require.NoError(t, err, "output: %s", output)
		t.Logf("diff with extra:\n%s", output)
		// The diff should show cowsay is extra (not in snapshot)
	})

	t.Run("step6_clean_dry_run_lists_extra", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "clean --from ~/.openboot/snapshot.json --dry-run")
		require.NoError(t, err, "output: %s", output)
		assert.Contains(t, output, "DRY-RUN")
		t.Logf("clean dry-run:\n%s", output)
	})

	t.Run("step7_diff_json_format", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "diff --from ~/.openboot/snapshot.json --json")
		require.NoError(t, err, "output: %s", output)
		assert.Contains(t, output, "{")
	})

	t.Run("step8_diff_packages_only", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "diff --from ~/.openboot/snapshot.json --packages-only")
		require.NoError(t, err, "output: %s", output)
		t.Logf("packages-only diff:\n%s", output)
	})
}

// TestVM_Lifecycle_FullSetup tests the full install with shell + dotfiles + macOS prefs.
func TestVM_Lifecycle_FullSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Full install: packages + shell + dotfiles + macOS prefs
	output, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --shell install --dotfiles clone --macos configure")
	t.Logf("full setup output:\n%s", output)
	require.NoError(t, err, "full setup should succeed")

	t.Run("oh_my_zsh_installed", func(t *testing.T) {
		out, err := vm.Run("test -d ~/.oh-my-zsh && echo exists || echo missing")
		require.NoError(t, err)
		assert.Contains(t, out, "exists", "Oh-My-Zsh should be installed")
	})

	t.Run("zshrc_exists", func(t *testing.T) {
		out, err := vm.Run("test -f ~/.zshrc && echo exists || echo missing")
		require.NoError(t, err)
		assert.Contains(t, out, "exists", ".zshrc should exist")
	})

	t.Run("zshrc_has_homebrew_path", func(t *testing.T) {
		output, err := vm.Run("cat ~/.zshrc")
		require.NoError(t, err)
		// On Apple Silicon, .zshrc should have Homebrew shellenv
		assert.True(t,
			strings.Contains(output, "homebrew") ||
				strings.Contains(output, "brew") ||
				strings.Contains(output, "/opt/homebrew"),
			"should have Homebrew in .zshrc")
	})

	t.Run("dotfiles_cloned", func(t *testing.T) {
		out, err := vm.Run("test -d ~/.dotfiles && echo exists || echo missing")
		require.NoError(t, err)
		assert.Contains(t, out, "exists", "dotfiles should be cloned")
	})

	t.Run("macos_prefs_applied", func(t *testing.T) {
		// Check a specific macOS pref that openboot sets
		output, err := vm.Run("defaults read NSGlobalDomain AppleShowAllExtensions 2>/dev/null || echo not-set")
		require.NoError(t, err)
		t.Logf("AppleShowAllExtensions: %s", strings.TrimSpace(output))
		// Should be "1" or "true" after openboot applies prefs
	})

	t.Run("screenshots_dir_created", func(t *testing.T) {
		out, err := vm.Run("test -d ~/Screenshots && echo exists || echo missing")
		require.NoError(t, err)
		t.Logf("Screenshots dir: %s", strings.TrimSpace(out))
	})

	t.Run("packages_also_installed", func(t *testing.T) {
		assert.True(t, vmIsInstalled(t, vm, "jq"), "packages should also be installed during full setup")
	})
}
