package installer

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/dotfiles"
	"github.com/openbootdotdev/openboot/internal/macos"
)

func requireZsh(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/bin/zsh"); err != nil {
		t.Skip("/bin/zsh not available")
	}
}

// TestApplyMacOSPrefs_EmptyPrefs verifies that applyMacOSPrefs is a no-op when
// the plan has no macOS preferences — it returns nil without calling any macOS
// command.
func TestApplyMacOSPrefs_EmptyPrefs(t *testing.T) {
	plan := InstallPlan{
		DryRun:     false,
		MacOSPrefs: nil,
	}
	err := applyMacOSPrefs(plan, NopReporter{})
	assert.NoError(t, err)
}

// TestApplyMacOSPrefs_EmptySlice verifies that an empty (non-nil) slice also
// short-circuits without error.
func TestApplyMacOSPrefs_EmptySlice(t *testing.T) {
	plan := InstallPlan{
		DryRun:     false,
		MacOSPrefs: []macos.Preference{},
	}
	err := applyMacOSPrefs(plan, NopReporter{})
	assert.NoError(t, err)
}

// TestApplyMacOSPrefs_DryRun verifies that when DryRun=true the function
// returns nil immediately without attempting any real system call. This is
// safe to run in CI because no `defaults write` is executed.
func TestApplyMacOSPrefs_DryRun(t *testing.T) {
	plan := InstallPlan{
		DryRun: true,
		MacOSPrefs: []macos.Preference{
			{Domain: "com.apple.dock", Key: "autohide", Type: "bool", Value: "true"},
			{Domain: "com.apple.finder", Key: "ShowPathbar", Type: "bool", Value: "true"},
		},
	}
	err := applyMacOSPrefs(plan, NopReporter{})
	assert.NoError(t, err)
}

// TestApplyMacOSPrefs_DryRun_SinglePref exercises the dry-run path with a
// single preference to ensure count formatting doesn't panic.
func TestApplyMacOSPrefs_DryRun_SinglePref(t *testing.T) {
	plan := InstallPlan{
		DryRun: true,
		MacOSPrefs: []macos.Preference{
			{Domain: "com.apple.dock", Key: "tilesize", Type: "int", Value: "48"},
		},
	}
	err := applyMacOSPrefs(plan, NopReporter{})
	assert.NoError(t, err)
}

// TestApplyPostInstall_EmptyCommands verifies that applyPostInstall is a no-op
// when the plan carries no post-install commands.
func TestApplyPostInstall_EmptyCommands(t *testing.T) {
	plan := InstallPlan{
		DryRun:      false,
		PostInstall: nil,
	}
	err := applyPostInstall(plan, NopReporter{})
	assert.NoError(t, err)
}

// TestApplyPostInstall_DryRun verifies that DryRun=true causes the function to
// print the preview and skip execution.
func TestApplyPostInstall_DryRun(t *testing.T) {
	plan := InstallPlan{
		DryRun:      true,
		PostInstall: []string{"echo hello", "echo world"},
	}
	err := applyPostInstall(plan, NopReporter{})
	assert.NoError(t, err)
}

// TestApplyPostInstall_SilentNoTTY_WithoutFlag verifies that in silent / non-TTY
// mode without AllowPostInstall the script is skipped.
func TestApplyPostInstall_SilentNoTTY_WithoutFlag(t *testing.T) {
	// system.HasTTY() returns false in test environments (no controlling terminal).
	// Silent=true ensures we hit the "skip silent" guard regardless of TTY.
	plan := InstallPlan{
		DryRun:           false,
		Silent:           true,
		AllowPostInstall: false,
		PostInstall:      []string{"echo should-not-run"},
	}
	err := applyPostInstall(plan, NopReporter{})
	assert.NoError(t, err)
}

// TestApplyPostInstall_SilentWithFlag_MarkerCreated verifies that the script
// actually runs by checking for a side-effect file.
func TestApplyPostInstall_SilentWithFlag_MarkerCreated(t *testing.T) {
	requireZsh(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	markerFile := tmpDir + "/post-install-ran"
	plan := InstallPlan{
		DryRun:           false,
		Silent:           true,
		AllowPostInstall: true,
		PostInstall:      []string{"touch " + markerFile},
	}
	err := applyPostInstall(plan, NopReporter{})
	require.NoError(t, err)

	// The marker file must exist after execution.
	require.FileExists(t, markerFile)
}

// TestApplyPostInstall_CommandFailure_ReturnsError verifies that a failing
// script propagates an error.
func TestApplyPostInstall_CommandFailure_ReturnsError(t *testing.T) {
	requireZsh(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	plan := InstallPlan{
		DryRun:           false,
		Silent:           true,
		AllowPostInstall: true,
		PostInstall:      []string{"false"},
	}
	err := applyPostInstall(plan, NopReporter{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "post-install")
}

// TestApplyPostInstall_MultilineScript verifies that multi-line constructs
// (arrays, loops) are executed correctly as a single script.
func TestApplyPostInstall_MultilineScript(t *testing.T) {
	requireZsh(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	markerFile := tmpDir + "/multiline-ran"
	plan := InstallPlan{
		DryRun:           false,
		Silent:           true,
		AllowPostInstall: true,
		PostInstall: []string{
			"ITEMS=(a b c)",
			"for item in \"${ITEMS[@]}\"; do",
			"  touch " + markerFile,
			"done",
		},
	}
	err := applyPostInstall(plan, NopReporter{})
	require.NoError(t, err)
	require.FileExists(t, markerFile, "loop body must execute")
}

// TestApplyMacOSPrefs_DryRun_TableDriven is a table-driven test covering the
// main branches of applyMacOSPrefs.
func TestApplyMacOSPrefs_DryRun_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		prefs   []macos.Preference
		dryRun  bool
		wantErr bool
	}{
		{
			name:    "nil prefs no-op",
			prefs:   nil,
			dryRun:  false,
			wantErr: false,
		},
		{
			name:    "empty slice no-op",
			prefs:   []macos.Preference{},
			dryRun:  false,
			wantErr: false,
		},
		{
			name:    "dry-run with prefs",
			prefs:   []macos.Preference{{Domain: "com.apple.dock", Key: "autohide", Type: "bool", Value: "true"}},
			dryRun:  true,
			wantErr: false,
		},
		{
			name: "dry-run multiple prefs",
			prefs: []macos.Preference{
				{Domain: "com.apple.dock", Key: "autohide", Type: "bool", Value: "true"},
				{Domain: "com.apple.finder", Key: "ShowPathbar", Type: "bool", Value: "false"},
				{Domain: "NSGlobalDomain", Key: "AppleShowAllExtensions", Type: "bool", Value: "true"},
			},
			dryRun:  true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := InstallPlan{DryRun: tt.dryRun, MacOSPrefs: tt.prefs}
			err := applyMacOSPrefs(plan, NopReporter{})
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestApplyPostInstall_DryRun_TableDriven exercises applyPostInstall dry-run
// and skip paths with multiple command sets.
func TestApplyPostInstall_DryRun_TableDriven(t *testing.T) {
	tests := []struct {
		name             string
		commands         []string
		dryRun           bool
		silent           bool
		allowPostInstall bool
		wantErr          bool
	}{
		{
			name:    "nil commands no-op",
			dryRun:  false,
			wantErr: false,
		},
		{
			name:     "empty commands no-op",
			commands: []string{},
			dryRun:   false,
			wantErr:  false,
		},
		{
			name:     "dry-run skips execution",
			commands: []string{"echo hello"},
			dryRun:   true,
			wantErr:  false,
		},
		{
			name:             "silent without allow skips",
			commands:         []string{"echo hello"},
			silent:           true,
			allowPostInstall: false,
			wantErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := InstallPlan{
				DryRun:           tt.dryRun,
				Silent:           tt.silent,
				AllowPostInstall: tt.allowPostInstall,
				PostInstall:      tt.commands,
			}
			err := applyPostInstall(plan, NopReporter{})
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// applyShell
// ---------------------------------------------------------------------------

// TestApplyShell_NoOMZ verifies that when InstallOhMyZsh is false, applyShell
// skips all Oh-My-Zsh work and returns nil.
func TestApplyShell_NoOMZ_ReturnsNil(t *testing.T) {
	plan := InstallPlan{
		DryRun:         true,
		InstallOhMyZsh: false,
		// DotfilesURL empty → EnsureBrewShellenv would be called, but DryRun makes it a no-op.
		DotfilesURL: "",
	}
	err := applyShell(plan, NopReporter{})
	assert.NoError(t, err)
}

// TestApplyShell_DryRun_WithOMZ verifies that DryRun=true with OMZ requested
// returns nil without performing any real install.
func TestApplyShell_DryRun_WithOMZ(t *testing.T) {
	plan := InstallPlan{
		DryRun:         true,
		InstallOhMyZsh: true,
		DotfilesURL:    "",
	}
	err := applyShell(plan, NopReporter{})
	assert.NoError(t, err)
}

// TestApplyShell_DotfilesURL_SkipsBrewShellenv verifies that when a custom
// dotfiles URL is set (not the default), EnsureBrewShellenv is NOT called —
// the dotfiles repo manages .zshrc itself.
func TestApplyShell_DotfilesURL_SkipsBrewShellenv(t *testing.T) {
	plan := InstallPlan{
		DryRun:         true,
		InstallOhMyZsh: false,
		DotfilesURL:    "https://github.com/user/dotfiles", // custom, not DefaultDotfilesURL
	}
	// applyShell only calls EnsureBrewShellenv when DotfilesURL == "" or DefaultDotfilesURL.
	// With a custom URL it should return nil without error.
	err := applyShell(plan, NopReporter{})
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// applyDotfiles
// ---------------------------------------------------------------------------

// TestApplyDotfiles_EmptyURL_ReturnsNil verifies that when DotfilesURL is ""
// (the "skip" case), applyDotfiles is a no-op.
func TestApplyDotfiles_EmptyURL_ReturnsNil(t *testing.T) {
	plan := InstallPlan{
		DryRun:      false,
		DotfilesURL: "",
	}
	err := applyDotfiles(plan, NopReporter{})
	assert.NoError(t, err)
}

// TestApplyDotfiles_DryRun_WithURL verifies that DryRun=true with a URL set
// returns nil — the dry-run path in dotfiles.Clone is a no-op.
func TestApplyDotfiles_DryRun_WithURL(t *testing.T) {
	plan := InstallPlan{
		DryRun:      true,
		DotfilesURL: dotfiles.DefaultDotfilesURL,
	}
	err := applyDotfiles(plan, NopReporter{})
	assert.NoError(t, err)
}

func TestApplyMacOSPrefs_NilFieldsSkipSubtasks(t *testing.T) {
	// Per spec: missing fields (nil) = skip subtasks; with all three nil/empty,
	// the function early-returns nil without any output or error.
	plan := InstallPlan{DryRun: true}
	err := applyMacOSPrefs(plan, NopReporter{})
	assert.NoError(t, err)
}

func TestApplyMacOSPrefs_DryRunRunsDockSubtask(t *testing.T) {
	plan := InstallPlan{
		DryRun:   true,
		DockApps: []string{"/Applications/Calculator.app"},
	}
	err := applyMacOSPrefs(plan, NopReporter{})
	assert.NoError(t, err)
}

func TestApplyMacOSPrefs_DryRunRunsLoginItemsSubtask(t *testing.T) {
	plan := InstallPlan{
		DryRun: true,
		LoginItems: []macos.LoginItem{
			{Name: "Calculator", Path: "/Applications/Calculator.app"},
		},
	}
	err := applyMacOSPrefs(plan, NopReporter{})
	assert.NoError(t, err)
}

func TestApplyMacOSPrefs_DryRunAllThreeSubtasks(t *testing.T) {
	// All three subtasks together in dry-run mode — verifies orchestration
	// runs each and aggregates without error.
	plan := InstallPlan{
		DryRun: true,
		MacOSPrefs: []macos.Preference{
			{Domain: "com.apple.dock", Key: "tilesize", Type: "int", Value: "48"},
		},
		DockApps: []string{"/Applications/Calculator.app"},
		LoginItems: []macos.LoginItem{
			{Name: "Calculator", Path: "/Applications/Calculator.app"},
		},
	}
	err := applyMacOSPrefs(plan, NopReporter{})
	assert.NoError(t, err)
}
