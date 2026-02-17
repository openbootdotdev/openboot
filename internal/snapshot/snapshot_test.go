package snapshot

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestSnapshot_Creation tests basic snapshot creation and field initialization.
func TestSnapshot_Creation(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test-machine",
		Packages: PackageSnapshot{
			Formulae: []string{"git", "go"},
			Casks:    []string{"docker"},
			Npm:      []string{"typescript"},
		},
	}

	assert.Equal(t, 1, snap.Version)
	assert.Equal(t, "test-machine", snap.Hostname)
	assert.Equal(t, 2, len(snap.Packages.Formulae))
	assert.Equal(t, 1, len(snap.Packages.Casks))
	assert.Equal(t, 1, len(snap.Packages.Npm))
}

// TestPackageSnapshot_Empty tests empty package snapshot.
func TestPackageSnapshot_Empty(t *testing.T) {
	ps := PackageSnapshot{}

	assert.Empty(t, ps.Formulae)
	assert.Empty(t, ps.Casks)
	assert.Empty(t, ps.Npm)
	assert.Empty(t, ps.Taps)
}

// TestMacOSPref_Creation tests MacOS preference creation.
func TestMacOSPref_Creation(t *testing.T) {
	pref := MacOSPref{
		Domain: "com.apple.finder",
		Key:    "ShowPathbar",
		Value:  "1",
		Desc:   "Show path bar in Finder",
	}

	assert.Equal(t, "com.apple.finder", pref.Domain)
	assert.Equal(t, "ShowPathbar", pref.Key)
	assert.Equal(t, "1", pref.Value)
	assert.Equal(t, "Show path bar in Finder", pref.Desc)
}

// TestShellSnapshot_Creation tests shell snapshot creation.
func TestShellSnapshot_Creation(t *testing.T) {
	shell := ShellSnapshot{
		Default: "/bin/zsh",
		OhMyZsh: true,
		Plugins: []string{"git", "docker"},
		Theme:   "robbyrussell",
	}

	assert.Equal(t, "/bin/zsh", shell.Default)
	assert.True(t, shell.OhMyZsh)
	assert.Equal(t, 2, len(shell.Plugins))
	assert.Equal(t, "robbyrussell", shell.Theme)
}

// TestGitSnapshot_Creation tests git snapshot creation.
func TestGitSnapshot_Creation(t *testing.T) {
	git := GitSnapshot{
		UserName:  "Test User",
		UserEmail: "test@example.com",
	}

	assert.Equal(t, "Test User", git.UserName)
	assert.Equal(t, "test@example.com", git.UserEmail)
}

// TestDevTool_Creation tests dev tool creation.
func TestDevTool_Creation(t *testing.T) {
	tool := DevTool{
		Name:    "go",
		Version: "1.22.0",
	}

	assert.Equal(t, "go", tool.Name)
	assert.Equal(t, "1.22.0", tool.Version)
}

// TestCatalogMatch_Creation tests catalog match creation.
func TestCatalogMatch_Creation(t *testing.T) {
	match := CatalogMatch{
		Matched:   []string{"git", "go"},
		Unmatched: []string{"unknown"},
		MatchRate: 0.667,
	}

	assert.Equal(t, 2, len(match.Matched))
	assert.Equal(t, 1, len(match.Unmatched))
	assert.InDelta(t, 0.667, match.MatchRate, 0.001)
}

// TestSnapshot_WithAllFields tests snapshot with all fields populated.
func TestSnapshot_WithAllFields(t *testing.T) {
	now := time.Now()
	snap := &Snapshot{
		Version:    1,
		CapturedAt: now,
		Hostname:   "dev-machine",
		Packages: PackageSnapshot{
			Formulae: []string{"git", "go", "node"},
			Casks:    []string{"docker", "vscode"},
			Taps:     []string{"homebrew/cask"},
			Npm:      []string{"typescript", "eslint"},
		},
		MacOSPrefs: []MacOSPref{
			{Domain: "com.apple.finder", Key: "ShowPathbar", Value: "1", Desc: "Show path bar"},
		},
		Shell: ShellSnapshot{
			Default: "/bin/zsh",
			OhMyZsh: true,
			Plugins: []string{"git", "docker"},
			Theme:   "robbyrussell",
		},
		Git: GitSnapshot{
			UserName:  "Developer",
			UserEmail: "dev@example.com",
		},
		DevTools: []DevTool{
			{Name: "go", Version: "1.22.0"},
			{Name: "node", Version: "20.11.0"},
		},
		MatchedPreset: "developer",
		CatalogMatch: CatalogMatch{
			Matched:   []string{"git", "go", "node", "docker"},
			Unmatched: []string{"vscode"},
			MatchRate: 0.8,
		},
	}

	assert.Equal(t, 1, snap.Version)
	assert.Equal(t, "dev-machine", snap.Hostname)
	assert.Equal(t, 3, len(snap.Packages.Formulae))
	assert.Equal(t, 2, len(snap.Packages.Casks))
	assert.Equal(t, 1, len(snap.MacOSPrefs))
	assert.True(t, snap.Shell.OhMyZsh)
	assert.Equal(t, "Developer", snap.Git.UserName)
	assert.Equal(t, 2, len(snap.DevTools))
	assert.Equal(t, "developer", snap.MatchedPreset)
	assert.Equal(t, 4, len(snap.CatalogMatch.Matched))
}

// TestSnapshot_EmptyPackages tests snapshot with empty packages.
func TestSnapshot_EmptyPackages(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "empty-machine",
		Packages: PackageSnapshot{
			Formulae: []string{},
			Casks:    []string{},
			Npm:      []string{},
			Taps:     []string{},
		},
	}

	assert.Empty(t, snap.Packages.Formulae)
	assert.Empty(t, snap.Packages.Casks)
	assert.Empty(t, snap.Packages.Npm)
	assert.Empty(t, snap.Packages.Taps)
}

// TestSnapshot_NilPackages tests snapshot with nil packages.
func TestSnapshot_NilPackages(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "nil-machine",
		Packages: PackageSnapshot{
			Formulae: nil,
			Casks:    nil,
			Npm:      nil,
			Taps:     nil,
		},
	}

	assert.Nil(t, snap.Packages.Formulae)
	assert.Nil(t, snap.Packages.Casks)
	assert.Nil(t, snap.Packages.Npm)
	assert.Nil(t, snap.Packages.Taps)
}

// TestSnapshot_LargePackageLists tests snapshot with large package lists.
func TestSnapshot_LargePackageLists(t *testing.T) {
	formulae := make([]string, 100)
	for i := 0; i < 100; i++ {
		formulae[i] = "package-" + string(rune(i))
	}

	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "large-machine",
		Packages: PackageSnapshot{
			Formulae: formulae,
			Casks:    make([]string, 50),
			Npm:      make([]string, 75),
		},
	}

	assert.Equal(t, 100, len(snap.Packages.Formulae))
	assert.Equal(t, 50, len(snap.Packages.Casks))
	assert.Equal(t, 75, len(snap.Packages.Npm))
}

// TestSnapshot_SpecialCharactersInPackageNames tests packages with special characters.
func TestSnapshot_SpecialCharactersInPackageNames(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "special-machine",
		Packages: PackageSnapshot{
			Formulae: []string{"@angular/cli", "pkg-with-dash", "pkg_with_underscore"},
			Casks:    []string{},
			Npm:      []string{"@babel/core", "@types/node"},
		},
	}

	assert.Equal(t, 3, len(snap.Packages.Formulae))
	assert.Equal(t, 2, len(snap.Packages.Npm))
	assert.Contains(t, snap.Packages.Formulae, "@angular/cli")
	assert.Contains(t, snap.Packages.Npm, "@babel/core")
}

// TestSnapshot_DuplicatePackages tests snapshot with duplicate packages.
func TestSnapshot_DuplicatePackages(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "dup-machine",
		Packages: PackageSnapshot{
			Formulae: []string{"git", "git", "go"},
			Casks:    []string{"docker", "docker"},
			Npm:      []string{"typescript"},
		},
	}

	assert.Equal(t, 3, len(snap.Packages.Formulae))
	assert.Equal(t, 2, len(snap.Packages.Casks))
	assert.Equal(t, 1, len(snap.Packages.Npm))
}

// TestSnapshot_EmptyHostname tests snapshot with empty hostname.
func TestSnapshot_EmptyHostname(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "",
		Packages: PackageSnapshot{
			Formulae: []string{"git"},
		},
	}

	assert.Empty(t, snap.Hostname)
}

// TestSnapshot_ZeroVersion tests snapshot with zero version.
func TestSnapshot_ZeroVersion(t *testing.T) {
	snap := &Snapshot{
		Version:    0,
		CapturedAt: time.Now(),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: []string{"git"},
		},
	}

	assert.Equal(t, 0, snap.Version)
}

// TestSnapshot_MultipleDevTools tests snapshot with multiple dev tools.
func TestSnapshot_MultipleDevTools(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "dev-machine",
		DevTools: []DevTool{
			{Name: "go", Version: "1.22.0"},
			{Name: "node", Version: "20.11.0"},
			{Name: "python3", Version: "3.12.0"},
			{Name: "rustc", Version: "1.75.0"},
			{Name: "java", Version: "21.0.1"},
			{Name: "ruby", Version: "3.2.2"},
			{Name: "docker", Version: "24.0.7"},
		},
	}

	assert.Equal(t, 7, len(snap.DevTools))
	assert.Equal(t, "go", snap.DevTools[0].Name)
	assert.Equal(t, "docker", snap.DevTools[6].Name)
}

// TestSnapshot_MultipleMacOSPrefs tests snapshot with multiple macOS preferences.
func TestSnapshot_MultipleMacOSPrefs(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "mac-machine",
		MacOSPrefs: []MacOSPref{
			{Domain: "com.apple.finder", Key: "ShowPathbar", Value: "1", Desc: "Show path bar"},
			{Domain: "com.apple.dock", Key: "autohide", Value: "1", Desc: "Auto-hide dock"},
			{Domain: "com.apple.menuextra.clock", Key: "DateFormat", Value: "EEE MMM d  h:mm:ss a", Desc: "Clock format"},
		},
	}

	assert.Equal(t, 3, len(snap.MacOSPrefs))
	assert.Equal(t, "com.apple.finder", snap.MacOSPrefs[0].Domain)
	assert.Equal(t, "com.apple.dock", snap.MacOSPrefs[1].Domain)
}

// TestSnapshot_ShellWithMultiplePlugins tests shell snapshot with multiple plugins.
func TestSnapshot_ShellWithMultiplePlugins(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "shell-machine",
		Shell: ShellSnapshot{
			Default: "/bin/zsh",
			OhMyZsh: true,
			Plugins: []string{"git", "docker", "kubectl", "aws", "node", "npm", "yarn"},
			Theme:   "robbyrussell",
		},
	}

	assert.Equal(t, 7, len(snap.Shell.Plugins))
	assert.Contains(t, snap.Shell.Plugins, "kubectl")
	assert.Contains(t, snap.Shell.Plugins, "aws")
}

// TestSnapshot_CatalogMatchWithHighRate tests catalog match with high match rate.
func TestSnapshot_CatalogMatchWithHighRate(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "match-machine",
		CatalogMatch: CatalogMatch{
			Matched:   []string{"git", "go", "node", "docker", "typescript"},
			Unmatched: []string{},
			MatchRate: 1.0,
		},
	}

	assert.Equal(t, 5, len(snap.CatalogMatch.Matched))
	assert.Equal(t, 0, len(snap.CatalogMatch.Unmatched))
	assert.Equal(t, 1.0, snap.CatalogMatch.MatchRate)
}

// TestCaptureHealth_DefaultIsHealthy verifies a zero-value CaptureHealth is not partial.
func TestCaptureHealth_DefaultIsHealthy(t *testing.T) {
	snap := &Snapshot{Version: 1}
	assert.False(t, snap.Health.Partial)
	assert.Empty(t, snap.Health.FailedSteps)
}

// TestCaptureHealth_PartialWhenStepsFail verifies Health reflects failed steps.
func TestCaptureHealth_PartialWhenStepsFail(t *testing.T) {
	snap := &Snapshot{
		Health: CaptureHealth{
			FailedSteps: []string{"Homebrew Formulae", "Homebrew Casks"},
			Partial:     true,
		},
	}
	assert.True(t, snap.Health.Partial)
	assert.Equal(t, 2, len(snap.Health.FailedSteps))
	assert.Contains(t, snap.Health.FailedSteps, "Homebrew Formulae")
}

// TestSnapshot_CatalogMatchWithLowRate tests catalog match with low match rate.
func TestSnapshot_CatalogMatchWithLowRate(t *testing.T) {
	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "low-match-machine",
		CatalogMatch: CatalogMatch{
			Matched:   []string{"git"},
			Unmatched: []string{"unknown1", "unknown2", "unknown3", "unknown4"},
			MatchRate: 0.2,
		},
	}

	assert.Equal(t, 1, len(snap.CatalogMatch.Matched))
	assert.Equal(t, 4, len(snap.CatalogMatch.Unmatched))
	assert.InDelta(t, 0.2, snap.CatalogMatch.MatchRate, 0.01)
}
