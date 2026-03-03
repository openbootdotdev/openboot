package cli

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"github.com/openbootdotdev/openboot/internal/config"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncCmdRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "sync" {
			found = true
			break
		}
	}
	assert.True(t, found, "sync command should be registered on rootCmd")
}

func TestSyncCmdFlags(t *testing.T) {
	f := syncCmd.Flags()

	sourceFlag := f.Lookup("source")
	assert.NotNil(t, sourceFlag)
	assert.Equal(t, "string", sourceFlag.Value.Type())

	dryRunFlag := f.Lookup("dry-run")
	assert.NotNil(t, dryRunFlag)
	assert.Equal(t, "bool", dryRunFlag.Value.Type())

	installOnlyFlag := f.Lookup("install-only")
	assert.NotNil(t, installOnlyFlag)
	assert.Equal(t, "bool", installOnlyFlag.Value.Type())
}

func TestBuildMissingOptions(t *testing.T) {
	d := &syncpkg.SyncDiff{
		MissingFormulae: []string{"ripgrep", "fd"},
		MissingCasks:    []string{"raycast"},
		MissingNpm:      []string{"turbo"},
		MissingTaps:     []string{"homebrew/cask-fonts"},
	}

	opts := buildMissingOptions(d)
	assert.Len(t, opts, 5)
	assert.Equal(t, "ripgrep", opts[0].Label)
	assert.Equal(t, categoryFormulae, opts[0].Category)
	assert.Equal(t, "raycast (cask)", opts[2].Label)
	assert.Equal(t, categoryCasks, opts[2].Category)
	assert.Equal(t, "turbo (npm)", opts[3].Label)
	assert.Equal(t, categoryNpm, opts[3].Category)
	assert.Equal(t, "homebrew/cask-fonts (tap)", opts[4].Label)
	assert.Equal(t, categoryTaps, opts[4].Category)
}

func TestBuildMissingOptionsEmpty(t *testing.T) {
	d := &syncpkg.SyncDiff{}
	opts := buildMissingOptions(d)
	assert.Empty(t, opts)
}

func TestBuildExtraOptions(t *testing.T) {
	d := &syncpkg.SyncDiff{
		ExtraFormulae: []string{"htop"},
		ExtraCasks:    []string{"slack"},
	}

	opts := buildExtraOptions(d)
	assert.Len(t, opts, 2)
	assert.Equal(t, "htop", opts[0].Label)
	assert.Equal(t, categoryFormulae, opts[0].Category)
	assert.Equal(t, "slack (cask)", opts[1].Label)
	assert.Equal(t, categoryCasks, opts[1].Category)
}

func TestBuildExtraOptionsAllCategories(t *testing.T) {
	d := &syncpkg.SyncDiff{
		ExtraFormulae: []string{"htop"},
		ExtraCasks:    []string{"slack"},
		ExtraNpm:      []string{"create-react-app"},
		ExtraTaps:     []string{"old/tap"},
	}

	opts := buildExtraOptions(d)
	assert.Len(t, opts, 4)
	assert.Equal(t, "htop", opts[0].Label)
	assert.Equal(t, categoryFormulae, opts[0].Category)
	assert.Equal(t, "slack (cask)", opts[1].Label)
	assert.Equal(t, categoryCasks, opts[1].Category)
	assert.Equal(t, "create-react-app (npm)", opts[2].Label)
	assert.Equal(t, categoryNpm, opts[2].Category)
	assert.Equal(t, "old/tap (tap)", opts[3].Label)
	assert.Equal(t, categoryTaps, opts[3].Category)
}

func TestBuildExtraOptionsEmpty(t *testing.T) {
	d := &syncpkg.SyncDiff{}
	opts := buildExtraOptions(d)
	assert.Empty(t, opts)
}

func TestCategorizeMissing(t *testing.T) {
	d := &syncpkg.SyncDiff{
		MissingFormulae: []string{"ripgrep"},
		MissingCasks:    []string{"raycast"},
		MissingNpm:      []string{"turbo"},
		MissingTaps:     []string{"homebrew/cask-fonts"},
	}

	selected := []pkgOption{
		{Label: "ripgrep", Category: categoryFormulae},
		{Label: "raycast (cask)", Category: categoryCasks},
		{Label: "turbo (npm)", Category: categoryNpm},
	}

	result := categorizeMissing(selected, d)
	assert.Equal(t, []string{"ripgrep"}, result.Formulae)
	assert.Equal(t, []string{"raycast"}, result.Casks)
	assert.Equal(t, []string{"turbo"}, result.Npm)
	assert.Empty(t, result.Taps)
}

func TestCategorizeMissingAllCategories(t *testing.T) {
	d := &syncpkg.SyncDiff{
		MissingFormulae: []string{"ripgrep"},
		MissingCasks:    []string{"raycast"},
		MissingNpm:      []string{"turbo"},
		MissingTaps:     []string{"homebrew/cask-fonts"},
	}

	selected := []pkgOption{
		{Label: "ripgrep", Category: categoryFormulae},
		{Label: "raycast (cask)", Category: categoryCasks},
		{Label: "turbo (npm)", Category: categoryNpm},
		{Label: "homebrew/cask-fonts (tap)", Category: categoryTaps},
	}

	result := categorizeMissing(selected, d)
	assert.Equal(t, []string{"ripgrep"}, result.Formulae)
	assert.Equal(t, []string{"raycast"}, result.Casks)
	assert.Equal(t, []string{"turbo"}, result.Npm)
	assert.Equal(t, []string{"homebrew/cask-fonts"}, result.Taps)
}

func TestCategorizeMissingNoneSelected(t *testing.T) {
	d := &syncpkg.SyncDiff{
		MissingFormulae: []string{"ripgrep"},
	}

	result := categorizeMissing([]pkgOption{}, d)
	assert.Empty(t, result.Formulae)
}

func TestCategorizeMissingNotInDiff(t *testing.T) {
	d := &syncpkg.SyncDiff{
		MissingFormulae: []string{"ripgrep"},
	}

	// Select a formula that's not actually in the diff
	selected := []pkgOption{
		{Label: "unknown-pkg", Category: categoryFormulae},
	}

	result := categorizeMissing(selected, d)
	assert.Empty(t, result.Formulae)
}

func TestCategorizeExtra(t *testing.T) {
	d := &syncpkg.SyncDiff{
		ExtraFormulae: []string{"htop"},
		ExtraCasks:    []string{"slack"},
	}

	selected := []pkgOption{
		{Label: "htop", Category: categoryFormulae},
	}

	result := categorizeExtra(selected, d)
	assert.Equal(t, []string{"htop"}, result.Formulae)
	assert.Empty(t, result.Casks)
}

func TestCategorizeExtraAllCategories(t *testing.T) {
	d := &syncpkg.SyncDiff{
		ExtraFormulae: []string{"htop"},
		ExtraCasks:    []string{"slack"},
		ExtraNpm:      []string{"create-react-app"},
		ExtraTaps:     []string{"old/tap"},
	}

	selected := []pkgOption{
		{Label: "htop", Category: categoryFormulae},
		{Label: "slack (cask)", Category: categoryCasks},
		{Label: "create-react-app (npm)", Category: categoryNpm},
		{Label: "old/tap (tap)", Category: categoryTaps},
	}

	result := categorizeExtra(selected, d)
	assert.Equal(t, []string{"htop"}, result.Formulae)
	assert.Equal(t, []string{"slack"}, result.Casks)
	assert.Equal(t, []string{"create-react-app"}, result.Npm)
	assert.Equal(t, []string{"old/tap"}, result.Taps)
}

func TestCategorizeExtraNoneSelected(t *testing.T) {
	d := &syncpkg.SyncDiff{
		ExtraFormulae: []string{"htop"},
	}

	result := categorizeExtra([]pkgOption{}, d)
	assert.Empty(t, result.Formulae)
}

func TestCategorizeExtraNotInDiff(t *testing.T) {
	d := &syncpkg.SyncDiff{
		ExtraFormulae: []string{"htop"},
	}

	selected := []pkgOption{
		{Label: "unknown-pkg", Category: categoryFormulae},
	}

	result := categorizeExtra(selected, d)
	assert.Empty(t, result.Formulae)
}

func TestUpdateSyncedAt(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	now := time.Now().Truncate(time.Second)
	source := &syncpkg.SyncSource{
		UserSlug:    "alice/setup",
		Username:    "alice",
		Slug:        "setup",
		InstalledAt: now,
	}
	rc := &config.RemoteConfig{
		Username: "alice",
		Slug:     "setup",
	}

	updateSyncedAt(source, "", rc)

	loaded, err := syncpkg.LoadSource()
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "alice/setup", loaded.UserSlug)
	assert.Equal(t, "alice", loaded.Username)
	assert.Equal(t, "setup", loaded.Slug)
	assert.False(t, loaded.SyncedAt.IsZero())
	assert.True(t, loaded.InstalledAt.Equal(now))
}

func TestUpdateSyncedAtWithOverride(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	source := &syncpkg.SyncSource{
		UserSlug:    "old/config",
		Username:    "old",
		Slug:        "config",
		InstalledAt: time.Now().Truncate(time.Second),
	}
	rc := &config.RemoteConfig{
		Username: "new",
		Slug:     "setup",
	}

	updateSyncedAt(source, "new/setup", rc)

	loaded, err := syncpkg.LoadSource()
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "new/setup", loaded.UserSlug)
	assert.Equal(t, "new", loaded.Username)
	assert.Equal(t, "setup", loaded.Slug)
}

func TestUpdateSyncedAtZeroInstalledAt(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	source := &syncpkg.SyncSource{
		UserSlug: "user/config",
		Username: "user",
		Slug:     "config",
		// InstalledAt is zero
	}
	rc := &config.RemoteConfig{
		Username: "user",
		Slug:     "config",
	}

	updateSyncedAt(source, "", rc)

	loaded, err := syncpkg.LoadSource()
	require.NoError(t, err)
	require.NotNil(t, loaded)
	// When InstalledAt was zero, it should be set to now
	assert.False(t, loaded.InstalledAt.IsZero())
}

// captureStdout captures stdout during fn execution.
func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestPrintSyncDiffPackages(t *testing.T) {
	d := &syncpkg.SyncDiff{
		MissingFormulae: []string{"ripgrep", "fd"},
		MissingCasks:    []string{"raycast"},
		ExtraFormulae:   []string{"htop"},
	}

	output := captureStdout(func() {
		printSyncDiff(d)
	})

	assert.Contains(t, output, "Formulae to install (2)")
	assert.Contains(t, output, "ripgrep, fd")
	assert.Contains(t, output, "Casks to install (1)")
	assert.Contains(t, output, "raycast")
	assert.Contains(t, output, "Formulae extra (1)")
	assert.Contains(t, output, "htop")
}

func TestPrintSyncDiffShell(t *testing.T) {
	d := &syncpkg.SyncDiff{
		ShellChanged: true,
		ShellDiff: &syncpkg.ShellDiff{
			ThemeChanged:   true,
			RemoteTheme:    "agnoster",
			LocalTheme:     "robbyrussell",
			MissingPlugins: []string{"zsh-autosuggestions"},
			ExtraPlugins:   []string{"old-plugin"},
		},
	}

	output := captureStdout(func() {
		printSyncDiff(d)
	})

	assert.Contains(t, output, "Theme:")
	assert.Contains(t, output, "robbyrussell")
	assert.Contains(t, output, "agnoster")
	assert.Contains(t, output, "New plugins: zsh-autosuggestions")
	assert.Contains(t, output, "Extra plugins: old-plugin")
}

func TestPrintSyncDiffMacOS(t *testing.T) {
	d := &syncpkg.SyncDiff{
		MacOSChanged: []syncpkg.MacOSPrefDiff{
			{Domain: "com.apple.dock", Key: "autohide", Desc: "Dock auto-hide", RemoteValue: "true", LocalValue: "false"},
		},
	}

	output := captureStdout(func() {
		printSyncDiff(d)
	})

	assert.Contains(t, output, "Dock auto-hide")
	assert.Contains(t, output, "false")
	assert.Contains(t, output, "true")
}

func TestPrintSyncDiffMacOSNoDesc(t *testing.T) {
	d := &syncpkg.SyncDiff{
		MacOSChanged: []syncpkg.MacOSPrefDiff{
			{Domain: "com.apple.dock", Key: "autohide", Desc: "", RemoteValue: "true", LocalValue: "false"},
		},
	}

	output := captureStdout(func() {
		printSyncDiff(d)
	})

	// When desc is empty, should show domain.key
	assert.Contains(t, output, "com.apple.dock.autohide")
}

func TestPrintSyncDiffDotfiles(t *testing.T) {
	d := &syncpkg.SyncDiff{
		DotfilesChanged: true,
		RemoteDotfiles:  "https://github.com/new/dots",
		LocalDotfiles:   "https://github.com/old/dots",
	}

	output := captureStdout(func() {
		printSyncDiff(d)
	})

	assert.Contains(t, output, "Dotfiles")
	assert.Contains(t, output, "Repo changed")
	assert.Contains(t, output, "https://github.com/old/dots")
	assert.Contains(t, output, "https://github.com/new/dots")
}

func TestPrintSyncDiffEmpty(t *testing.T) {
	d := &syncpkg.SyncDiff{}

	output := captureStdout(func() {
		printSyncDiff(d)
	})

	assert.Empty(t, output)
}

func TestPrintMissingExtra(t *testing.T) {
	output := captureStdout(func() {
		printMissingExtra("Formulae", []string{"ripgrep", "fd"}, []string{"htop"})
	})

	assert.Contains(t, output, "Formulae to install (2)")
	assert.Contains(t, output, "ripgrep, fd")
	assert.Contains(t, output, "Formulae extra (1)")
	assert.Contains(t, output, "htop")
}

func TestPrintMissingExtraOnlyMissing(t *testing.T) {
	output := captureStdout(func() {
		printMissingExtra("Casks", []string{"raycast"}, nil)
	})

	assert.Contains(t, output, "Casks to install (1)")
	assert.NotContains(t, output, "extra")
}

func TestPrintMissingExtraOnlyExtra(t *testing.T) {
	output := captureStdout(func() {
		printMissingExtra("NPM", nil, []string{"old-pkg"})
	})

	assert.Contains(t, output, "NPM extra (1)")
	assert.NotContains(t, output, "install")
}

func TestPrintMissingExtraBothEmpty(t *testing.T) {
	output := captureStdout(func() {
		printMissingExtra("Taps", nil, nil)
	})

	assert.Empty(t, output)
}

func TestPkgCategoryConstants(t *testing.T) {
	assert.Equal(t, pkgCategory("formulae"), categoryFormulae)
	assert.Equal(t, pkgCategory("casks"), categoryCasks)
	assert.Equal(t, pkgCategory("npm"), categoryNpm)
	assert.Equal(t, pkgCategory("taps"), categoryTaps)
}

func TestBuildDryRunPlan(t *testing.T) {
	d := &syncpkg.SyncDiff{
		MissingFormulae: []string{"ripgrep", "fd"},
		MissingCasks:    []string{"raycast"},
		MissingNpm:      []string{"turbo"},
		MissingTaps:     []string{"homebrew/cask-fonts"},
		DotfilesChanged: true,
		RemoteDotfiles:  "https://github.com/user/dots",
		ShellChanged:    true,
		ShellDiff: &syncpkg.ShellDiff{
			ThemeChanged:   true,
			RemoteTheme:    "agnoster",
			LocalTheme:     "robbyrussell",
			MissingPlugins: []string{"zsh-autosuggestions"},
		},
		MacOSChanged: []syncpkg.MacOSPrefDiff{
			{Domain: "com.apple.dock", Key: "autohide", RemoteValue: "true", Desc: "Dock auto-hide"},
		},
	}

	plan := buildDryRunPlan(d)

	assert.Equal(t, []string{"ripgrep", "fd"}, plan.InstallFormulae)
	assert.Equal(t, []string{"raycast"}, plan.InstallCasks)
	assert.Equal(t, []string{"turbo"}, plan.InstallNpm)
	assert.Equal(t, []string{"homebrew/cask-fonts"}, plan.InstallTaps)
	assert.Equal(t, "agnoster", plan.UpdateTheme)
	assert.Equal(t, []string{"zsh-autosuggestions"}, plan.InstallPlugins)
	assert.Equal(t, "https://github.com/user/dots", plan.UpdateDotfiles)
	assert.Len(t, plan.UpdateMacOSPrefs, 1)
	assert.Equal(t, "com.apple.dock", plan.UpdateMacOSPrefs[0].Domain)
}

func TestBuildDryRunPlanEmpty(t *testing.T) {
	d := &syncpkg.SyncDiff{}
	plan := buildDryRunPlan(d)
	assert.True(t, plan.IsEmpty())
}

func TestBuildDryRunPlanNoShellDiff(t *testing.T) {
	d := &syncpkg.SyncDiff{
		MissingFormulae: []string{"ripgrep"},
	}
	plan := buildDryRunPlan(d)
	assert.Equal(t, []string{"ripgrep"}, plan.InstallFormulae)
	assert.Empty(t, plan.UpdateTheme)
	assert.Empty(t, plan.InstallPlugins)
}
