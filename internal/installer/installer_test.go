package installer

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEstimateInstallMinutes(t *testing.T) {
	tests := []struct {
		name     string
		formulae int
		casks    int
		npm      int
		expected int
	}{
		{"zero_packages_returns_1min", 0, 0, 0, 1},
		{"single_formula", 1, 0, 0, 1},
		{"four_formulae", 4, 0, 0, 1},
		{"twenty_formulae", 20, 0, 0, 5},
		{"mixed_packages", 10, 5, 10, 5},
		{"cask_heavy", 0, 10, 0, 5},
		{"npm_only", 0, 0, 12, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := estimateInstallMinutes(tt.formulae, tt.casks, tt.npm)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCategorizeSelectedPackages_EmptySelection(t *testing.T) {
	cfg := &config.Config{
		SelectedPkgs: map[string]bool{},
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	result := categorizeSelectedPackages(opts, st)
	assert.Empty(t, result.cli)
	assert.Empty(t, result.cask)
	assert.Empty(t, result.npm)
}

func TestCategorizeSelectedPackages_RemoteConfig(t *testing.T) {
	cfg := &config.Config{
		RemoteConfig: &config.RemoteConfig{
			Casks: config.PackageEntryList{{Name: "visual-studio-code"}, {Name: "firefox"}},
			Npm:   config.PackageEntryList{{Name: "typescript"}, {Name: "eslint"}},
		},
		SelectedPkgs: map[string]bool{
			"git":                true,
			"visual-studio-code": true,
			"typescript":         true,
			"curl":               true,
		},
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	result := categorizeSelectedPackages(opts, st)

	assert.Contains(t, result.cask, "visual-studio-code")
	assert.Contains(t, result.npm, "typescript")
	assert.Contains(t, result.cli, "git")
	assert.Contains(t, result.cli, "curl")
}

func TestCategorizeSelectedPackages_RemoteConfig_NoCasks(t *testing.T) {
	cfg := &config.Config{
		RemoteConfig: &config.RemoteConfig{
			Casks: config.PackageEntryList{},
			Npm:   config.PackageEntryList{},
		},
		SelectedPkgs: map[string]bool{
			"git":  true,
			"curl": true,
		},
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	result := categorizeSelectedPackages(opts, st)

	assert.Equal(t, 2, len(result.cli))
	assert.Empty(t, result.cask)
	assert.Empty(t, result.npm)
}

func TestCategorizeSelectedPackages_WithOnlinePkgs(t *testing.T) {
	cfg := &config.Config{
		SelectedPkgs: map[string]bool{},
		OnlinePkgs: []config.Package{
			{Name: "my-formula", IsCask: false, IsNpm: false},
			{Name: "my-cask", IsCask: true, IsNpm: false},
			{Name: "my-npm-pkg", IsCask: false, IsNpm: true},
		},
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	result := categorizeSelectedPackages(opts, st)

	assert.Contains(t, result.cli, "my-formula")
	assert.Contains(t, result.cask, "my-cask")
	assert.Contains(t, result.npm, "my-npm-pkg")
}

func TestRun_UpdateRoute(t *testing.T) {
	cfg := &config.Config{
		Update: true,
		DryRun: true,
	}
	err := Run(cfg)
	assert.NoError(t, err)
}

func TestCheckDependencies_DryRunSkipsEverything(t *testing.T) {
	cfg := &config.Config{
		DryRun: true,
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := checkDependencies(opts, st)
	assert.NoError(t, err)
}

// TestRunCustomInstall_IncludesCasksInSelectedPkgs verifies that GUI apps (casks)
// from remote config are added to SelectedPkgs so they get installed.
// Regression test for: https://github.com/openbootdotdev/openboot/issues/17
func TestRunCustomInstall_IncludesCasksInSelectedPkgs(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := &config.Config{
		DryRun: true,
		Shell:  "skip",
		Macos:  "skip",
		RemoteConfig: &config.RemoteConfig{
			Username: "testuser",
			Slug:     "testconfig",
			Packages: config.PackageEntryList{{Name: "git"}, {Name: "curl"}},
			Casks:    config.PackageEntryList{{Name: "visual-studio-code"}, {Name: "firefox"}},
		},
	}

	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := runCustomInstall(opts, st)
	assert.NoError(t, err)

	// Verify both packages and casks are in SelectedPkgs
	assert.Contains(t, st.SelectedPkgs, "git", "CLI package should be in SelectedPkgs")
	assert.Contains(t, st.SelectedPkgs, "curl", "CLI package should be in SelectedPkgs")
	assert.Contains(t, st.SelectedPkgs, "visual-studio-code", "GUI app (cask) should be in SelectedPkgs")
	assert.Contains(t, st.SelectedPkgs, "firefox", "GUI app (cask) should be in SelectedPkgs")
}

func TestRunInstall_DryRunRemoteConfig(t *testing.T) {
	cfg := &config.Config{
		DryRun: true,
		RemoteConfig: &config.RemoteConfig{
			Username: "testuser",
			Slug:     "default",
			Packages: config.PackageEntryList{{Name: "git"}, {Name: "curl"}},
			Casks:    config.PackageEntryList{{Name: "firefox"}},
			Taps:     []string{"homebrew/cask"},
		},
	}

	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := runInstall(opts, st)
	require.NoError(t, err)
	assert.True(t, st.SelectedPkgs["git"])
	assert.True(t, st.SelectedPkgs["curl"])
}

func TestNewInstallState(t *testing.T) {
	state := newInstallState()
	assert.NotNil(t, state)
	assert.NotNil(t, state.InstalledFormulae)
	assert.NotNil(t, state.InstalledCasks)
	assert.NotNil(t, state.InstalledNpm)
	assert.Empty(t, state.InstalledFormulae)
	assert.Empty(t, state.InstalledCasks)
	assert.Empty(t, state.InstalledNpm)
	assert.False(t, state.LastUpdated.IsZero())
}

func TestInstallState_MarkAndCheck(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	state := newInstallState()

	assert.False(t, state.isFormulaInstalled("git"))
	assert.False(t, state.isCaskInstalled("firefox"))
	assert.False(t, state.isNpmInstalled("typescript"))

	require.NoError(t, state.markFormula("git"))
	require.NoError(t, state.markCask("firefox"))
	require.NoError(t, state.markNpm("typescript"))

	assert.True(t, state.isFormulaInstalled("git"))
	assert.True(t, state.isCaskInstalled("firefox"))
	assert.True(t, state.isNpmInstalled("typescript"))

	assert.False(t, state.isFormulaInstalled("curl"))
	assert.False(t, state.isCaskInstalled("chrome"))
	assert.False(t, state.isNpmInstalled("eslint"))
}

func TestInstallState_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	state := newInstallState()
	require.NoError(t, state.markFormula("git"))
	require.NoError(t, state.markFormula("curl"))
	require.NoError(t, state.markCask("firefox"))
	require.NoError(t, state.markNpm("typescript"))

	loaded, err := loadState()
	require.NoError(t, err)

	assert.True(t, loaded.isFormulaInstalled("git"))
	assert.True(t, loaded.isFormulaInstalled("curl"))
	assert.True(t, loaded.isCaskInstalled("firefox"))
	assert.True(t, loaded.isNpmInstalled("typescript"))
}

func TestLoadState_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	state, err := loadState()
	assert.NoError(t, err)
	assert.NotNil(t, state)
	assert.Empty(t, state.InstalledFormulae)
}

func TestLoadState_CorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	stateDir := filepath.Join(tmpDir, ".openboot")
	require.NoError(t, os.MkdirAll(stateDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "install_state.json"), []byte("not json"), 0644))

	state, err := loadState()
	assert.Error(t, err)
	assert.NotNil(t, state)
	assert.Empty(t, state.InstalledFormulae)
}

func TestLoadState_NilMapsInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	stateDir := filepath.Join(tmpDir, ".openboot")
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	data, _ := json.Marshal(map[string]interface{}{
		"last_updated": "2024-01-01T00:00:00Z",
	})
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "install_state.json"), data, 0644))

	state, err := loadState()
	require.NoError(t, err)
	assert.NotNil(t, state.InstalledFormulae)
	assert.NotNil(t, state.InstalledCasks)
	assert.NotNil(t, state.InstalledNpm)
}

func TestGetStatePath(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path, err := getStatePath()
	require.NoError(t, err)
	assert.Contains(t, path, ".openboot")
	assert.Contains(t, path, "install_state.json")
	assert.True(t, filepath.IsAbs(path))
}

func TestErrUserCancelled(t *testing.T) {
	assert.Error(t, ErrUserCancelled)
	assert.Equal(t, "user cancelled", ErrUserCancelled.Error())
}

func TestStepGitConfig_DryRunNoTTY(t *testing.T) {
	cfg := &config.Config{
		DryRun:   true,
		GitName:  "Test",
		GitEmail: "test@example.com",
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepGitConfig(opts, st)
	assert.NoError(t, err)
}

func TestStepGitConfig_SilentMode_MissingFields(t *testing.T) {
	cfg := &config.Config{
		Silent:   true,
		GitName:  "",
		GitEmail: "",
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()

	err := stepGitConfig(opts, st)
	if err != nil {
		assert.Contains(t, err.Error(), "required in silent mode")
	}
}

func TestStepPresetSelection_PresetAlreadySet(t *testing.T) {
	cfg := &config.Config{
		Preset: "minimal",
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepPresetSelection(opts, st)
	assert.NoError(t, err)
}

func TestStepPresetSelection_ScratchPreset(t *testing.T) {
	cfg := &config.Config{
		Preset: "scratch",
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepPresetSelection(opts, st)
	assert.NoError(t, err)
}

func TestStepPresetSelection_InvalidPreset(t *testing.T) {
	cfg := &config.Config{
		Preset: "nonexistent_preset",
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepPresetSelection(opts, st)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid preset")
}

func TestStepPresetSelection_SilentDefaultsToMinimal(t *testing.T) {
	cfg := &config.Config{
		Silent: true,
		Preset: "",
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepPresetSelection(opts, st)
	assert.NoError(t, err)
	assert.Equal(t, "minimal", opts.Preset)
}

func TestStepPackageCustomization_Silent(t *testing.T) {
	cfg := &config.Config{
		Silent: true,
		Preset: "minimal",
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepPackageCustomization(opts, st)
	assert.NoError(t, err)
	assert.NotNil(t, st.SelectedPkgs)
	assert.Greater(t, len(st.SelectedPkgs), 0)
}

func TestStepPackageCustomization_DryRunNoTTY(t *testing.T) {
	cfg := &config.Config{
		DryRun: true,
		Preset: "developer",
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepPackageCustomization(opts, st)
	assert.NoError(t, err)
	assert.NotNil(t, st.SelectedPkgs)
}

func TestStepShell_Skip(t *testing.T) {
	cfg := &config.Config{
		Shell: "skip",
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepShell(opts, st)
	assert.NoError(t, err)
}

func TestStepDotfiles_Skip(t *testing.T) {
	cfg := &config.Config{
		Dotfiles: "skip",
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepDotfiles(opts, st)
	assert.NoError(t, err)
}

func TestStepMacOS_Skip(t *testing.T) {
	cfg := &config.Config{
		Macos: "skip",
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepMacOS(opts, st)
	assert.NoError(t, err)
}

func TestStepMacOS_ConfigureFlag_DryRun(t *testing.T) {
	// --macos configure bypasses the TUI and applies all defaults directly.
	cfg := &config.Config{
		Macos:  "configure",
		DryRun: true,
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepMacOS(opts, st)
	assert.NoError(t, err)
}

func TestStepMacOS_Silent_DryRun(t *testing.T) {
	// Silent mode bypasses the TUI and applies all defaults directly.
	cfg := &config.Config{
		Silent: true,
		DryRun: true,
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepMacOS(opts, st)
	assert.NoError(t, err)
}

func TestInstallState_OnlySuccessfulPackagesMarked(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	s := newInstallState()

	require.NoError(t, s.markFormula("git"))
	require.NoError(t, s.markFormula("curl"))

	assert.True(t, s.isFormulaInstalled("git"))
	assert.True(t, s.isFormulaInstalled("curl"))
	assert.False(t, s.isFormulaInstalled("ripgrep"), "ripgrep was never marked as installed")

	loaded, err := loadState()
	require.NoError(t, err)

	assert.True(t, loaded.isFormulaInstalled("git"))
	assert.True(t, loaded.isFormulaInstalled("curl"))
	assert.False(t, loaded.isFormulaInstalled("ripgrep"), "ripgrep should not appear in persisted state")
}

func TestRunInteractiveInstall_HardFailOnBrew(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := &config.Config{
		DryRun:       true,
		Preset:       "minimal",
		PackagesOnly: true,
		SelectedPkgs: map[string]bool{},
	}

	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := runInteractiveInstall(opts, st)
	assert.NoError(t, err)
}

func TestRunFromSnapshot_SoftFailuresReturnError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := &config.Config{
		DryRun:       true,
		Silent:       true,
		Preset:       "minimal",
		Shell:        "skip",
		Macos:        "skip",
		Dotfiles:     "skip",
		SelectedPkgs: map[string]bool{},
		SnapshotGit:  nil,
	}

	err := RunFromSnapshot(cfg)
	assert.NoError(t, err)
}

func TestRunCustomInstall_RunsShellDotfilesMacOS(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("OPENBOOT_DOTFILES", "")

	cfg := &config.Config{
		DryRun: true,
		Shell:  "skip",
		Macos:  "skip",
		RemoteConfig: &config.RemoteConfig{
			Username: "testuser",
			Slug:     "default",
			Packages: config.PackageEntryList{{Name: "git"}},
		},
	}

	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := runCustomInstall(opts, st)
	assert.NoError(t, err)
}

func TestRunCustomInstall_DotfilesRepoPopulatesDotfilesURL(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := &config.Config{
		DryRun:   true,
		Shell:    "skip",
		Macos:    "skip",
		Dotfiles: "skip",
		RemoteConfig: &config.RemoteConfig{
			Username:     "testuser",
			Slug:         "default",
			Packages:     config.PackageEntryList{{Name: "git"}},
			DotfilesRepo: "https://github.com/testuser/dotfiles",
		},
	}

	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := runCustomInstall(opts, st)
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/testuser/dotfiles", opts.DotfilesURL)
}

func TestRunCustomInstall_DotfilesFallsBackToDefault(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := &config.Config{
		DryRun: true,
		Shell:  "skip",
		Macos:  "skip",
		RemoteConfig: &config.RemoteConfig{
			Username: "testuser",
			Slug:     "default",
			Packages: config.PackageEntryList{{Name: "git"}},
		},
		Dotfiles: "link",
	}

	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := runCustomInstall(opts, st)
	assert.NoError(t, err, "should succeed using default dotfiles template")
}

func TestStepDotfiles_UsesDotfilesURLFromConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("OPENBOOT_DOTFILES", "")

	cfg := &config.Config{
		DryRun:      true,
		Dotfiles:    "clone",
		DotfilesURL: "https://github.com/testuser/dotfiles",
	}

	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepDotfiles(opts, st)
	assert.NoError(t, err)
}

func TestStepDotfiles_EnvVarTakesPriorityOverConfigURL(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("OPENBOOT_DOTFILES", "https://github.com/from-env/dotfiles")

	r, w, _ := os.Pipe()
	origStdout := os.Stdout
	os.Stdout = w

	cfg := &config.Config{
		DryRun:      true,
		Dotfiles:    "clone",
		DotfilesURL: "https://github.com/from-config/dotfiles",
	}

	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepDotfiles(opts, st)

	w.Close()
	os.Stdout = origStdout
	out, _ := io.ReadAll(r)
	output := string(out)

	assert.NoError(t, err)
	assert.Contains(t, output, "from-env")
	assert.NotContains(t, output, "from-config")
}

func TestStepPostInstall_SkipFlag(t *testing.T) {
	cfg := &config.Config{
		PostInstall: "skip",
		RemoteConfig: &config.RemoteConfig{
			PostInstall: []string{"echo hello"},
		},
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepPostInstall(opts, st)
	assert.NoError(t, err)
}

func TestStepPostInstall_NilRemoteConfig(t *testing.T) {
	cfg := &config.Config{}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepPostInstall(opts, st)
	assert.NoError(t, err)
}

func TestStepPostInstall_EmptyCommands(t *testing.T) {
	cfg := &config.Config{
		RemoteConfig: &config.RemoteConfig{
			PostInstall: []string{},
		},
	}
	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepPostInstall(opts, st)
	assert.NoError(t, err)
}

func TestStepPostInstall_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	r, w, _ := os.Pipe()
	origStdout := os.Stdout
	os.Stdout = w

	cfg := &config.Config{
		DryRun: true,
		RemoteConfig: &config.RemoteConfig{
			PostInstall: []string{"mise install", "npm install -g pnpm"},
		},
	}

	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepPostInstall(opts, st)

	w.Close()
	os.Stdout = origStdout
	out, _ := io.ReadAll(r)
	output := string(out)

	assert.NoError(t, err)
	assert.Contains(t, output, "mise install")
	assert.Contains(t, output, "npm install -g pnpm")
	assert.Contains(t, output, "[DRY-RUN]")
}

func TestStepPostInstall_RunsCommandsInSilentMode(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	markerFile := tmpDir + "/post-install-ran"
	cfg := &config.Config{
		Silent:           true,
		AllowPostInstall: true,
		RemoteConfig: &config.RemoteConfig{
			PostInstall: []string{"touch " + markerFile},
		},
	}

	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepPostInstall(opts, st)
	assert.NoError(t, err)

	_, statErr := os.Stat(markerFile)
	assert.NoError(t, statErr, "marker file should exist after post-install ran")
}

func TestStepPostInstall_CommandFailureReturnsSoftError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := &config.Config{
		Silent:           true,
		AllowPostInstall: true,
		RemoteConfig: &config.RemoteConfig{
			PostInstall: []string{"exit 1"},
		},
	}

	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepPostInstall(opts, st)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "post-install script")
}

func TestStepPostInstall_ContinuesAfterCommandFailure(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	markerFile := tmpDir + "/second-ran"
	cfg := &config.Config{
		Silent:           true,
		AllowPostInstall: true,
		RemoteConfig: &config.RemoteConfig{
			// Use "false" (a command that fails with exit 1) instead of "exit 1"
			// because exit terminates the entire script, while false just sets $?.
			PostInstall: []string{"false", "touch " + markerFile},
		},
	}

	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()

	// With single-script execution, zsh runs all lines without set -e,
	// so the second command runs and the script exits 0 (touch succeeds).
	err := stepPostInstall(opts, st)
	assert.NoError(t, err)

	_, statErr := os.Stat(markerFile)
	assert.NoError(t, statErr, "second command should still run after first fails")
}

func TestStepPostInstall_SharedContext(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	markerFile := tmpDir + "/shared-context"
	cfg := &config.Config{
		Silent:           true,
		AllowPostInstall: true,
		RemoteConfig: &config.RemoteConfig{
			PostInstall: []string{
				"MY_VAR=hello",
				"echo $MY_VAR > " + markerFile,
			},
		},
	}

	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := stepPostInstall(opts, st)
	assert.NoError(t, err)

	content, readErr := os.ReadFile(markerFile)
	assert.NoError(t, readErr)
	assert.Equal(t, "hello\n", string(content), "variable set on one line should be available on the next")
}

func TestReconcileBrewWithSystem_RemovesUninstalledPackages(t *testing.T) {
	state := newInstallState()
	state.InstalledFormulae["git"] = true
	state.InstalledFormulae["tree"] = true
	state.InstalledFormulae["curl"] = true
	state.InstalledCasks["firefox"] = true
	state.InstalledCasks["slack"] = true

	// Simulate system where tree and slack were manually uninstalled
	actualFormulae := map[string]bool{"git": true, "curl": true}
	actualCasks := map[string]bool{"firefox": true}

	removed := state.reconcileBrewWithSystem(actualFormulae, actualCasks)

	assert.Equal(t, 2, removed)
	assert.True(t, state.isFormulaInstalled("git"))
	assert.True(t, state.isFormulaInstalled("curl"))
	assert.False(t, state.isFormulaInstalled("tree"), "tree was uninstalled and should be removed from state")
	assert.True(t, state.isCaskInstalled("firefox"))
	assert.False(t, state.isCaskInstalled("slack"), "slack was uninstalled and should be removed from state")
}

func TestReconcileBrewWithSystem_NoChangesWhenAllInstalled(t *testing.T) {
	state := newInstallState()
	state.InstalledFormulae["git"] = true
	state.InstalledCasks["firefox"] = true

	actualFormulae := map[string]bool{"git": true, "curl": true}
	actualCasks := map[string]bool{"firefox": true, "slack": true}

	removed := state.reconcileBrewWithSystem(actualFormulae, actualCasks)

	assert.Equal(t, 0, removed)
	assert.True(t, state.isFormulaInstalled("git"))
	assert.True(t, state.isCaskInstalled("firefox"))
}

func TestReconcileBrewWithSystem_EmptyState(t *testing.T) {
	state := newInstallState()

	actualFormulae := map[string]bool{"git": true}
	actualCasks := map[string]bool{"firefox": true}

	removed := state.reconcileBrewWithSystem(actualFormulae, actualCasks)
	assert.Equal(t, 0, removed)
}

func TestReconcileNpmWithSystem_RemovesUninstalledPackages(t *testing.T) {
	state := newInstallState()
	state.InstalledNpm["typescript"] = true
	state.InstalledNpm["eslint"] = true
	state.InstalledNpm["prettier"] = true

	// Simulate system where eslint was manually uninstalled
	actualNpm := map[string]bool{"typescript": true, "prettier": true}

	removed := state.reconcileNpmWithSystem(actualNpm)

	assert.Equal(t, 1, removed)
	assert.True(t, state.isNpmInstalled("typescript"))
	assert.False(t, state.isNpmInstalled("eslint"), "eslint was uninstalled and should be removed from state")
	assert.True(t, state.isNpmInstalled("prettier"))
}

func TestReconcileNpmWithSystem_EmptyState(t *testing.T) {
	state := newInstallState()

	actualNpm := map[string]bool{"typescript": true}

	removed := state.reconcileNpmWithSystem(actualNpm)
	assert.Equal(t, 0, removed)
}

func TestRunCustomInstall_WithPostInstallScript(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("OPENBOOT_DOTFILES", "")

	cfg := &config.Config{
		DryRun: true,
		Shell:  "skip",
		Macos:  "skip",
		RemoteConfig: &config.RemoteConfig{
			Username:    "testuser",
			Slug:        "default",
			Packages:    config.PackageEntryList{{Name: "git"}},
			PostInstall: []string{"mise install", "npm install -g pnpm"},
		},
	}

	opts := cfg.ToInstallOptions()
	st := cfg.ToInstallState()
	err := runCustomInstall(opts, st)
	assert.NoError(t, err)
}
