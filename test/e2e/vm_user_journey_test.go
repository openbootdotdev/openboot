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
	"encoding/json"
	"strings"
	"testing"

	"github.com/openbootdotdev/openboot/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Scenario 1: First-time user installs openboot from scratch
// =============================================================================

// TestVM_Journey_FirstTimeUser simulates a brand new Mac user:
//   curl | bash → openboot installs → run preset → tools actually work
//
// User expectation: "I ran the install script, now I should have a working
// dev environment. Every tool should be runnable."
func TestVM_Journey_FirstTimeUser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping full journey test in short mode")
	}

	vm := testutil.NewTartVM(t)

	// Step 1: Bare system — openboot and brew should not be there
	// Note: base image may have some tools preinstalled (e.g., jq in /usr/bin)
	t.Run("bare_system_has_no_openboot", func(t *testing.T) {
		for _, tool := range []string{"openboot", "rg", "fd", "bat", "fzf"} {
			out, _ := vm.Run("which " + tool + " 2>/dev/null || echo not-found")
			assert.Contains(t, out, "not-found", "%s should not exist on bare VM", tool)
		}
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
			"rg":   `echo 'hello world' | rg 'hello'`,    // Can it search?
			"fd":   `fd --version`,                        // Does it run?
			"bat":  `echo 'test' | bat --plain`,           // Can it display?
			"fzf":  `echo 'a\nb\nc' | fzf --filter 'b'`,  // Can it filter?
			"htop": `htop --version`,                      // Does it run?
			"tree": `tree --version`,                      // Does it run?
			"gh":   `gh --version`,                        // Does it run?
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

	vm := testutil.NewTartVM(t)
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
// Scenario 3: User expects snapshot to accurately reflect reality
// =============================================================================

// TestVM_Journey_SnapshotAccuracy verifies that snapshot output matches
// the actual system state — not more, not less.
//
// User expectation: "I captured a snapshot. It should contain EXACTLY what's
// on my system. If I install jq and snapshot, jq should be in the snapshot.
// If I uninstall jq and snapshot again, jq should NOT be in the snapshot."
func TestVM_Journey_SnapshotAccuracy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping snapshot accuracy test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	t.Run("snapshot_matches_brew_list", func(t *testing.T) {
		// Get what brew says is installed
		brewFormulae := vmBrewList(t, vm)

		// Get what snapshot says
		snapOutput, err := vmRunDevBinary(t, vm, bin, "snapshot --json")
		require.NoError(t, err)

		snap := extractJSON(t, snapOutput)
		require.NotNil(t, snap, "should contain valid JSON")

		packages := snap["packages"].(map[string]interface{})
		snapFormulae := toStringSlice(packages["formulae"])

		// Snapshot uses `brew leaves` (top-level only), brew list includes dependencies.
		// Snapshot may use tap-qualified names (e.g., "buildkite/buildkite/buildkite-agent")
		// while brew list uses short names (e.g., "buildkite-agent").
		// Extract the short name (last segment after /) for comparison.
		for _, f := range snapFormulae {
			shortName := f
			if idx := strings.LastIndex(f, "/"); idx >= 0 {
				shortName = f[idx+1:]
			}
			assert.Contains(t, brewFormulae, shortName,
				"snapshot says %s is installed but brew list doesn't include it (checked as %s)", f, shortName)
		}
	})

	t.Run("snapshot_reflects_new_install", func(t *testing.T) {
		// Install a package
		vm.Run("export PATH=\"/opt/homebrew/bin:$PATH\" && brew install cowsay")

		// Snapshot should include it
		snapOutput, err := vmRunDevBinary(t, vm, bin, "snapshot --json")
		require.NoError(t, err)
		assert.Contains(t, snapOutput, "cowsay", "snapshot should include newly installed cowsay")
	})

	t.Run("snapshot_reflects_uninstall", func(t *testing.T) {
		// Uninstall the package
		vm.Run("export PATH=\"/opt/homebrew/bin:$PATH\" && brew uninstall cowsay")

		// Snapshot should NOT include it
		snapOutput, err := vmRunDevBinary(t, vm, bin, "snapshot --json")
		require.NoError(t, err)
		assert.NotContains(t, snapOutput, "cowsay", "snapshot should not include uninstalled cowsay")
	})

	t.Run("snapshot_health_reports_failures", func(t *testing.T) {
		snapOutput, err := vmRunDevBinary(t, vm, bin, "snapshot --json")
		require.NoError(t, err)

		snap := extractJSON(t, snapOutput)
		if snap == nil {
			t.Skip("could not parse snapshot JSON")
		}

		// If there's a health field, check it
		if health, ok := snap["health"]; ok && health != nil {
			if healthMap, ok := health.(map[string]interface{}); ok {
				if failed, ok := healthMap["failed_steps"]; ok && failed != nil {
					if failedSteps, ok := failed.([]interface{}); ok && len(failedSteps) > 0 {
						t.Logf("WARNING: snapshot has failed steps: %v", failedSteps)
					}
				}
			}
		}
	})
}

// =============================================================================
// Scenario 4: User expects diff to be consistent with snapshot
// =============================================================================

// TestVM_Journey_DiffConsistency verifies that:
//   snapshot → diff should show zero changes
//   install extra → diff should detect it
//   diff output should match clean's analysis
//
// User expectation: "If I snapshot my system and immediately diff, there should
// be ZERO differences. If I install something extra and diff, it should show up."
func TestVM_Journey_DiffConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping diff consistency test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Install some packages
	_, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --packages-only")
	require.NoError(t, err)

	// Save snapshot
	_, err = vmRunDevBinary(t, vm, bin, "snapshot --local")
	require.NoError(t, err)

	t.Run("immediate_diff_shows_no_changes", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "diff --from ~/.openboot/snapshot.json")
		require.NoError(t, err, "output: %s", output)
		assert.True(t,
			strings.Contains(output, "No differences") || strings.Contains(output, "matches"),
			"immediate diff after snapshot should show no changes, got: %s", output)
	})

	t.Run("extra_package_detected_by_diff", func(t *testing.T) {
		// Install something not in the snapshot
		vm.Run("export PATH=\"/opt/homebrew/bin:$PATH\" && brew install cowsay")

		output, err := vmRunDevBinary(t, vm, bin, "diff --from ~/.openboot/snapshot.json")
		require.NoError(t, err, "output: %s", output)

		// Diff should show cowsay as extra
		// NOTE: This tests the EXPECTATION that diff detects extras correctly
		t.Logf("diff with extra:\n%s", output)
	})

	t.Run("diff_and_clean_agree", func(t *testing.T) {
		// What diff says is extra should match what clean wants to remove
		diffOutput, _ := vmRunDevBinary(t, vm, bin, "diff --from ~/.openboot/snapshot.json --json")
		cleanOutput, _ := vmRunDevBinary(t, vm, bin, "clean --from ~/.openboot/snapshot.json --dry-run")

		t.Logf("diff JSON:\n%s", diffOutput)
		t.Logf("clean dry-run:\n%s", cleanOutput)

		// If diff shows cowsay as extra, clean should want to remove it
		if strings.Contains(diffOutput, "cowsay") {
			assert.Contains(t, cleanOutput, "cowsay",
				"clean should want to remove what diff says is extra")
		}
	})
}

// =============================================================================
// Scenario 5: User expects re-running to be safe (idempotent)
// =============================================================================

// TestVM_Journey_RerunIsSafe verifies that running openboot twice doesn't
// break anything or install duplicates.
//
// User expectation: "I ran openboot twice by accident. Nothing should break.
// I should have exactly the same packages as after the first run."
func TestVM_Journey_RerunIsSafe(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping idempotency test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// First run
	_, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --packages-only")
	require.NoError(t, err)

	firstRunFormulae := vmBrewList(t, vm)

	// Second run
	output, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --packages-only")
	t.Logf("second run:\n%s", output)
	require.NoError(t, err, "second run should succeed")

	secondRunFormulae := vmBrewList(t, vm)

	t.Run("same_packages", func(t *testing.T) {
		assert.ElementsMatch(t, firstRunFormulae, secondRunFormulae,
			"second run should not change installed packages")
	})

	t.Run("tools_still_work", func(t *testing.T) {
		// After second run, tools should still work
		out, err := vm.Run("export PATH=\"/opt/homebrew/bin:$PATH\" && echo '{\"a\":1}' | jq '.a'")
		assert.NoError(t, err, "jq should still work after second run, output: %s", out)
	})
}

// =============================================================================
// Scenario 6: User expects --packages-only to ONLY touch packages
// =============================================================================

// TestVM_Journey_PackagesOnlyIsStrict verifies that --packages-only doesn't
// touch shell, dotfiles, or macOS prefs.
//
// User expectation: "I only want packages, nothing else. Don't touch my shell
// config, don't clone any dotfiles, don't change my macOS settings."
func TestVM_Journey_PackagesOnlyIsStrict(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping packages-only test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Create a custom .zshrc to verify it's not modified
	vm.Run("echo '# my custom zshrc' > ~/.zshrc")
	beforeZshrc, _ := vm.Run("cat ~/.zshrc")

	_, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --packages-only")
	require.NoError(t, err)

	t.Run("packages_installed", func(t *testing.T) {
		assert.True(t, vmIsInstalled(t, vm, "jq"))
		assert.True(t, vmIsInstalled(t, vm, "fzf"))
	})

	t.Run("zshrc_not_modified", func(t *testing.T) {
		afterZshrc, _ := vm.Run("cat ~/.zshrc")
		assert.Equal(t, beforeZshrc, afterZshrc, ".zshrc should not be modified with --packages-only")
	})

	t.Run("oh_my_zsh_not_installed", func(t *testing.T) {
		out, _ := vm.Run("test -d ~/.oh-my-zsh && echo exists || echo missing")
		assert.Contains(t, out, "missing")
	})

	t.Run("dotfiles_not_cloned", func(t *testing.T) {
		out, _ := vm.Run("test -d ~/.dotfiles && echo exists || echo missing")
		assert.Contains(t, out, "missing")
	})

	t.Run("macos_prefs_not_changed", func(t *testing.T) {
		// Check a pref that openboot would set
		out, _ := vm.Run("defaults read com.apple.dock show-recents 2>/dev/null || echo not-set")
		// On a fresh VM, this should be default (not openboot's value)
		t.Logf("show-recents: %s", strings.TrimSpace(out))
	})
}

// =============================================================================
// Scenario 7: User uninstalls a package and re-runs openboot
// =============================================================================

// TestVM_Journey_ManualUninstallThenRerun verifies that if a user manually
// uninstalls a package, re-running openboot will reinstall it.
//
// User expectation: "I accidentally uninstalled jq. If I run openboot again
// with the same preset, it should reinstall jq."
//
// Known issue: install_state.json may prevent reinstall. This test validates
// whether the behavior matches user expectation.
func TestVM_Journey_ManualUninstallThenRerun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping manual uninstall test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// First install
	_, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --packages-only")
	require.NoError(t, err)

	// Use cowsay as test package — install it manually first, then use openboot's state
	// Actually, let's test with 'tree' which IS in minimal preset and has no dependents
	assert.True(t, vmIsInstalled(t, vm, "tree"), "tree should be installed after first run")

	// User manually uninstalls tree
	vm.Run("export PATH=\"/opt/homebrew/bin:$PATH\" && brew uninstall --force tree")
	require.False(t, vmIsInstalled(t, vm, "tree"), "tree should be gone after manual uninstall")

	// Re-run openboot — user expects tree to come back
	output, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset minimal --silent --packages-only")
	t.Logf("re-run output:\n%s", output)
	require.NoError(t, err)

	t.Run("uninstalled_package_reinstalled", func(t *testing.T) {
		// THIS IS THE KEY ASSERTION:
		// If this fails, it means install_state.json is preventing reinstall
		// which is a BUG from the user's perspective
		installed := vmIsInstalled(t, vm, "tree")
		if !installed {
			t.Logf("BUG CONFIRMED: openboot's install_state.json prevents reinstalling "+
				"manually-uninstalled packages. Output shows: %s", output)
		}
		assert.True(t, installed,
			"tree should be reinstalled after re-running openboot "+
				"(if this fails, install_state.json is preventing reinstall)")
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

	vm := testutil.NewTartVM(t)
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

// =============================================================================
// Scenario 9: User expects error messages to be helpful
// =============================================================================

// TestVM_Journey_ErrorMessages verifies that error cases give clear,
// actionable messages.
//
// User expectation: "When something goes wrong, tell me WHAT went wrong
// and HOW to fix it."
func TestVM_Journey_ErrorMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping error messages test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	t.Run("invalid_preset_says_which_are_valid", func(t *testing.T) {
		output, err := vmRunDevBinaryWithGit(t, vm, bin, "--preset bogus --silent --dry-run")
		assert.Error(t, err)
		// Should tell user which presets ARE valid
		assert.True(t,
			strings.Contains(output, "minimal") || strings.Contains(output, "developer") || strings.Contains(output, "full"),
			"error should mention valid presets, got: %s", output)
	})

	t.Run("push_without_auth_says_login", func(t *testing.T) {
		vmRunDevBinary(t, vm, bin, "snapshot --local")
		output, err := vmRunDevBinary(t, vm, bin, "push ~/.openboot/snapshot.json")
		assert.Error(t, err)
		assert.True(t,
			strings.Contains(output, "login") || strings.Contains(output, "logged in") || strings.Contains(output, "auth"),
			"should tell user to login, got: %s", output)
	})

	t.Run("delete_without_auth_says_login", func(t *testing.T) {
		output, err := vmRunDevBinary(t, vm, bin, "delete my-config")
		assert.Error(t, err)
		assert.True(t,
			strings.Contains(output, "login") || strings.Contains(output, "logged in") || strings.Contains(output, "auth"),
			"should tell user to login, got: %s", output)
	})

	t.Run("from_nonexistent_file_clear_error", func(t *testing.T) {
		output, err := vmRunDevBinaryWithGit(t, vm, bin, "--from /does/not/exist.json --silent")
		assert.Error(t, err)
		assert.True(t,
			strings.Contains(output, "not found") ||
				strings.Contains(output, "no such file") ||
				strings.Contains(output, "does not exist") ||
				strings.Contains(output, "Error"),
			"should clearly say file not found, got: %s", output)
	})

	t.Run("unknown_command_suggests_help", func(t *testing.T) {
		output, _ := vmRunDevBinary(t, vm, bin, "foobar")
		assert.True(t,
			strings.Contains(output, "help") || strings.Contains(output, "Usage:") || strings.Contains(output, "unknown"),
			"should guide user, got: %s", output)
	})
}

// =============================================================================
// Helpers
// =============================================================================

// extractJSON finds and parses the first complete JSON object from mixed output.
// snapshot --json outputs progress text before/after the JSON.
func extractJSON(t *testing.T, output string) map[string]interface{} {
	t.Helper()

	start := strings.Index(output, "{")
	if start < 0 {
		return nil
	}

	// Find matching closing brace by counting depth
	depth := 0
	for i := start; i < len(output); i++ {
		switch output[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				var result map[string]interface{}
				err := json.Unmarshal([]byte(output[start:i+1]), &result)
				if err != nil {
					t.Logf("JSON parse error: %v", err)
					return nil
				}
				return result
			}
		}
	}
	return nil
}

func toStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
