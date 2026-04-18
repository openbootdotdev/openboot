// NOTE: tests in this file must NOT use t.Parallel() due to shared
// package-level variables (fetchLatestVersion, execBrewUpgrade).
package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Version comparison ---

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

func TestIsNewerVersion_DevBuild(t *testing.T) {
	assert.False(t, isNewerVersion("v99.0.0", "dev"), "dev builds should never trigger update")
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

func TestTrimVersionPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"with v prefix", "v1.2.3", "1.2.3"},
		{"without v prefix", "1.2.3", "1.2.3"},
		{"empty string", "", ""},
		{"just v", "v", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, trimVersionPrefix(tt.input))
		})
	}
}

// --- Path detection ---

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

// --- State persistence ---

func TestGetCheckFilePath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, err := getCheckFilePath()
	require.NoError(t, err)
	assert.Contains(t, path, ".openboot")
	assert.Contains(t, path, "update_state.json")
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

// --- User config ---

func TestGetUserConfigPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, err := getUserConfigPath()
	require.NoError(t, err)
	assert.Contains(t, path, ".openboot")
	assert.Contains(t, path, "config.json")
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

// --- HTTP client ---

func TestGetHTTPClient_Singleton(t *testing.T) {
	c1 := getHTTPClient()
	c2 := getHTTPClient()
	assert.Same(t, c1, c2, "getHTTPClient should return same instance")
	assert.NotNil(t, c1)
}

// --- AutoUpgrade kill switches ---

func TestAutoUpgrade_DisabledByEnv(t *testing.T) {
	t.Setenv("OPENBOOT_DISABLE_AUTOUPDATE", "1")
	AutoUpgrade("1.0.0") // should return immediately
}

func TestAutoUpgrade_DevVersion(t *testing.T) {
	// dev builds should never trigger any network call or upgrade
	AutoUpgrade("dev")
}

func TestAutoUpgrade_UserDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".openboot")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))
	data, _ := json.Marshal(UserConfig{AutoUpdate: AutoUpdateDisabled})
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0644))

	// Should not make any network call — inject fetchLatestVersion to verify
	called := false
	origFetch := fetchLatestVersion
	fetchLatestVersion = func() (string, error) { called = true; return "v99.0.0", nil }
	defer func() { fetchLatestVersion = origFetch }()

	AutoUpgrade("1.0.0")

	assert.False(t, called, "disabled mode should not check for updates")
}

// --- resolveLatestVersion ---

func TestResolveLatestVersion_FreshCache(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	require.NoError(t, SaveState(&CheckState{
		LastCheck:       time.Now(),
		LatestVersion:   "v2.0.0",
		UpdateAvailable: true,
	}))

	// Should NOT call the API
	called := false
	origFetch := fetchLatestVersion
	fetchLatestVersion = func() (string, error) { called = true; return "v3.0.0", nil }
	defer func() { fetchLatestVersion = origFetch }()

	result := resolveLatestVersion("1.0.0")

	assert.Equal(t, "v2.0.0", result, "should return cached version")
	assert.False(t, called, "should not call API when cache is fresh")
}

func TestResolveLatestVersion_StaleCache(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	require.NoError(t, SaveState(&CheckState{
		LastCheck:       time.Now().Add(-48 * time.Hour),
		LatestVersion:   "v1.0.0",
		UpdateAvailable: false,
	}))

	origFetch := fetchLatestVersion
	fetchLatestVersion = func() (string, error) { return "v2.0.0", nil }
	defer func() { fetchLatestVersion = origFetch }()

	result := resolveLatestVersion("1.0.0")

	assert.Equal(t, "v2.0.0", result, "should return fresh version from API")

	// Verify state was saved
	state, err := LoadState()
	require.NoError(t, err)
	assert.Equal(t, "v2.0.0", state.LatestVersion)
	assert.True(t, state.UpdateAvailable)
}

func TestResolveLatestVersion_NoCache(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	origFetch := fetchLatestVersion
	fetchLatestVersion = func() (string, error) { return "v2.0.0", nil }
	defer func() { fetchLatestVersion = origFetch }()

	result := resolveLatestVersion("1.0.0")

	assert.Equal(t, "v2.0.0", result)

	// Verify state was saved for next time
	state, err := LoadState()
	require.NoError(t, err)
	assert.Equal(t, "v2.0.0", state.LatestVersion)
}

func TestResolveLatestVersion_APIError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	origFetch := fetchLatestVersion
	fetchLatestVersion = func() (string, error) { return "", fmt.Errorf("network error") }
	defer func() { fetchLatestVersion = origFetch }()

	result := resolveLatestVersion("1.0.0")

	assert.Equal(t, "", result, "should return empty on API error")
}

func TestResolveLatestVersion_SameVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	origFetch := fetchLatestVersion
	fetchLatestVersion = func() (string, error) { return "v1.0.0", nil }
	defer func() { fetchLatestVersion = origFetch }()

	result := resolveLatestVersion("1.0.0")

	assert.Equal(t, "v1.0.0", result)

	// State should record UpdateAvailable=false
	state, err := LoadState()
	require.NoError(t, err)
	assert.False(t, state.UpdateAvailable)
}

// --- doBrewUpgrade ---

func TestDoBrewUpgrade_Success(t *testing.T) {
	var calledWith string
	origExec := execBrewUpgrade
	execBrewUpgrade = func(formula string) error { calledWith = formula; return nil }
	defer func() { execBrewUpgrade = origExec }()

	restarted := false
	origSelf := execSelf
	execSelf = func() { restarted = true }
	defer func() { execSelf = origSelf }()

	doBrewUpgrade("1.0.0", "v2.0.0")

	assert.Equal(t, brewFormula, calledWith)
	assert.True(t, restarted, "should re-exec after successful upgrade")
	assert.Equal(t, "1", os.Getenv("OPENBOOT_UPGRADING"), "should set OPENBOOT_UPGRADING before re-exec")
	os.Unsetenv("OPENBOOT_UPGRADING")
}

func TestDoBrewUpgrade_Failure(t *testing.T) {
	called := false
	origExec := execBrewUpgrade
	execBrewUpgrade = func(formula string) error { called = true; return fmt.Errorf("brew failed") }
	defer func() { execBrewUpgrade = origExec }()

	restarted := false
	origSelf := execSelf
	execSelf = func() { restarted = true }
	defer func() { execSelf = origSelf }()

	doBrewUpgrade("1.0.0", "v2.0.0")

	assert.True(t, called, "brew should have been attempted")
	assert.False(t, restarted, "should not re-exec on failed upgrade")
}

func TestAutoUpgrade_SkipsWhenUpgrading(t *testing.T) {
	t.Setenv("OPENBOOT_UPGRADING", "1")

	// If AutoUpgrade does NOT skip, it would call resolveLatestVersion
	// which would try the network. The test passing without network
	// proves the guard works.
	AutoUpgrade("1.0.0")

	// Env var should be cleared after being consumed
	assert.Empty(t, os.Getenv("OPENBOOT_UPGRADING"), "should unset OPENBOOT_UPGRADING after consuming it")
}

// --- End-to-end: AutoUpgrade with Homebrew (via resolveLatestVersion) ---
// Note: AutoUpgrade calls IsHomebrewInstall() which checks the actual binary path.
// In tests, the binary is not in a Homebrew path, so it goes down the direct path.
// We test the Homebrew upgrade logic via doBrewUpgrade directly.

func TestAutoUpgrade_NotifyMode_ShowsMessage(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Set notify mode
	cfgDir := filepath.Join(tmpDir, ".openboot")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))
	data, _ := json.Marshal(UserConfig{AutoUpdate: AutoUpdateNotify})
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0644))

	origFetch := fetchLatestVersion
	fetchLatestVersion = func() (string, error) { return "v2.0.0", nil }
	defer func() { fetchLatestVersion = origFetch }()

	// Should not attempt brew or download — just notify
	brewCalled := false
	origExec := execBrewUpgrade
	execBrewUpgrade = func(formula string) error { brewCalled = true; return nil }
	defer func() { execBrewUpgrade = origExec }()

	AutoUpgrade("1.0.0")

	assert.False(t, brewCalled, "notify mode should not trigger upgrade")
}

func TestAutoUpgrade_NotifyMode_NoUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfgDir := filepath.Join(tmpDir, ".openboot")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))
	data, _ := json.Marshal(UserConfig{AutoUpdate: AutoUpdateNotify})
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0644))

	origFetch := fetchLatestVersion
	fetchLatestVersion = func() (string, error) { return "v1.0.0", nil }
	defer func() { fetchLatestVersion = origFetch }()

	// Should not panic or error when already on latest
	AutoUpgrade("1.0.0")
}

// --- Checksum verification ---

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, data, 0644))
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestParseChecksumsFile(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr string
	}{
		{
			name: "standard sha256sum format",
			input: "abc123  openboot-darwin-arm64\n" +
				"def456  openboot-darwin-amd64\n",
			want: map[string]string{
				"openboot-darwin-arm64": "abc123",
				"openboot-darwin-amd64": "def456",
			},
		},
		{
			name:  "binary mode asterisk prefix",
			input: "abc123 *openboot-darwin-arm64\n",
			want:  map[string]string{"openboot-darwin-arm64": "abc123"},
		},
		{
			name:  "leading dot-slash is stripped",
			input: "abc123  ./openboot-darwin-arm64\n",
			want:  map[string]string{"openboot-darwin-arm64": "abc123"},
		},
		{
			name:  "blank lines and comments ignored",
			input: "# header\n\nabc123  openboot-darwin-arm64\n",
			want:  map[string]string{"openboot-darwin-arm64": "abc123"},
		},
		{
			name:  "hash is lowercased",
			input: "ABCDEF123  openboot-darwin-arm64\n",
			want:  map[string]string{"openboot-darwin-arm64": "abcdef123"},
		},
		{
			name:    "empty file rejected",
			input:   "",
			wantErr: "empty or unparseable",
		},
		{
			name:    "only comments rejected",
			input:   "# nothing here\n",
			wantErr: "empty or unparseable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseChecksumsFile(strings.NewReader(tt.input))
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVerifyChecksum(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "openboot-darwin-arm64")
	payload := []byte("fake binary contents for test")
	writeFile(t, binPath, payload)
	expectedHash := sha256Hex(payload)

	t.Run("matching hash passes", func(t *testing.T) {
		err := verifyChecksum(binPath, "openboot-darwin-arm64", map[string]string{
			"openboot-darwin-arm64": expectedHash,
		})
		assert.NoError(t, err)
	})

	t.Run("mismatched hash fails", func(t *testing.T) {
		err := verifyChecksum(binPath, "openboot-darwin-arm64", map[string]string{
			"openboot-darwin-arm64": strings.Repeat("0", 64),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "checksum mismatch")
	})

	t.Run("missing entry fails", func(t *testing.T) {
		err := verifyChecksum(binPath, "openboot-darwin-arm64", map[string]string{
			"openboot-darwin-amd64": expectedHash,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no checksum entry")
	})

	t.Run("hash comparison is case-insensitive", func(t *testing.T) {
		err := verifyChecksum(binPath, "openboot-darwin-arm64", map[string]string{
			"openboot-darwin-arm64": strings.ToUpper(expectedHash),
		})
		assert.NoError(t, err)
	})

	t.Run("missing file fails", func(t *testing.T) {
		err := verifyChecksum(filepath.Join(tmpDir, "does-not-exist"), "openboot-darwin-arm64", map[string]string{
			"openboot-darwin-arm64": expectedHash,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "open")
	})
}
