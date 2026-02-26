package updater

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		name     string
		latest   string
		current  string
		expected bool
	}{
		{
			name:     "empty latest version",
			latest:   "",
			current:  "1.0.0",
			expected: false,
		},
		{
			name:     "same version",
			latest:   "1.0.0",
			current:  "1.0.0",
			expected: false,
		},
		{
			name:     "newer version",
			latest:   "2.0.0",
			current:  "1.0.0",
			expected: true,
		},
		{
			name:     "older version",
			latest:   "1.0.0",
			current:  "2.0.0",
			expected: false,
		},
		{
			name:     "latest with v prefix",
			latest:   "v2.0.0",
			current:  "1.0.0",
			expected: true,
		},
		{
			name:     "current with v prefix",
			latest:   "2.0.0",
			current:  "v1.0.0",
			expected: true,
		},
		{
			name:     "both with v prefix",
			latest:   "v2.0.0",
			current:  "v1.0.0",
			expected: true,
		},
		{
			name:     "same version with different prefixes",
			latest:   "v1.0.0",
			current:  "1.0.0",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNewerVersion(tt.latest, tt.current)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsHomebrewPath(t *testing.T) {
	homebrewPaths := []string{
		"/opt/homebrew/Cellar/openboot/0.21.0/bin/openboot",
		"/usr/local/Homebrew/Cellar/openboot/0.21.0/bin/openboot",
		"/opt/homebrew/bin/openboot",
		"/home/linuxbrew/.linuxbrew/Cellar/openboot/0.21.0/bin/openboot",
	}
	for _, p := range homebrewPaths {
		assert.True(t, isHomebrewPath(p), "should detect Homebrew path: %s", p)
	}

	nonHomebrewPaths := []string{
		"/usr/local/bin/openboot",
		"/Users/user/.openboot/bin/openboot",
		"/tmp/openboot",
	}
	for _, p := range nonHomebrewPaths {
		assert.False(t, isHomebrewPath(p), "should not detect as Homebrew: %s", p)
	}
}

func TestTrimVersionPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "with v prefix",
			input:    "v1.2.3",
			expected: "1.2.3",
		},
		{
			name:     "without v prefix",
			input:    "1.2.3",
			expected: "1.2.3",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "just v",
			input:    "v",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimVersionPrefix(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"2.0.0", "1.0.0", 1},
		{"1.0.0", "2.0.0", -1},
		{"1.0.0", "1.0.0", 0},
		{"1.2.3", "1.2.2", 1},
		{"1.2.2", "1.2.3", -1},
		{"1.10.0", "1.9.0", 1},
		{"0.0.1", "0.0.0", 1},
	}
	for _, tt := range tests {
		result := compareSemver(tt.a, tt.b)
		assert.Equal(t, tt.expected, result, "compareSemver(%q, %q)", tt.a, tt.b)
	}
}

func TestParseSemver(t *testing.T) {
	tests := []struct {
		input    string
		expected [3]int
	}{
		{"1.2.3", [3]int{1, 2, 3}},
		{"10.20.30", [3]int{10, 20, 30}},
		{"1.0.0", [3]int{1, 0, 0}},
		{"", [3]int{0, 0, 0}},
		{"abc", [3]int{0, 0, 0}},
		{"1.abc.3", [3]int{1, 0, 3}},
	}
	for _, tt := range tests {
		result := parseSemver(tt.input)
		assert.Equal(t, tt.expected, result, "parseSemver(%q)", tt.input)
	}
}

func TestIsNewerVersion_DevBuild(t *testing.T) {
	assert.False(t, isNewerVersion("v99.0.0", "dev"), "dev builds should never trigger update")
}

func TestGetHTTPClient_Singleton(t *testing.T) {
	c1 := getHTTPClient()
	c2 := getHTTPClient()
	assert.Same(t, c1, c2, "getHTTPClient should return same instance")
	assert.NotNil(t, c1)
}

func TestGetCheckFilePath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, err := getCheckFilePath()
	require.NoError(t, err)
	assert.Contains(t, path, ".openboot")
	assert.Contains(t, path, "update_state.json")
}

func TestGetUserConfigPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, err := getUserConfigPath()
	require.NoError(t, err)
	assert.Contains(t, path, ".openboot")
	assert.Contains(t, path, "config.json")
}

func TestLoadState_FileNotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := LoadState()
	assert.Error(t, err)
}

func TestSaveState_And_LoadState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := time.Now().Truncate(time.Second)
	state := &CheckState{
		LastCheck:       now,
		LatestVersion:   "v1.2.3",
		UpdateAvailable: true,
	}
	require.NoError(t, SaveState(state))

	loaded, err := LoadState()
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", loaded.LatestVersion)
	assert.True(t, loaded.UpdateAvailable)
	assert.True(t, now.Equal(loaded.LastCheck.Truncate(time.Second)), "expected %v, got %v", now, loaded.LastCheck)
}

func TestLoadUserConfig_Default_NoFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := LoadUserConfig()
	assert.Equal(t, AutoUpdateEnabled, cfg.AutoUpdate)
}

func TestLoadUserConfig_FromFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".openboot")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))
	data, err := json.Marshal(UserConfig{AutoUpdate: AutoUpdateNotify})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0644))

	cfg := LoadUserConfig()
	assert.Equal(t, AutoUpdateNotify, cfg.AutoUpdate)
}

func TestLoadUserConfig_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".openboot")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte("{bad json"), 0644))

	cfg := LoadUserConfig()
	assert.Equal(t, AutoUpdateEnabled, cfg.AutoUpdate)
}

func TestLoadUserConfig_EmptyAutoUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".openboot")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"autoupdate":""}`), 0644))

	cfg := LoadUserConfig()
	assert.Equal(t, AutoUpdateEnabled, cfg.AutoUpdate)
}

func TestLoadUserConfig_DisabledMode(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".openboot")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))
	data, err := json.Marshal(UserConfig{AutoUpdate: AutoUpdateDisabled})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0644))

	cfg := LoadUserConfig()
	assert.Equal(t, AutoUpdateDisabled, cfg.AutoUpdate)
}

func TestAutoUpgrade_DisabledByEnv(t *testing.T) {
	t.Setenv("OPENBOOT_DISABLE_AUTOUPDATE", "1")
	AutoUpgrade("1.0.0")
}

func TestNotifyIfUpdateAvailable_NoStateFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	NotifyIfUpdateAvailable("1.0.0")
}

func TestNotifyIfUpdateAvailable_UpdateAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	require.NoError(t, SaveState(&CheckState{
		LastCheck:       time.Now(),
		LatestVersion:   "v2.0.0",
		UpdateAvailable: true,
	}))
	NotifyIfUpdateAvailable("1.0.0")
}

func TestNotifyIfUpdateAvailable_NoUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	require.NoError(t, SaveState(&CheckState{
		LastCheck:       time.Now(),
		LatestVersion:   "v1.0.0",
		UpdateAvailable: false,
	}))
	NotifyIfUpdateAvailable("1.0.0")
}

func TestAutoUpdateModeConstants(t *testing.T) {
	assert.Equal(t, AutoUpdateMode("true"), AutoUpdateEnabled)
	assert.Equal(t, AutoUpdateMode("notify"), AutoUpdateNotify)
	assert.Equal(t, AutoUpdateMode("false"), AutoUpdateDisabled)
}

func TestHomebrewAutoUpgrade_NoStateFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	called := false
	origExec := execBrewUpgrade
	execBrewUpgrade = func(formula string) error { called = true; return nil }
	defer func() { execBrewUpgrade = origExec }()

	homebrewAutoUpgrade("1.0.0")

	assert.False(t, called, "brew should not run when there is no state file")
}

func TestHomebrewAutoUpgrade_NoUpdateAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	require.NoError(t, SaveState(&CheckState{
		LastCheck:       time.Now(),
		LatestVersion:   "v1.0.0",
		UpdateAvailable: false,
	}))

	called := false
	origExec := execBrewUpgrade
	execBrewUpgrade = func(formula string) error { called = true; return nil }
	defer func() { execBrewUpgrade = origExec }()

	homebrewAutoUpgrade("1.0.0")

	assert.False(t, called, "brew should not run when no update is available")
}

func TestHomebrewAutoUpgrade_UpdateAvailable_Success(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	require.NoError(t, SaveState(&CheckState{
		LastCheck:       time.Now(),
		LatestVersion:   "v2.0.0",
		UpdateAvailable: true,
	}))

	var calledWith string
	origExec := execBrewUpgrade
	execBrewUpgrade = func(formula string) error { calledWith = formula; return nil }
	defer func() { execBrewUpgrade = origExec }()

	homebrewAutoUpgrade("1.0.0")

	assert.Equal(t, "openboot", calledWith, "should call brew upgrade openboot")
}

func TestHomebrewAutoUpgrade_UpdateAvailable_Failure(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	require.NoError(t, SaveState(&CheckState{
		LastCheck:       time.Now(),
		LatestVersion:   "v2.0.0",
		UpdateAvailable: true,
	}))

	origExec := execBrewUpgrade
	execBrewUpgrade = func(formula string) error { return fmt.Errorf("brew failed") }
	defer func() { execBrewUpgrade = origExec }()

	// should not panic
	homebrewAutoUpgrade("1.0.0")
}
