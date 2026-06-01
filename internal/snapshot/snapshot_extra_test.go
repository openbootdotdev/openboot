package snapshot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// SaveLocal / LoadLocal
// ---------------------------------------------------------------------------

func TestSaveLocal_CreatesFileAndDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "save-test",
		Packages: PackageSnapshot{
			Formulae: []string{"git", "curl"},
			Casks:    []string{"docker"},
		},
	}

	path, err := SaveLocal(snap, false)
	require.NoError(t, err)
	assert.NotEmpty(t, path)
	assert.Contains(t, path, ".openboot")
	assert.Contains(t, path, "snapshot.json")

	// File must exist.
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr)
}

func TestSaveLocal_FileIsValidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now().Truncate(time.Second),
		Hostname:   "json-test",
		Git: GitSnapshot{
			UserName:  "Alice",
			UserEmail: "alice@example.com",
		},
	}

	path, err := SaveLocal(snap, false)
	require.NoError(t, err)

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)

	var loaded Snapshot
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.Equal(t, "json-test", loaded.Hostname)
	assert.Equal(t, "Alice", loaded.Git.UserName)
}

func TestSaveLocal_LoadLocal_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	original := &Snapshot{
		Version:    1,
		CapturedAt: time.Now().Truncate(time.Millisecond),
		Hostname:   "roundtrip",
		Packages: PackageSnapshot{
			Formulae: []string{"git", "ripgrep"},
			Casks:    []string{"firefox"},
			Taps:     []string{"homebrew/cask"},
			Npm:      []string{"typescript"},
		},
		Git: GitSnapshot{
			UserName:  "Bob",
			UserEmail: "bob@example.com",
		},
		DevTools: []DevTool{
			{Name: "go", Version: "1.22.0"},
		},
	}

	_, err := SaveLocal(original, false)
	require.NoError(t, err)

	loaded, err := LoadLocal()
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, original.Hostname, loaded.Hostname)
	assert.Equal(t, original.Packages.Formulae, loaded.Packages.Formulae)
	assert.Equal(t, original.Packages.Casks, loaded.Packages.Casks)
	assert.Equal(t, original.Packages.Taps, loaded.Packages.Taps)
	assert.Equal(t, original.Packages.Npm, loaded.Packages.Npm)
	assert.Equal(t, original.Git.UserName, loaded.Git.UserName)
	assert.Equal(t, original.Git.UserEmail, loaded.Git.UserEmail)
	assert.Equal(t, 1, len(loaded.DevTools))
	assert.Equal(t, "go", loaded.DevTools[0].Name)
}

func TestSaveLocal_AtomicWrite_TmpFileCleaned(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	snap := &Snapshot{Version: 1, CapturedAt: time.Now(), Hostname: "atomic"}

	path, err := SaveLocal(snap, false)
	require.NoError(t, err)

	// The .tmp file should not exist after a successful save.
	_, statErr := os.Stat(path + ".tmp")
	assert.True(t, os.IsNotExist(statErr), ".tmp file should be cleaned up after atomic rename")
}

func TestSaveLocal_OverwritesPreviousSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	first := &Snapshot{Version: 1, CapturedAt: time.Now(), Hostname: "first"}
	_, err := SaveLocal(first, false)
	require.NoError(t, err)

	second := &Snapshot{Version: 1, CapturedAt: time.Now(), Hostname: "second"}
	_, err = SaveLocal(second, false)
	require.NoError(t, err)

	loaded, err := LoadLocal()
	require.NoError(t, err)
	assert.Equal(t, "second", loaded.Hostname)
}

func TestLoadLocal_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	// No snapshot saved — should return an error.
	_, err := LoadLocal()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot file not found")
}

func TestLocalPath_ContainsOpenboot(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path := LocalPath()
	assert.True(t, filepath.IsAbs(path))
	assert.Contains(t, path, ".openboot")
	assert.Contains(t, path, "snapshot.json")
}

// ---------------------------------------------------------------------------
// UnmarshalJSON — PackageSnapshot additional edge cases
// ---------------------------------------------------------------------------

func TestPackageSnapshot_UnmarshalJSON_RichObjectWithDesc(t *testing.T) {
	// Rich object format with descriptions.
	input := `{
		"formulae": [{"name":"git","desc":"Version control system"}],
		"casks": [{"name":"docker","desc":"Container platform"}],
		"npm": [{"name":"typescript","desc":"Typed JavaScript"}],
		"taps": ["homebrew/cask"]
	}`

	var ps PackageSnapshot
	err := json.Unmarshal([]byte(input), &ps)
	require.NoError(t, err)

	assert.Equal(t, []string{"git"}, ps.Formulae)
	assert.Equal(t, []string{"docker"}, ps.Casks)
	assert.Equal(t, []string{"typescript"}, ps.Npm)
	assert.Equal(t, []string{"homebrew/cask"}, ps.Taps)
	require.NotNil(t, ps.Descriptions)
	assert.Equal(t, "Version control system", ps.Descriptions["git"])
	assert.Equal(t, "Container platform", ps.Descriptions["docker"])
}

func TestPackageSnapshot_UnmarshalJSON_EmptyObject(t *testing.T) {
	input := `{"formulae":[],"casks":[],"taps":[],"npm":[]}`

	var ps PackageSnapshot
	err := json.Unmarshal([]byte(input), &ps)
	require.NoError(t, err)
	assert.Empty(t, ps.Formulae)
	assert.Empty(t, ps.Casks)
}

func TestPackageSnapshot_UnmarshalJSON_TypedArrayDefaultIsFormula(t *testing.T) {
	// Unknown type defaults to formula.
	input := `[{"name":"mypkg","type":"unknown-type"}]`

	var ps PackageSnapshot
	err := json.Unmarshal([]byte(input), &ps)
	require.NoError(t, err)
	assert.Contains(t, ps.Formulae, "mypkg")
}

func TestPackageSnapshot_UnmarshalJSON_FlatArrayAllFormulae(t *testing.T) {
	input := `["git","curl","ripgrep"]`

	var ps PackageSnapshot
	err := json.Unmarshal([]byte(input), &ps)
	require.NoError(t, err)
	assert.Equal(t, []string{"git", "curl", "ripgrep"}, ps.Formulae)
	assert.Empty(t, ps.Casks)
	assert.Empty(t, ps.Npm)
}

func TestPackageSnapshot_UnmarshalJSON_InvalidType(t *testing.T) {
	var ps PackageSnapshot
	err := json.Unmarshal([]byte(`42`), &ps)
	assert.Error(t, err)
}

func TestPackageSnapshot_UnmarshalJSON_BoolInvalid(t *testing.T) {
	var ps PackageSnapshot
	err := json.Unmarshal([]byte(`true`), &ps)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// MarshalJSON — PackageSnapshot
// ---------------------------------------------------------------------------

func TestPackageSnapshot_MarshalJSON_EmptySlices(t *testing.T) {
	ps := PackageSnapshot{
		Formulae: []string{},
		Casks:    []string{},
		Taps:     []string{},
		Npm:      []string{},
	}

	data, err := json.Marshal(ps)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	_, hasFormulae := raw["formulae"]
	assert.True(t, hasFormulae)
}

func TestPackageSnapshot_MarshalJSON_NilSlices(t *testing.T) {
	ps := PackageSnapshot{}

	data, err := json.Marshal(ps)
	require.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestPackageSnapshot_MarshalJSON_DescriptionsNotSerialized(t *testing.T) {
	ps := PackageSnapshot{
		Formulae:     []string{"git"},
		Descriptions: map[string]string{"git": "Version control"},
	}

	data, err := json.Marshal(ps)
	require.NoError(t, err)

	// "descriptions" key must not appear in the serialized output.
	assert.NotContains(t, string(data), `"descriptions"`)
	assert.Contains(t, string(data), `"git"`)
}

// ---------------------------------------------------------------------------
// CaptureHealth
// ---------------------------------------------------------------------------

func TestCaptureHealth_Serialization(t *testing.T) {
	health := CaptureHealth{
		FailedSteps: []string{"Homebrew Formulae"},
		Partial:     true,
	}

	data, err := json.Marshal(health)
	require.NoError(t, err)

	var loaded CaptureHealth
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.True(t, loaded.Partial)
	assert.Equal(t, []string{"Homebrew Formulae"}, loaded.FailedSteps)
}

func TestCaptureHealth_EmptyFailedSteps(t *testing.T) {
	health := CaptureHealth{
		FailedSteps: []string{},
		Partial:     false,
	}

	data, err := json.Marshal(health)
	require.NoError(t, err)

	var loaded CaptureHealth
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.False(t, loaded.Partial)
	assert.Empty(t, loaded.FailedSteps)
}

// ---------------------------------------------------------------------------
// Snapshot full serialization round-trip
// ---------------------------------------------------------------------------

func TestSnapshot_FullRoundTrip(t *testing.T) {
	original := &Snapshot{
		Version:    1,
		CapturedAt: time.Now().Truncate(time.Millisecond),
		Hostname:   "myhost",
		Packages: PackageSnapshot{
			Formulae: []string{"git", "go"},
			Casks:    []string{"docker"},
			Taps:     []string{"homebrew/cask"},
			Npm:      []string{"typescript"},
		},
		MacOSPrefs: []MacOSPref{
			{Domain: "com.apple.finder", Key: "ShowPathbar", Type: "bool", Value: "true", Desc: "Show path bar"},
		},
		Git: GitSnapshot{
			UserName:  "Alice",
			UserEmail: "alice@example.com",
		},
		Dotfiles: DotfilesSnapshot{
			RepoURL: "https://github.com/alice/dotfiles",
		},
		DevTools: []DevTool{
			{Name: "go", Version: "1.22.0"},
			{Name: "node", Version: "20.11.0"},
		},
		MatchedPreset: "developer",
		CatalogMatch: CatalogMatch{
			Matched:   []string{"git", "go"},
			Unmatched: []string{"custom-pkg"},
			MatchRate: 0.75,
		},
		Health: CaptureHealth{
			FailedSteps: []string{},
			Partial:     false,
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var loaded Snapshot
	require.NoError(t, json.Unmarshal(data, &loaded))

	assert.Equal(t, original.Version, loaded.Version)
	assert.Equal(t, original.Hostname, loaded.Hostname)
	assert.Equal(t, original.Packages.Formulae, loaded.Packages.Formulae)
	assert.Equal(t, original.Packages.Casks, loaded.Packages.Casks)
	assert.Equal(t, original.Packages.Taps, loaded.Packages.Taps)
	assert.Equal(t, original.Packages.Npm, loaded.Packages.Npm)
	assert.Equal(t, original.Git.UserName, loaded.Git.UserName)
	assert.Equal(t, original.Git.UserEmail, loaded.Git.UserEmail)
	assert.Equal(t, original.Dotfiles.RepoURL, loaded.Dotfiles.RepoURL)
	assert.Equal(t, original.MatchedPreset, loaded.MatchedPreset)
	assert.InDelta(t, original.CatalogMatch.MatchRate, loaded.CatalogMatch.MatchRate, 0.001)
	assert.False(t, loaded.Health.Partial)
}

// ---------------------------------------------------------------------------
// sanitizePath — edge cases
// ---------------------------------------------------------------------------

func TestSanitizePath_HomeDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	result := sanitizePath(tmpDir)
	assert.Equal(t, "~", result)
}

func TestSanitizePath_SubdirOfHome(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	subdir := filepath.Join(tmpDir, "projects", "myapp")
	result := sanitizePath(subdir)
	assert.Equal(t, "~/projects/myapp", result)
}

func TestSanitizePath_NotUnderHome(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	result := sanitizePath("/usr/local/bin")
	assert.Equal(t, "/usr/local/bin", result)
}

func TestSanitizePath_Empty(t *testing.T) {
	result := sanitizePath("")
	assert.Equal(t, "", result)
}

// ---------------------------------------------------------------------------
// parseLines — edge cases beyond existing tests
// ---------------------------------------------------------------------------

func TestParseLines_WindowsLineEndings(t *testing.T) {
	input := "git\r\ncurl\r\ngo"
	result := parseLines(input)
	// TrimSpace on each line removes \r.
	assert.Len(t, result, 3)
	for _, line := range result {
		assert.NotContains(t, line, "\r")
	}
}

func TestParseLines_OnlyWhitespace(t *testing.T) {
	result := parseLines("   \n   \t   ")
	assert.Empty(t, result)
}

// ---------------------------------------------------------------------------
// parseVersion — additional tool formats
// ---------------------------------------------------------------------------

func TestParseVersion_GoMissingVersion(t *testing.T) {
	// Malformed go version output
	result := parseVersion("go", "not the usual format")
	assert.Equal(t, "", result)
}

func TestParseVersion_RustcShort(t *testing.T) {
	// Fewer fields than expected
	result := parseVersion("rustc", "rustc")
	assert.Equal(t, "", result)
}

func TestParseVersion_JavaMultiLine(t *testing.T) {
	output := "openjdk 17.0.5 2022-10-18\nOpenJDK Runtime Environment"
	result := parseVersion("java", output)
	assert.Equal(t, "17.0.5", result)
}

func TestParseVersion_DockerCommaStripped(t *testing.T) {
	result := parseVersion("docker", "Docker version 25.0.3, build 4debf41")
	assert.Equal(t, "25.0.3", result)
}

func TestParseVersion_DockerShortOutput(t *testing.T) {
	result := parseVersion("docker", "Docker version")
	assert.Equal(t, "", result)
}

func TestParseVersion_RubyFull(t *testing.T) {
	output := "ruby 3.3.0 (2023-12-25 revision 5124f9ac75) [arm64-darwin23]"
	result := parseVersion("ruby", output)
	assert.Equal(t, "3.3.0", result)
}

func TestParseVersion_UnknownToolReturnsRaw(t *testing.T) {
	result := parseVersion("madeuptools", "v1.0.0 stable")
	assert.Equal(t, "v1.0.0 stable", result)
}

// ---------------------------------------------------------------------------
// CaptureShell
// ---------------------------------------------------------------------------

func TestCaptureShell_NoDotZshrc(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	snap, err := CaptureShell()
	require.NoError(t, err)
	require.NotNil(t, snap)
	// No .zshrc — theme and plugins should be empty.
	assert.Empty(t, snap.Theme)
	assert.Empty(t, snap.Plugins)
}

func TestCaptureShell_WithZshrcThemeAndPlugins(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	zshrc := `export ZSH="$HOME/.oh-my-zsh"
ZSH_THEME="robbyrussell"
plugins=(git brew docker)
source $ZSH/oh-my-zsh.sh`

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".zshrc"), []byte(zshrc), 0644))

	snap, err := CaptureShell()
	require.NoError(t, err)
	require.NotNil(t, snap)
	assert.Equal(t, "robbyrussell", snap.Theme)
	assert.Contains(t, snap.Plugins, "git")
	assert.Contains(t, snap.Plugins, "brew")
	assert.Contains(t, snap.Plugins, "docker")
}

func TestCaptureShell_WithOhMyZshDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create .oh-my-zsh directory to trigger OhMyZsh detection.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".oh-my-zsh"), 0755))

	snap, err := CaptureShell()
	require.NoError(t, err)
	require.NotNil(t, snap)
	assert.True(t, snap.OhMyZsh)
}

func TestCaptureShell_WithoutOhMyZshDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	snap, err := CaptureShell()
	require.NoError(t, err)
	require.NotNil(t, snap)
	assert.False(t, snap.OhMyZsh)
}

func TestCaptureShell_EmptyTheme(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// .zshrc without ZSH_THEME set.
	zshrc := `export PATH="$HOME/bin:$PATH"`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".zshrc"), []byte(zshrc), 0644))

	snap, err := CaptureShell()
	require.NoError(t, err)
	require.NotNil(t, snap)
	assert.Empty(t, snap.Theme)
	assert.Empty(t, snap.Plugins)
}

func TestCaptureShell_EmptyPlugins(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// .zshrc with theme but empty plugins.
	zshrc := `ZSH_THEME="agnoster"
plugins=()`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".zshrc"), []byte(zshrc), 0644))

	snap, err := CaptureShell()
	require.NoError(t, err)
	require.NotNil(t, snap)
	assert.Equal(t, "agnoster", snap.Theme)
	assert.Empty(t, snap.Plugins)
}

// ---------------------------------------------------------------------------
// CaptureDotfiles — additional filesystem scenarios
// ---------------------------------------------------------------------------

func TestCaptureDotfiles_DotfilesWithGitAndRemote(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create a minimal git repo in .dotfiles
	dotfilesDir := filepath.Join(tmpDir, ".dotfiles")
	require.NoError(t, os.MkdirAll(filepath.Join(dotfilesDir, ".git"), 0755))

	// CaptureGit.remote.get-url will fail without a proper git repo, so the result
	// will have an empty RepoURL — that is the expected safe behaviour.
	snap, err := CaptureDotfiles()
	require.NoError(t, err)
	require.NotNil(t, snap)
	// Cannot verify the URL without a real git repo, but must not panic.
	_ = snap.RepoURL
}

// ---------------------------------------------------------------------------
// jaccardSimilarity (tested through DetectBestPreset, but also directly)
// ---------------------------------------------------------------------------

func TestJaccardSimilarity_IdenticalSets(t *testing.T) {
	a := []string{"git", "curl", "go"}
	b := []string{"git", "curl", "go"}
	sim := jaccardSimilarity(a, b)
	assert.InDelta(t, 1.0, sim, 0.001)
}

func TestJaccardSimilarity_DisjointSets(t *testing.T) {
	a := []string{"git", "curl"}
	b := []string{"npm", "node"}
	sim := jaccardSimilarity(a, b)
	assert.InDelta(t, 0.0, sim, 0.001)
}

func TestJaccardSimilarity_EmptySets(t *testing.T) {
	sim := jaccardSimilarity([]string{}, []string{})
	assert.InDelta(t, 0.0, sim, 0.001)
}

func TestJaccardSimilarity_OneEmptySet(t *testing.T) {
	a := []string{"git", "curl"}
	sim := jaccardSimilarity(a, []string{})
	assert.InDelta(t, 0.0, sim, 0.001)
}

func TestJaccardSimilarity_PartialOverlap(t *testing.T) {
	// |A ∩ B| = 2, |A ∪ B| = 4 → 0.5
	a := []string{"git", "curl"}
	b := []string{"git", "curl", "go", "node"}
	sim := jaccardSimilarity(a, b)
	assert.InDelta(t, 0.5, sim, 0.001)
}

func TestJaccardSimilarity_DuplicatesHandled(t *testing.T) {
	// Sets deduplicate — duplicates should not inflate union.
	a := []string{"git", "git", "curl"}
	b := []string{"git", "curl"}
	sim := jaccardSimilarity(a, b)
	// With deduplication: |{git,curl} ∩ {git,curl}| / |{git,curl} ∪ {git,curl}| = 2/2 = 1.0
	assert.InDelta(t, 1.0, sim, 0.001)
}

// ---------------------------------------------------------------------------
// MatchPackages — additional cases
// ---------------------------------------------------------------------------

func TestMatchPackages_NilPackages(t *testing.T) {
	snap := &Snapshot{
		Packages: PackageSnapshot{
			Formulae: nil,
			Casks:    nil,
			Npm:      nil,
		},
	}

	match := MatchPackages(snap)
	require.NotNil(t, match)
	assert.Equal(t, 0.0, match.MatchRate)
	assert.Empty(t, match.Matched)
	assert.Empty(t, match.Unmatched)
}

func TestMatchPackages_OnlyCasks(t *testing.T) {
	snap := &Snapshot{
		Packages: PackageSnapshot{
			Casks: []string{"unknown-cask-xyz"},
		},
	}

	match := MatchPackages(snap)
	require.NotNil(t, match)
	assert.Equal(t, 1, len(match.Matched)+len(match.Unmatched))
}

func TestMatchPackages_MixedAllTypes(t *testing.T) {
	snap := &Snapshot{
		Packages: PackageSnapshot{
			Formulae: []string{"git"},
			Casks:    []string{"unknown-cask-xyz"},
			Npm:      []string{"unknown-npm-xyz"},
		},
	}

	match := MatchPackages(snap)
	require.NotNil(t, match)
	total := len(match.Matched) + len(match.Unmatched)
	assert.Equal(t, 3, total)
}

func TestMatchPackages_MatchRateIsProportional(t *testing.T) {
	// All 4 should be unmatched → rate = 0
	snap := &Snapshot{
		Packages: PackageSnapshot{
			Formulae: []string{"zzz-unknown1", "zzz-unknown2", "zzz-unknown3", "zzz-unknown4"},
		},
	}

	match := MatchPackages(snap)
	require.NotNil(t, match)
	assert.Equal(t, 0.0, match.MatchRate)
	assert.Empty(t, match.Matched)
	assert.Len(t, match.Unmatched, 4)
}

// ---------------------------------------------------------------------------
// LoadFile — additional scenarios
// ---------------------------------------------------------------------------

func TestLoadFile_SnapshotWithHealth(t *testing.T) {
	tmpDir := t.TempDir()
	snapFile := filepath.Join(tmpDir, "health.json")

	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now().Truncate(time.Millisecond),
		Hostname:   "health-host",
		Health: CaptureHealth{
			FailedSteps: []string{"Homebrew Formulae"},
			Partial:     true,
		},
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(snapFile, data, 0644))

	loaded, err := LoadFile(snapFile)
	require.NoError(t, err)
	assert.True(t, loaded.Health.Partial)
	assert.Equal(t, []string{"Homebrew Formulae"}, loaded.Health.FailedSteps)
}

func TestLoadFile_SnapshotWithDotfiles(t *testing.T) {
	tmpDir := t.TempDir()
	snapFile := filepath.Join(tmpDir, "dotfiles.json")

	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now().Truncate(time.Millisecond),
		Hostname:   "dotfiles-host",
		Dotfiles: DotfilesSnapshot{
			RepoURL: "https://github.com/user/dotfiles",
		},
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(snapFile, data, 0644))

	loaded, err := LoadFile(snapFile)
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/user/dotfiles", loaded.Dotfiles.RepoURL)
}

func TestLoadFile_SnapshotWithCatalogMatch(t *testing.T) {
	tmpDir := t.TempDir()
	snapFile := filepath.Join(tmpDir, "catalog.json")

	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now().Truncate(time.Millisecond),
		CatalogMatch: CatalogMatch{
			Matched:   []string{"git", "curl"},
			Unmatched: []string{"custom-tool"},
			MatchRate: 0.666,
		},
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(snapFile, data, 0644))

	loaded, err := LoadFile(snapFile)
	require.NoError(t, err)
	assert.InDelta(t, 0.666, loaded.CatalogMatch.MatchRate, 0.001)
	assert.Contains(t, loaded.CatalogMatch.Matched, "git")
	assert.Contains(t, loaded.CatalogMatch.Unmatched, "custom-tool")
}
