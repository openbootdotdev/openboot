package sync

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/brew"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/npm"
)

// fakeBrewRunner is a test double for the brew.Runner interface.
// forceErr causes every method to return the configured error.
type fakeBrewRunner struct {
	err error
	// output is returned verbatim for Output / CombinedOutput when err == nil.
	output []byte
}

func (f fakeBrewRunner) Output(args ...string) ([]byte, error) {
	return f.output, f.err
}

func (f fakeBrewRunner) CombinedOutput(args ...string) ([]byte, error) {
	return f.output, f.err
}

func (f fakeBrewRunner) Run(args ...string) error {
	return f.err
}

func (f fakeBrewRunner) RunInteractive(args ...string) error {
	return f.err
}

// fakeNpmRunner is a test double for the npm.Runner interface.
type fakeNpmRunner struct {
	err    error
	output []byte
}

func (f fakeNpmRunner) Output(args ...string) ([]byte, error) {
	return f.output, f.err
}

func (f fakeNpmRunner) CombinedOutput(args ...string) ([]byte, error) {
	return f.output, f.err
}

// TestExecute_EmptyPlan verifies that an empty plan returns no error and a
// zero-value SyncResult.
func TestExecute_EmptyPlan(t *testing.T) {
	plan := &SyncPlan{}
	result, err := Execute(plan, true)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Installed)
	assert.Equal(t, 0, result.Uninstalled)
	assert.Equal(t, 0, result.Updated)
	assert.Empty(t, result.Errors)
}

// TestExecute_DryRun_InstallFormulae verifies that formulae installs in dry-run
// mode succeed and increment Installed.
func TestExecute_DryRun_InstallFormulae(t *testing.T) {
	plan := &SyncPlan{
		InstallFormulae: []string{"ripgrep", "fd"},
	}
	result, err := Execute(plan, true)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Installed)
	assert.Empty(t, result.Errors)
}

// TestExecute_DryRun_InstallCasks verifies that cask installs in dry-run mode
// succeed and increment Installed.
func TestExecute_DryRun_InstallCasks(t *testing.T) {
	plan := &SyncPlan{
		InstallCasks: []string{"raycast", "firefox"},
	}
	result, err := Execute(plan, true)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Installed)
	assert.Empty(t, result.Errors)
}

// TestExecute_DryRun_InstallTaps verifies that tap installs in dry-run mode
// succeed.
func TestExecute_DryRun_InstallTaps(t *testing.T) {
	plan := &SyncPlan{
		InstallTaps: []string{"homebrew/cask-fonts"},
	}
	result, err := Execute(plan, true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Installed)
	assert.Empty(t, result.Errors)
}

// TestExecute_DryRun_InstallNpm verifies that npm installs in dry-run mode
// succeed. npm.Install with dryRun=true returns nil immediately after printing
// the plan, regardless of whether npm is in PATH.
func TestExecute_DryRun_InstallNpm(t *testing.T) {
	plan := &SyncPlan{
		InstallNpm: []string{"typescript", "eslint"},
	}
	result, err := Execute(plan, true)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Installed)
	assert.Empty(t, result.Errors)
}

// TestExecute_DryRun_UninstallFormulae verifies that formula uninstalls in
// dry-run mode succeed.
func TestExecute_DryRun_UninstallFormulae(t *testing.T) {
	plan := &SyncPlan{
		UninstallFormulae: []string{"htop"},
	}
	result, err := Execute(plan, true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Uninstalled)
	assert.Empty(t, result.Errors)
}

// TestExecute_DryRun_UninstallCasks verifies that cask uninstalls in dry-run
// mode succeed.
func TestExecute_DryRun_UninstallCasks(t *testing.T) {
	plan := &SyncPlan{
		UninstallCasks: []string{"slack"},
	}
	result, err := Execute(plan, true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Uninstalled)
	assert.Empty(t, result.Errors)
}

// TestExecute_DryRun_UninstallTaps verifies that tap removal in dry-run mode
// succeeds.
func TestExecute_DryRun_UninstallTaps(t *testing.T) {
	plan := &SyncPlan{
		UninstallTaps: []string{"old/tap"},
	}
	result, err := Execute(plan, true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Uninstalled)
	assert.Empty(t, result.Errors)
}

// TestExecute_DryRun_UninstallNpm verifies that npm uninstalls in dry-run mode
// succeed.
func TestExecute_DryRun_UninstallNpm(t *testing.T) {
	plan := &SyncPlan{
		UninstallNpm: []string{"create-react-app"},
	}
	result, err := Execute(plan, true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Uninstalled)
	assert.Empty(t, result.Errors)
}

// TestExecute_DryRun_MixedPlan verifies that a plan with installs and
// uninstalls across multiple categories is executed correctly.
func TestExecute_DryRun_MixedPlan(t *testing.T) {
	plan := &SyncPlan{
		InstallFormulae:   []string{"ripgrep", "fd"},
		InstallCasks:      []string{"raycast"},
		InstallNpm:        []string{"typescript"},
		InstallTaps:       []string{"homebrew/cask-fonts"},
		UninstallFormulae: []string{"htop"},
		UninstallCasks:    []string{"slack"},
		UninstallNpm:      []string{"create-react-app"},
		UninstallTaps:     []string{"old/tap"},
	}

	result, err := Execute(plan, true)
	require.NoError(t, err)
	// Installed: 2 formulae + 1 cask + 1 npm + 1 tap = 5
	assert.Equal(t, 5, result.Installed)
	// Uninstalled: 1 formula + 1 cask + 1 npm + 1 tap = 4
	assert.Equal(t, 4, result.Uninstalled)
	assert.Empty(t, result.Errors)
}

// TestExecute_UninstallFormulae_BrewRunnerFails verifies that when the brew
// runner returns an error for uninstall, Execute collects the error and returns
// it but keeps going.
func TestExecute_UninstallFormulae_BrewRunnerFails(t *testing.T) {
	restoreBrew := brew.SetRunner(fakeBrewRunner{err: errors.New("brew: no such formula")})
	t.Cleanup(restoreBrew)

	plan := &SyncPlan{
		UninstallFormulae: []string{"htop"},
	}
	result, err := Execute(plan, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "uninstall formulae")
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0], "uninstall formulae")
}

// TestExecute_UninstallCasks_BrewRunnerFails verifies that a cask uninstall
// failure is collected and returned.
func TestExecute_UninstallCasks_BrewRunnerFails(t *testing.T) {
	restoreBrew := brew.SetRunner(fakeBrewRunner{err: errors.New("brew: cask not found")})
	t.Cleanup(restoreBrew)

	plan := &SyncPlan{
		UninstallCasks: []string{"slack"},
	}
	result, err := Execute(plan, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "uninstall casks")
	assert.Len(t, result.Errors, 1)
}

// TestExecute_Untap_BrewRunnerFails verifies that a tap removal failure is
// collected and returned.
func TestExecute_Untap_BrewRunnerFails(t *testing.T) {
	restoreBrew := brew.SetRunner(fakeBrewRunner{err: errors.New("brew: not tapped")})
	t.Cleanup(restoreBrew)

	plan := &SyncPlan{
		UninstallTaps: []string{"old/tap"},
	}
	result, err := Execute(plan, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "untap")
	assert.Len(t, result.Errors, 1)
}

// TestExecute_UninstallNpm_RunnerFails verifies that an npm uninstall failure
// is collected and returned.
func TestExecute_UninstallNpm_RunnerFails(t *testing.T) {
	restoreNpm := npm.SetRunner(fakeNpmRunner{err: errors.New("npm: package not found")})
	t.Cleanup(restoreNpm)

	plan := &SyncPlan{
		UninstallNpm: []string{"create-react-app"},
	}
	result, err := Execute(plan, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "uninstall npm")
	assert.Len(t, result.Errors, 1)
}

// TestExecute_MultipleFailures_AllCollected verifies that Execute continues past
// the first failure and returns all errors joined together.
func TestExecute_MultipleFailures_AllCollected(t *testing.T) {
	restoreBrew := brew.SetRunner(fakeBrewRunner{err: errors.New("brew: offline")})
	t.Cleanup(restoreBrew)
	restoreNpm := npm.SetRunner(fakeNpmRunner{err: errors.New("npm: offline")})
	t.Cleanup(restoreNpm)

	plan := &SyncPlan{
		UninstallFormulae: []string{"htop"},
		UninstallNpm:      []string{"create-react-app"},
	}
	result, err := Execute(plan, false)
	require.Error(t, err)
	assert.GreaterOrEqual(t, len(result.Errors), 2, "both errors should be collected")
}

// TestExecute_DryRun_UpdateDotfiles verifies that the dotfiles update path in
// dry-run mode succeeds and increments Updated.
func TestExecute_DryRun_UpdateDotfiles(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	plan := &SyncPlan{
		UpdateDotfiles: "https://github.com/user/dotfiles",
	}
	result, err := Execute(plan, true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Updated)
	assert.Empty(t, result.Errors)
}

// TestExecute_DryRun_UpdateShell verifies that the shell update path in dry-run
// mode succeeds and increments Updated.
func TestExecute_DryRun_UpdateShell(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	plan := &SyncPlan{
		UpdateShell:  true,
		ShellOhMyZsh: true,
		ShellTheme:   "robbyrussell",
		ShellPlugins: []string{"git", "z"},
	}
	result, err := Execute(plan, true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Updated)
	assert.Empty(t, result.Errors)
}

// TestExecute_DryRun_MacOSPrefs verifies that the macOS pref update path in
// dry-run mode succeeds and increments Updated.
func TestExecute_DryRun_MacOSPrefs(t *testing.T) {
	plan := &SyncPlan{
		UpdateMacOSPrefs: []config.RemoteMacOSPref{
			{Domain: "com.apple.dock", Key: "autohide", Value: "true"},
			{Domain: "com.apple.finder", Key: "ShowPathbar", Value: "true"},
		},
	}
	result, err := Execute(plan, true)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Updated)
	assert.Empty(t, result.Errors)
}

// TestExecute_DryRun_MacOSPrefs_TypeInference verifies that an empty Type field
// is handled gracefully via type inference.
func TestExecute_DryRun_MacOSPrefs_TypeInference(t *testing.T) {
	plan := &SyncPlan{
		UpdateMacOSPrefs: []config.RemoteMacOSPref{
			// No Type provided — should be inferred from the value.
			{Domain: "com.apple.dock", Key: "autohide", Value: "true"},
		},
	}
	result, err := Execute(plan, true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Updated)
}

// TestExecute_DryRun_AllSections verifies the combined install + uninstall +
// update path across all plan sections in a single pass.
func TestExecute_DryRun_AllSections(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	plan := &SyncPlan{
		InstallFormulae:   []string{"ripgrep"},
		InstallCasks:      []string{"raycast"},
		InstallNpm:        []string{"typescript"},
		InstallTaps:       []string{"homebrew/cask-fonts"},
		UninstallFormulae: []string{"htop"},
		UninstallCasks:    []string{"slack"},
		UninstallNpm:      []string{"create-react-app"},
		UninstallTaps:     []string{"old/tap"},
		UpdateDotfiles:    "https://github.com/user/dotfiles",
		UpdateShell:       true,
		ShellTheme:        "robbyrussell",
		UpdateMacOSPrefs: []config.RemoteMacOSPref{
			{Domain: "com.apple.dock", Key: "autohide", Value: "true"},
		},
	}

	result, err := Execute(plan, true)
	require.NoError(t, err)
	// Installed: 1 formula + 1 cask + 1 npm + 1 tap = 4
	assert.Equal(t, 4, result.Installed)
	// Uninstalled: 1 formula + 1 cask + 1 npm + 1 tap = 4
	assert.Equal(t, 4, result.Uninstalled)
	// Updated: 1 dotfiles + 1 shell + 1 macos pref = 3
	assert.Equal(t, 3, result.Updated)
	assert.Empty(t, result.Errors)
}

// TestExecute_DryRun_ResultCountsAreAccurate is a table-driven test that checks
// Installed and Uninstalled counts across several plan shapes.
func TestExecute_DryRun_ResultCountsAreAccurate(t *testing.T) {
	tests := []struct {
		name            string
		plan            *SyncPlan
		wantInstalled   int
		wantUninstalled int
	}{
		{
			name:            "empty plan",
			plan:            &SyncPlan{},
			wantInstalled:   0,
			wantUninstalled: 0,
		},
		{
			name:            "formulae only",
			plan:            &SyncPlan{InstallFormulae: []string{"a", "b"}},
			wantInstalled:   2,
			wantUninstalled: 0,
		},
		{
			name:            "casks only",
			plan:            &SyncPlan{InstallCasks: []string{"app1"}},
			wantInstalled:   1,
			wantUninstalled: 0,
		},
		{
			name:            "taps only",
			plan:            &SyncPlan{InstallTaps: []string{"t1", "t2", "t3"}},
			wantInstalled:   3,
			wantUninstalled: 0,
		},
		{
			name:            "uninstall only",
			plan:            &SyncPlan{UninstallFormulae: []string{"x"}, UninstallCasks: []string{"y"}},
			wantInstalled:   0,
			wantUninstalled: 2,
		},
		{
			name: "install and uninstall",
			plan: &SyncPlan{
				InstallFormulae:   []string{"new1"},
				UninstallFormulae: []string{"old1", "old2"},
			},
			wantInstalled:   1,
			wantUninstalled: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Execute(tt.plan, true)
			require.NoError(t, err)
			assert.Equal(t, tt.wantInstalled, result.Installed, "Installed count")
			assert.Equal(t, tt.wantUninstalled, result.Uninstalled, "Uninstalled count")
		})
	}
}
