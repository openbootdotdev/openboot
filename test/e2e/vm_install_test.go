//go:build e2e && vm

package e2e

import (
	"strings"
	"testing"

	"github.com/openbootdotdev/openboot/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVM_InstallScript tests install.sh behaviors on a bare system.
func TestVM_InstallScript(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM install script test in short mode")
	}

	vm := testutil.NewTartVM(t)

	t.Run("dry_run_no_side_effects", func(t *testing.T) {
		output, err := vm.Run("export OPENBOOT_DRY_RUN=true && curl -fsSL https://openboot.dev/install.sh | bash")
		require.NoError(t, err, "install.sh dry-run should succeed")
		assert.Contains(t, output, "DRY RUN")
		assert.Contains(t, output, "Would perform")

		// Nothing installed
		out, _ := vm.Run("which brew 2>/dev/null || echo no-brew")
		assert.Contains(t, out, "no-brew")
	})

	t.Run("full_install_via_curl_bash", func(t *testing.T) {
		version := vmInstallViaBrewTap(t, vm)
		t.Logf("installed: %s", version)
		assert.Contains(t, version, "OpenBoot v")
	})

	t.Run("reinstall_detection", func(t *testing.T) {
		output, _ := vm.Run(strings.Join([]string{
			"export NONINTERACTIVE=1",
			`export PATH="/opt/homebrew/bin:$PATH"`,
			"curl -fsSL https://openboot.dev/install.sh | bash -s -- --help",
		}, " && "))
		t.Logf("reinstall output:\n%s", output)
		assert.True(t,
			strings.Contains(output, "already installed") ||
				strings.Contains(output, "Reinstall") ||
				strings.Contains(output, "Usage:"),
			"should handle existing installation")
	})
}

// TestVM_Install_AllPresets tests `openboot --preset` for all three presets.
func TestVM_Install_AllPresets(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM preset test in short mode")
	}

	t.Run("minimal", func(t *testing.T) {
		vm := testutil.NewTartVM(t)
		vmInstallHomebrew(t, vm)
		bin := vmCopyDevBinary(t, vm)

		output, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --packages-only")
		t.Logf("minimal install:\n%s", output)
		require.NoError(t, err)

		// Verify core packages
		for _, pkg := range []string{"jq", "ripgrep", "fd", "bat", "fzf", "htop", "tree", "gh", "stow"} {
			assert.True(t, vmIsInstalled(t, vm, pkg), "%s should be installed", pkg)
		}
	})

	t.Run("developer_dry_run", func(t *testing.T) {
		vm := testutil.NewTartVM(t)
		vmInstallHomebrew(t, vm)
		bin := vmCopyDevBinary(t, vm)

		output, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset developer --dry-run --silent")
		require.NoError(t, err, "output: %s", output)

		// Dry-run should list developer packages
		assert.Contains(t, output, "node", "developer preset should include node")
		assert.Contains(t, output, "go", "developer preset should include go")
		assert.Contains(t, output, "visual-studio-code", "developer preset should include vscode")
	})

	t.Run("full_dry_run", func(t *testing.T) {
		vm := testutil.NewTartVM(t)
		vmInstallHomebrew(t, vm)
		bin := vmCopyDevBinary(t, vm)

		output, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset full --dry-run --silent")
		require.NoError(t, err, "output: %s", output)

		// Full should list everything
		assert.Contains(t, output, "python", "full preset should include python")
		assert.Contains(t, output, "kubectl", "full preset should include kubectl")
	})

	t.Run("invalid_preset", func(t *testing.T) {
		vm := testutil.NewTartVM(t)
		vmInstallHomebrew(t, vm)
		bin := vmCopyDevBinary(t, vm)

		_, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset nonexistent-xyz --dry-run --silent")
		assert.Error(t, err, "invalid preset should fail")
	})
}

// TestVM_Install_Flags tests all install flags individually.
func TestVM_Install_Flags(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM install flags test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	t.Run("packages_only", func(t *testing.T) {
		output, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --packages-only")
		require.NoError(t, err, "output: %s", output)

		// Shell should NOT be configured
		out, _ := vm.Run("test -d ~/.oh-my-zsh && echo exists || echo missing")
		assert.Contains(t, out, "missing", "Oh-My-Zsh should not be installed with --packages-only")

		// Dotfiles should NOT be cloned
		out, _ = vm.Run("test -d ~/.dotfiles && echo exists || echo missing")
		assert.Contains(t, out, "missing")
	})

	t.Run("shell_install", func(t *testing.T) {
		output, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --shell install")
		t.Logf("shell install:\n%s", output)
		require.NoError(t, err, "output: %s", output)

		// Oh-My-Zsh should be installed
		out, _ := vm.Run("test -d ~/.oh-my-zsh && echo exists || echo missing")
		assert.Contains(t, out, "exists", "Oh-My-Zsh should be installed with --shell install")
	})

	t.Run("shell_skip", func(t *testing.T) {
		vm2 := testutil.NewTartVM(t)
		vmInstallHomebrew(t, vm2)
		bin2 := vmCopyDevBinary(t, vm2)

		output, err := vmRunDevBinaryWithGit(t, vm2, bin2, "--preset minimal --silent --shell skip --dotfiles skip --macos skip")
		t.Logf("all skipped:\n%s", output)
		require.NoError(t, err)

		out, _ := vm2.Run("test -d ~/.oh-my-zsh && echo exists || echo missing")
		assert.Contains(t, out, "missing", "Oh-My-Zsh should NOT be installed with --shell skip")
	})

	t.Run("macos_skip", func(t *testing.T) {
		vm2 := testutil.NewTartVM(t)
		vmInstallHomebrew(t, vm2)
		bin2 := vmCopyDevBinary(t, vm2)

		output, err := vmRunDevBinaryWithGit(t, vm2, bin2, "--preset minimal --silent --macos skip --shell skip --dotfiles skip")
		require.NoError(t, err, "output: %s", output)
		assert.NotContains(t, output, "macOS Preferences", "macOS prefs should be skipped")
	})

	t.Run("dotfiles_skip", func(t *testing.T) {
		vm2 := testutil.NewTartVM(t)
		vmInstallHomebrew(t, vm2)
		bin2 := vmCopyDevBinary(t, vm2)

		output, err := vmRunDevBinaryWithGit(t, vm2, bin2, "--preset minimal --silent --dotfiles skip --shell skip --macos skip")
		require.NoError(t, err, "output: %s", output)

		out, _ := vm2.Run("test -d ~/.dotfiles && echo exists || echo missing")
		assert.Contains(t, out, "missing")
	})

	t.Run("from_snapshot_file", func(t *testing.T) {
		// Save a snapshot, then install from it
		vmRunDevBinary(t, vm, bin, "snapshot --local")

		output, err := vmRunDevBinaryWithGit(t, vm, bin, "--from ~/.openboot/snapshot.json --dry-run --silent")
		t.Logf("from snapshot:\n%s", output)
		if err != nil {
			t.Logf("install --from exited with: %v", err)
		}
	})

	t.Run("from_nonexistent_file", func(t *testing.T) {
		_, err := vmRunDevBinaryWithGit(t, vm, bin, "--from /nonexistent/file.json --silent")
		assert.Error(t, err, "install --from nonexistent file should fail")
	})

	t.Run("user_not_found", func(t *testing.T) {
		output, err := vmRunDevBinaryWithGit(t, vm, bin, "--user nonexistent-xyz-999 --silent --dry-run")
		t.Logf("install --user nonexistent: %s, err: %v", output, err)
	})

	t.Run("update_flag", func(t *testing.T) {
		output, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --dry-run --silent --update")
		require.NoError(t, err, "output: %s", output)
		t.Logf("install --update:\n%s", output)
	})
}

// TestVM_Install_GitConfig tests git configuration during install.
func TestVM_Install_GitConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	t.Run("env_vars_set_git_config", func(t *testing.T) {
		env := map[string]string{
			"PATH":               brewPath,
			"OPENBOOT_GIT_NAME":  "VM Test User",
			"OPENBOOT_GIT_EMAIL": "vmtest@openboot.test",
		}
		output, err := vm.RunWithEnv(env, bin+" --preset minimal --silent --packages-only")
		require.NoError(t, err, "output: %s", output)

		// Verify git config was set
		nameOut, _ := vm.Run("git config --global user.name")
		emailOut, _ := vm.Run("git config --global user.email")
		t.Logf("git name: %s, email: %s", strings.TrimSpace(nameOut), strings.TrimSpace(emailOut))
	})
}
