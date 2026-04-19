//go:build e2e && vm

// Package e2e contains VM-based end-to-end tests that validate openboot from
// the USER'S perspective, not the code's perspective. Each test represents a
// real user scenario and verifies EXPECTED BEHAVIOR — if the code is buggy,
// these tests should catch it.
//
// Design principles:
// - Tests verify what the user EXPECTS, not what the code DOES
// - Tests can catch bugs even if the code "works" (e.g., silent data loss)
// - Tests verify ACTUAL SYSTEM STATE, not just command output
// - Tests validate error handling from the user's perspective

package e2e

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/testutil"
)

// =============================================================================
// Scenario 1: First-time user installs openboot from scratch
// =============================================================================

// TestVM_Journey_FirstTimeUser simulates a brand new Mac user:
//
//	curl | bash → openboot installs → run preset → tools actually work
//
// User expectation: "I ran the install script, now I should have a working
// dev environment. Every tool should be runnable."
func TestVM_Journey_FirstTimeUser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping full journey test in short mode")
	}

	vm := testutil.NewMacHost(t)

	// Step 1: openboot shouldn't leak in from a prior step. We don't assert on
	// rg/fd/bat/fzf absence anymore — GitHub Actions runners vary in what
	// ships preinstalled, and the post-install checks below are the
	// load-bearing assertion.
	t.Run("bare_system_has_no_openboot", func(t *testing.T) {
		out, _ := vm.Run("which openboot 2>/dev/null || echo not-found")
		assert.Contains(t, out, "not-found", "openboot should not exist before install")
	})

	// Step 2: Install via curl | bash (the real user journey)
	t.Run("curl_bash_installs_everything", func(t *testing.T) {
		version := vmInstallViaBrewTap(t, vm)
		assert.Contains(t, version, "OpenBoot v", "should report version after install")
	})

	// Step 3: Run openboot with minimal preset
	t.Run("minimal_preset_installs_usable_tools", func(t *testing.T) {
		output, err := vmRunOpenbootWithGit(t, vm, "--preset minimal --silent --packages-only")
		t.Logf("install output:\n%s", output)
		require.NoError(t, err, "minimal preset should succeed")

		// User expectation: every tool should be USABLE, not just "in PATH"
		toolChecks := map[string]string{
			"jq":   `echo '{"a":1}' | jq '.a'`,          // Can it parse JSON?
			"rg":   `echo 'hello world' | rg 'hello'`,   // Can it search?
			"fd":   `fd --version`,                      // Does it run?
			"bat":  `echo 'test' | bat --plain`,         // Can it display?
			"fzf":  `echo 'a\nb\nc' | fzf --filter 'b'`, // Can it filter?
			"htop": `htop --version`,                    // Does it run?
			"tree": `tree --version`,                    // Does it run?
			"gh":   `gh --version`,                      // Does it run?
		}

		for name, cmd := range toolChecks {
			t.Run("tool_works_"+name, func(t *testing.T) {
				fullCmd := "export PATH=\"/opt/homebrew/bin:$PATH\" && " + cmd
				output, err := vm.Run(fullCmd)
				assert.NoError(t, err, "%s should be usable, output: %s", name, output)
			})
		}
	})
}

// =============================================================================
// Scenario 2: User expects dry-run to be completely safe
// =============================================================================

// TestVM_Journey_DryRunIsCompletelySafe verifies that --dry-run changes
// ABSOLUTELY NOTHING on the system.
//
// User expectation: "I ran --dry-run to preview. Nothing should have changed.
// Not a single file, not a single package, not a single config."
func TestVM_Journey_DryRunIsCompletelySafe(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dry-run safety test in short mode")
	}

	vm := testutil.NewMacHost(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Capture COMPLETE system state before dry-run
	beforeFormulae := vmBrewList(t, vm)
	beforeCasks := vmBrewCaskList(t, vm)
	beforeZshrc, _ := vm.Run("cat ~/.zshrc 2>/dev/null || echo NO_ZSHRC")
	beforeOhMyZsh, _ := vm.Run("test -d ~/.oh-my-zsh && echo exists || echo missing")
	beforeDotfiles, _ := vm.Run("test -d ~/.dotfiles && echo exists || echo missing")
	beforeOpenbootDir, _ := vm.Run("ls ~/.openboot/ 2>/dev/null || echo NO_DIR")
	beforeScreenshots, _ := vm.Run("test -d ~/Screenshots && echo exists || echo missing")

	// Run dry-run with FULL preset (maximum possible changes)
	output, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset full --dry-run --silent")
	t.Logf("dry-run output:\n%s", output)
	assert.NoError(t, err)

	// Verify NOTHING changed
	t.Run("no_new_formulae", func(t *testing.T) {
		after := vmBrewList(t, vm)
		assert.ElementsMatch(t, beforeFormulae, after, "dry-run should not install any formulae")
	})

	t.Run("no_new_casks", func(t *testing.T) {
		after := vmBrewCaskList(t, vm)
		assert.ElementsMatch(t, beforeCasks, after, "dry-run should not install any casks")
	})

	t.Run("zshrc_unchanged", func(t *testing.T) {
		after, _ := vm.Run("cat ~/.zshrc 2>/dev/null || echo NO_ZSHRC")
		assert.Equal(t, beforeZshrc, after, ".zshrc should not be modified")
	})

	t.Run("oh_my_zsh_not_installed", func(t *testing.T) {
		after, _ := vm.Run("test -d ~/.oh-my-zsh && echo exists || echo missing")
		assert.Equal(t, beforeOhMyZsh, after, "Oh-My-Zsh should not be installed")
	})

	t.Run("dotfiles_not_cloned", func(t *testing.T) {
		after, _ := vm.Run("test -d ~/.dotfiles && echo exists || echo missing")
		assert.Equal(t, beforeDotfiles, after, "dotfiles should not be cloned")
	})

	t.Run("openboot_dir_unchanged", func(t *testing.T) {
		after, _ := vm.Run("ls ~/.openboot/ 2>/dev/null || echo NO_DIR")
		assert.Equal(t, beforeOpenbootDir, after, "~/.openboot/ should not be modified")
	})

	t.Run("screenshots_dir_not_created", func(t *testing.T) {
		after, _ := vm.Run("test -d ~/Screenshots && echo exists || echo missing")
		assert.Equal(t, beforeScreenshots, after, "~/Screenshots should not be created")
	})
}

// =============================================================================
// Scenario 8: User expects full setup to actually configure everything
// =============================================================================

// TestVM_Journey_FullSetupConfiguresEverything verifies that a full install
// (without --packages-only) actually sets up shell, dotfiles, and macOS prefs.
//
// User expectation: "I ran openboot without any skip flags. Everything should
// be configured: Oh-My-Zsh installed, dotfiles linked, macOS prefs applied."
func TestVM_Journey_FullSetupConfiguresEverything(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping full setup test in short mode")
	}

	vm := testutil.NewMacHost(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	output, err := vmRunDevBinaryWithGit(t, vm, bin,
		"--preset minimal --silent --shell install --dotfiles clone --macos configure")
	t.Logf("full setup:\n%s", output)
	require.NoError(t, err)

	t.Run("oh_my_zsh_installed_and_functional", func(t *testing.T) {
		out, err := vm.Run("test -d ~/.oh-my-zsh && echo exists || echo missing")
		require.NoError(t, err)
		assert.Contains(t, out, "exists", "Oh-My-Zsh should be installed")

		// Verify it has the expected structure
		out, err = vm.Run("test -f ~/.oh-my-zsh/oh-my-zsh.sh && echo exists || echo missing")
		require.NoError(t, err)
		assert.Contains(t, out, "exists", "Oh-My-Zsh main script should exist")
	})

	t.Run("zshrc_has_homebrew_path", func(t *testing.T) {
		out, err := vm.Run("cat ~/.zshrc")
		require.NoError(t, err)
		// On Apple Silicon, brew shellenv MUST be in .zshrc or brew won't be in PATH
		assert.True(t,
			strings.Contains(out, "brew shellenv") || strings.Contains(out, "/opt/homebrew"),
			".zshrc should have Homebrew path for Apple Silicon, got:\n%s", out)
	})

	t.Run("dotfiles_cloned", func(t *testing.T) {
		out, _ := vm.Run("test -d ~/.dotfiles/.git && echo git-repo || echo not-a-repo")
		assert.Contains(t, out, "git-repo", "dotfiles should be a git repo")
	})

	t.Run("macos_prefs_actually_applied", func(t *testing.T) {
		// Verify specific prefs that openboot sets
		checks := map[string]struct {
			domain   string
			key      string
			expected string
		}{
			"show_all_extensions": {"NSGlobalDomain", "AppleShowAllExtensions", "1"},
			"finder_list_view":    {"com.apple.finder", "FXPreferredViewStyle", "Nlsv"},
			"dock_no_recents":     {"com.apple.dock", "show-recents", "0"},
		}

		for name, check := range checks {
			t.Run(name, func(t *testing.T) {
				out, err := vm.Run("defaults read " + check.domain + " " + check.key + " 2>/dev/null || echo not-set")
				if err == nil {
					actual := strings.TrimSpace(out)
					t.Logf("%s.%s = %s (expected %s)", check.domain, check.key, actual, check.expected)
					assert.Equal(t, check.expected, actual,
						"macOS pref %s.%s should be %s", check.domain, check.key, check.expected)
				}
			})
		}
	})

	t.Run("screenshots_dir_created", func(t *testing.T) {
		out, _ := vm.Run("test -d ~/Screenshots && echo exists || echo missing")
		assert.Contains(t, out, "exists", "~/Screenshots should be created")
	})
}
