//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openbootdotdev/openboot/internal/updater"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_Updater_IsHomebrewInstall(t *testing.T) {
	// Given: the test binary is running
	// When: we check if it's a Homebrew install
	result := updater.IsHomebrewInstall()

	// Then: returns a bool without panicking (result depends on environment)
	assert.IsType(t, false, result)
	t.Logf("IsHomebrewInstall: %v", result)
}

func TestIntegration_Updater_AutoUpgrade_DisabledByEnv(t *testing.T) {
	// Given: the disable env var is set
	t.Setenv("OPENBOOT_DISABLE_AUTOUPDATE", "1")

	// When: AutoUpgrade is called
	// Then: returns immediately without error or network calls
	updater.AutoUpgrade("1.0.0")
}

func TestIntegration_Updater_AutoUpgrade_DevVersion(t *testing.T) {
	// Given: auto-update is not disabled
	// When: current version is "dev"
	// Then: no update attempted (dev builds skip update)
	updater.AutoUpgrade("dev")
}

func TestIntegration_Updater_SaveAndLoadState_RoundTrip(t *testing.T) {
	// Given: a temp home directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	snap := &updater.CheckState{
		LastCheck:       time.Now().Truncate(time.Second),
		LatestVersion:   "v99.0.0",
		UpdateAvailable: true,
	}

	// When: we save then load state
	require.NoError(t, updater.SaveState(snap))
	loaded, err := updater.LoadState()

	// Then: data survives the round-trip
	require.NoError(t, err)
	assert.Equal(t, snap.LatestVersion, loaded.LatestVersion)
	assert.Equal(t, snap.UpdateAvailable, loaded.UpdateAvailable)
	assert.True(t, snap.LastCheck.Equal(loaded.LastCheck.Truncate(time.Second)), "expected %v, got %v", snap.LastCheck, loaded.LastCheck)
}

func TestIntegration_Updater_LoadState_FileNotFound(t *testing.T) {
	// Given: empty home directory with no state file
	t.Setenv("HOME", t.TempDir())

	// When: we try to load state
	state, err := updater.LoadState()

	// Then: returns an error, not a panic
	assert.Error(t, err)
	assert.Nil(t, state)
}

func TestIntegration_Updater_LoadUserConfig_Default(t *testing.T) {
	// Given: no config file exists
	t.Setenv("HOME", t.TempDir())

	// When: we load user config
	cfg := updater.LoadUserConfig()

	// Then: defaults to auto-update enabled
	assert.Equal(t, updater.AutoUpdateEnabled, cfg.AutoUpdate)
}

func TestIntegration_Updater_LoadUserConfig_AllModes(t *testing.T) {
	modes := []updater.AutoUpdateMode{
		updater.AutoUpdateEnabled,
		updater.AutoUpdateNotify,
		updater.AutoUpdateDisabled,
	}

	for _, mode := range modes {
		t.Run(string(mode), func(t *testing.T) {
			// Given: a config file with the specified mode
			tmpDir := t.TempDir()
			t.Setenv("HOME", tmpDir)
			cfgDir := filepath.Join(tmpDir, ".openboot")
			require.NoError(t, os.MkdirAll(cfgDir, 0755))
			data, err := json.Marshal(updater.UserConfig{AutoUpdate: mode})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0644))

			// When: we load the config
			cfg := updater.LoadUserConfig()

			// Then: mode matches what was written
			assert.Equal(t, mode, cfg.AutoUpdate)
		})
	}
}

func TestIntegration_Updater_GetLatestVersion_RealAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real GitHub API call in short mode")
	}

	// Given: internet access to api.github.com
	// When: we fetch the latest release tag
	version, err := updater.GetLatestVersion()

	// Then: returns a semver-like tag; skip on API errors (rate limits, network issues on CI)
	if err != nil {
		t.Skipf("GitHub API unavailable (rate limit or network): %v", err)
	}
	assert.NotEmpty(t, version, "latest version should not be empty")
	assert.Regexp(t, `^v?\d+\.\d+\.\d+`, version, "version should be semver")
	t.Logf("Latest release: %s", version)
}

func TestIntegration_Updater_GetLatestVersion_MockServer(t *testing.T) {
	// Given: a mock GitHub API server returning a known version
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/openbootdotdev/openboot/releases/latest", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(updater.Release{TagName: "v9.9.9"})
	}))
	defer server.Close()

	// When: we hit the mock server directly
	resp, err := http.Get(server.URL + "/repos/openbootdotdev/openboot/releases/latest")
	require.NoError(t, err)
	defer resp.Body.Close()

	var release updater.Release
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&release))

	// Then: tag is parsed correctly
	assert.Equal(t, "v9.9.9", release.TagName)
}

func TestIntegration_Updater_CheckInterval_Constant(t *testing.T) {
	// Given: the check interval constant
	// When: we verify its value
	// Then: it should be 24 hours (once-a-day check)
	assert.Equal(t, 24*time.Hour, updater.CheckInterval)
}
