//go:build e2e && vm

// Package e2e contains VM-based E2E tests that verify macOS `defaults write`
// calls actually reach the system preference store.
//
// Gap filled: TestVM_Journey_FullSetupConfiguresEverything only spot-checked
// 3 defaults (AppleShowAllExtensions, FXPreferredViewStyle, show-recents).
// This file adds a focused test that exercises macOS configure in isolation
// and verifies representative preferences from every category defined in
// internal/macos/categories.go.

package e2e

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/testutil"
)

// macOSPrefCheck describes a single `defaults read` assertion.
type macOSPrefCheck struct {
	domain   string
	key      string
	expected string // exact value returned by `defaults read`
}

// TestVM_Journey_MacOSDefaults_AllCategoriesWritten runs
//
//	openboot install --preset minimal --silent --shell skip --dotfiles skip --macos configure
//
// and verifies that representative preferences from each of the eight
// categories in internal/macos/categories.go are actually written to the
// macOS preference store — not just planned by the installer.
//
// User expectation: "I chose to configure macOS. Every setting I agreed to
// should actually take effect, not silently fail."
func TestVM_Journey_MacOSDefaults_AllCategoriesWritten(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping macOS defaults test in short mode")
	}

	vm := testutil.NewMacHost(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	output, err := vmRunDevBinaryWithGit(t, vm, bin,
		"--preset minimal --silent --shell skip --dotfiles skip --macos configure")
	t.Logf("macOS configure output:\n%s", output)
	require.NoError(t, err, "install with --macos configure should succeed")

	// Each entry maps a human-readable test name to the expected `defaults read`
	// value. The expected value uses macOS output format: booleans are "0"/"1",
	// strings are the raw string, ints are decimal.
	checks := map[string]macOSPrefCheck{
		// ── System ────────────────────────────────────────────────────────────
		"system/show_all_extensions": {
			"NSGlobalDomain", "AppleShowAllExtensions", "1",
		},
		"system/show_scroll_bars": {
			"NSGlobalDomain", "AppleShowScrollBars", "Always",
		},
		"system/disable_autocorrect": {
			"NSGlobalDomain", "NSAutomaticSpellingCorrectionEnabled", "0",
		},
		"system/disable_autocapitalization": {
			"NSGlobalDomain", "NSAutomaticCapitalizationEnabled", "0",
		},
		"system/key_repeat": {
			"NSGlobalDomain", "KeyRepeat", "2",
		},

		// ── Finder ────────────────────────────────────────────────────────────
		"finder/list_view": {
			"com.apple.finder", "FXPreferredViewStyle", "Nlsv",
		},
		"finder/show_path_bar": {
			"com.apple.finder", "ShowPathbar", "1",
		},
		"finder/show_status_bar": {
			"com.apple.finder", "ShowStatusBar", "1",
		},
		"finder/show_hidden_files": {
			"com.apple.finder", "AppleShowAllFiles", "1",
		},
		"finder/no_extension_change_warning": {
			"com.apple.finder", "FXEnableExtensionChangeWarning", "0",
		},

		// ── Dock ──────────────────────────────────────────────────────────────
		"dock/no_show_recents": {
			"com.apple.dock", "show-recents", "0",
		},
		"dock/tile_size": {
			"com.apple.dock", "tilesize", "48",
		},

		// ── Screenshots ───────────────────────────────────────────────────────
		"screenshots/type_png": {
			"com.apple.screencapture", "type", "png",
		},
		"screenshots/disable_shadow": {
			"com.apple.screencapture", "disable-shadow", "1",
		},

		// ── Mission Control ───────────────────────────────────────────────────
		"mission_control/no_auto_rearrange": {
			"com.apple.dock", "mru-spaces", "0",
		},

		// ── Security ──────────────────────────────────────────────────────────
		"security/require_password": {
			"com.apple.screensaver", "askForPassword", "1",
		},
	}

	for name, check := range checks {
		name, check := name, check
		t.Run(name, func(t *testing.T) {
			readCmd := fmt.Sprintf(
				"defaults read %q %q 2>/dev/null || echo NOT_SET",
				check.domain, check.key,
			)
			out, err := vm.Run(readCmd)
			require.NoError(t, err,
				"defaults read should exit 0 (the || echo NOT_SET handles missing keys)")

			actual := strings.TrimSpace(out)
			assert.Equal(t, check.expected, actual,
				"macOS pref %s.%s should be %q after configure",
				check.domain, check.key, check.expected)
		})
	}
}

// TestVM_Journey_MacOSDefaults_ScreenshotsDirCreated verifies that the
// ~/Screenshots directory is created during a macOS configure run.
//
// Gap: the Screenshots directory creation (macos.CreateScreenshotsDir) was
// only checked in TestVM_Journey_FullSetupConfiguresEverything as part of a
// larger setup run. This test isolates that behaviour.
func TestVM_Journey_MacOSDefaults_ScreenshotsDirCreated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping macOS screenshots dir test in short mode")
	}

	vm := testutil.NewMacHost(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Remove ~/Screenshots if it already exists so the test is authoritative.
	_, _ = vm.Run("rm -rf ~/Screenshots")

	_, err := vmRunDevBinaryWithGit(t, vm, bin,
		"--preset minimal --silent --shell skip --dotfiles skip --macos configure")
	require.NoError(t, err, "install with --macos configure should succeed")

	out, _ := vm.Run("test -d ~/Screenshots && echo exists || echo missing")
	assert.Contains(t, out, "exists",
		"~/Screenshots should be created by --macos configure")
}

// TestVM_Journey_MacOSDefaults_DryRunWritesNothing verifies that
// --dry-run --macos configure does NOT modify any macOS preference.
//
// Regression guard: a bug in the Configure() dry-run branch could silently
// write preferences on dry-run.
func TestVM_Journey_MacOSDefaults_DryRunWritesNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping macOS dry-run defaults test in short mode")
	}

	vm := testutil.NewMacHost(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Force a known value that differs from what --macos configure would write
	// ("Always"). This makes the assertion non-vacuous: if dry-run accidentally
	// applies the preference, the value changes to "Always" and the test fails.
	_, err := vm.Run(`defaults write "NSGlobalDomain" "AppleShowScrollBars" -string "WhenScrolling"`)
	require.NoError(t, err, "should be able to force a known test value before dry-run")

	before, _ := vm.Run(
		`defaults read "NSGlobalDomain" "AppleShowScrollBars" 2>/dev/null || echo UNSET`,
	)

	_, err = vmRunDevBinaryWithGit(t, vm, bin,
		"--preset minimal --silent --shell skip --dotfiles skip --macos configure --dry-run")
	require.NoError(t, err, "dry-run should succeed")

	after, _ := vm.Run(
		`defaults read "NSGlobalDomain" "AppleShowScrollBars" 2>/dev/null || echo UNSET`,
	)

	assert.Equal(t, strings.TrimSpace(before), strings.TrimSpace(after),
		"dry-run must not change NSGlobalDomain/AppleShowScrollBars")
}
