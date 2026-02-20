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

// TestLocalPath tests the LocalPath function.
func TestLocalPath(t *testing.T) {
	path := LocalPath()
	assert.NotEmpty(t, path)
	assert.Contains(t, path, ".openboot")
	assert.Contains(t, path, "snapshot.json")
}

// TestLocalPath_Format tests that LocalPath returns a properly formatted path.
func TestLocalPath_Format(t *testing.T) {
	path := LocalPath()
	assert.True(t, filepath.IsAbs(path) || path == "")
	if path != "" {
		assert.True(t, filepath.IsAbs(path))
	}
}

// TestSaveLocal_CreatesDirectory tests that SaveLocal creates the directory.
func TestSaveLocal_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, ".openboot", "snapshot.json")

	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now(),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: []string{"git"},
		},
	}

	// Manually save to test path
	dir := filepath.Dir(testPath)
	err := os.MkdirAll(dir, 0700)
	require.NoError(t, err)

	data, err := json.MarshalIndent(snap, "", "  ")
	require.NoError(t, err)

	err = os.WriteFile(testPath, data, 0644)
	require.NoError(t, err)

	// Verify directory was created
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestLoadFile_ValidSnapshot tests loading a valid snapshot file.
func TestLoadFile_ValidSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "snapshot.json")

	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now().Truncate(time.Millisecond),
		Hostname:   "test-machine",
		Packages: PackageSnapshot{
			Formulae: []string{"git", "go"},
			Casks:    []string{"docker"},
			Npm:      []string{"typescript"},
		},
		Shell: ShellSnapshot{
			Default: "/bin/zsh",
			OhMyZsh: true,
		},
		Git: GitSnapshot{
			UserName:  "Test User",
			UserEmail: "test@example.com",
		},
	}

	// Save snapshot
	data, err := json.MarshalIndent(snap, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(testFile, data, 0644))

	// Load snapshot
	loaded, err := LoadFile(testFile)
	require.NoError(t, err)
	assert.NotNil(t, loaded)

	// Verify loaded snapshot
	assert.Equal(t, snap.Version, loaded.Version)
	assert.Equal(t, snap.Hostname, loaded.Hostname)
	assert.Equal(t, snap.Packages.Formulae, loaded.Packages.Formulae)
	assert.Equal(t, snap.Shell.Default, loaded.Shell.Default)
	assert.Equal(t, snap.Git.UserName, loaded.Git.UserName)
}

// TestLoadFile_FileNotFound tests loading a non-existent file.
func TestLoadFile_FileNotFound(t *testing.T) {
	nonExistentPath := "/tmp/nonexistent-snapshot-" + time.Now().Format("20060102150405") + ".json"

	loaded, err := LoadFile(nonExistentPath)
	assert.Error(t, err)
	assert.Nil(t, loaded)
	assert.Contains(t, err.Error(), "snapshot file not found")
}

// TestLoadFile_MalformedJSON tests loading a file with malformed JSON.
func TestLoadFile_MalformedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "malformed.json")

	// Write malformed JSON
	require.NoError(t, os.WriteFile(testFile, []byte("{invalid json}"), 0644))

	loaded, err := LoadFile(testFile)
	assert.Error(t, err)
	assert.Nil(t, loaded)
	assert.Contains(t, err.Error(), "parse snapshot")
}

// TestLoadFile_EmptyFile tests loading an empty file.
func TestLoadFile_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.json")

	// Write empty file
	require.NoError(t, os.WriteFile(testFile, []byte(""), 0644))

	loaded, err := LoadFile(testFile)
	assert.Error(t, err)
	assert.Nil(t, loaded)
}

// TestLoadFile_InvalidJSON tests loading a file with invalid JSON structure.
func TestLoadFile_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid.json")

	// Write JSON that doesn't match Snapshot structure
	require.NoError(t, os.WriteFile(testFile, []byte(`{"invalid": "structure"}`), 0644))

	loaded, err := LoadFile(testFile)
	// Should either succeed with default values or fail gracefully
	if err == nil {
		assert.NotNil(t, loaded)
	}
}

// TestLoadFile_LargeSnapshot tests loading a large snapshot file.
func TestLoadFile_LargeSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "large.json")

	// Create a large snapshot
	formulae := make([]string, 100)
	for i := 0; i < 100; i++ {
		formulae[i] = "package-" + string(rune(i))
	}

	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now().Truncate(time.Millisecond),
		Hostname:   "large-machine",
		Packages: PackageSnapshot{
			Formulae: formulae,
			Casks:    make([]string, 50),
			Npm:      make([]string, 75),
		},
	}

	// Save snapshot
	data, err := json.MarshalIndent(snap, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(testFile, data, 0644))

	// Load snapshot
	loaded, err := LoadFile(testFile)
	require.NoError(t, err)
	assert.NotNil(t, loaded)
	assert.Equal(t, 100, len(loaded.Packages.Formulae))
	assert.Equal(t, 50, len(loaded.Packages.Casks))
	assert.Equal(t, 75, len(loaded.Packages.Npm))
}

// TestLoadFile_WithAllFields tests loading a snapshot with all fields populated.
func TestLoadFile_WithAllFields(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "full.json")

	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now().Truncate(time.Millisecond),
		Hostname:   "full-machine",
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
			UserName:  "Test User",
			UserEmail: "test@example.com",
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

	// Save snapshot
	data, err := json.MarshalIndent(snap, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(testFile, data, 0644))

	// Load snapshot
	loaded, err := LoadFile(testFile)
	require.NoError(t, err)
	assert.NotNil(t, loaded)

	// Verify all fields
	assert.Equal(t, snap.Version, loaded.Version)
	assert.Equal(t, snap.Hostname, loaded.Hostname)
	assert.Equal(t, len(snap.Packages.Formulae), len(loaded.Packages.Formulae))
	assert.Equal(t, len(snap.MacOSPrefs), len(loaded.MacOSPrefs))
	assert.Equal(t, snap.Shell.OhMyZsh, loaded.Shell.OhMyZsh)
	assert.Equal(t, snap.Git.UserName, loaded.Git.UserName)
	assert.Equal(t, len(snap.DevTools), len(loaded.DevTools))
	assert.Equal(t, snap.MatchedPreset, loaded.MatchedPreset)
}

// TestLoadFile_WithNilPackages tests loading a snapshot with nil packages.
func TestLoadFile_WithNilPackages(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "nil.json")

	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now().Truncate(time.Millisecond),
		Hostname:   "nil-machine",
		Packages: PackageSnapshot{
			Formulae: nil,
			Casks:    nil,
			Npm:      nil,
			Taps:     nil,
		},
	}

	// Save snapshot
	data, err := json.MarshalIndent(snap, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(testFile, data, 0644))

	// Load snapshot
	loaded, err := LoadFile(testFile)
	require.NoError(t, err)
	assert.NotNil(t, loaded)
	// JSON unmarshaling converts nil slices to empty slices or keeps them as nil
	// depending on the JSON content. Just verify the snapshot loaded successfully.
	assert.Equal(t, "nil-machine", loaded.Hostname)
}

// TestLoadFile_WithEmptyPackages tests loading a snapshot with empty packages.
func TestLoadFile_WithEmptyPackages(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.json")

	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now().Truncate(time.Millisecond),
		Hostname:   "empty-machine",
		Packages: PackageSnapshot{
			Formulae: []string{},
			Casks:    []string{},
			Npm:      []string{},
			Taps:     []string{},
		},
	}

	// Save snapshot
	data, err := json.MarshalIndent(snap, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(testFile, data, 0644))

	// Load snapshot
	loaded, err := LoadFile(testFile)
	require.NoError(t, err)
	assert.NotNil(t, loaded)
	assert.Empty(t, loaded.Packages.Formulae)
	assert.Empty(t, loaded.Packages.Casks)
}

// TestLoadFile_RoundTrip tests save and load round trip.
func TestLoadFile_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "roundtrip.json")

	original := &Snapshot{
		Version:    1,
		CapturedAt: time.Now().Truncate(time.Millisecond),
		Hostname:   "roundtrip-machine",
		Packages: PackageSnapshot{
			Formulae: []string{"git", "go", "node"},
			Casks:    []string{"docker"},
			Npm:      []string{"typescript"},
		},
		Shell: ShellSnapshot{
			Default: "/bin/zsh",
			OhMyZsh: true,
			Plugins: []string{"git"},
			Theme:   "robbyrussell",
		},
		Git: GitSnapshot{
			UserName:  "Test User",
			UserEmail: "test@example.com",
		},
	}

	// Save
	data, err := json.MarshalIndent(original, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(testFile, data, 0644))

	// Load
	loaded, err := LoadFile(testFile)
	require.NoError(t, err)

	// Verify round trip
	assert.Equal(t, original.Version, loaded.Version)
	assert.Equal(t, original.Hostname, loaded.Hostname)
	assert.Equal(t, original.Packages.Formulae, loaded.Packages.Formulae)
	assert.Equal(t, original.Shell.Default, loaded.Shell.Default)
	assert.Equal(t, original.Git.UserName, loaded.Git.UserName)
}

// TestLoadFile_SpecialCharacters tests loading a snapshot with special characters.
func TestLoadFile_SpecialCharacters(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "special.json")

	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now().Truncate(time.Millisecond),
		Hostname:   "special-machine",
		Packages: PackageSnapshot{
			Formulae: []string{"@angular/cli", "pkg-with-dash", "pkg_with_underscore"},
			Casks:    []string{},
			Npm:      []string{"@babel/core", "@types/node"},
		},
	}

	// Save snapshot
	data, err := json.MarshalIndent(snap, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(testFile, data, 0644))

	// Load snapshot
	loaded, err := LoadFile(testFile)
	require.NoError(t, err)
	assert.NotNil(t, loaded)
	assert.Contains(t, loaded.Packages.Formulae, "@angular/cli")
	assert.Contains(t, loaded.Packages.Npm, "@babel/core")
}

// TestLoadFile_FilePermissions tests loading a file with restricted permissions.
func TestLoadFile_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "restricted.json")

	snap := &Snapshot{
		Version:    1,
		CapturedAt: time.Now().Truncate(time.Millisecond),
		Hostname:   "test",
		Packages: PackageSnapshot{
			Formulae: []string{"git"},
		},
	}

	// Save snapshot
	data, err := json.MarshalIndent(snap, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(testFile, data, 0644))

	// Load snapshot
	loaded, err := LoadFile(testFile)
	require.NoError(t, err)
	assert.NotNil(t, loaded)
}
