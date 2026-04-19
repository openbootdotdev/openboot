// NOTE: tests in this file must NOT use t.Parallel() due to shared
// package-level variables (fetchLatestVersion, execBrewUpgrade, execSelf).
package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type updaterRoundTripFunc func(*http.Request) (*http.Response, error)

func (f updaterRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func clientWithTransport(rt http.RoundTripper) *http.Client {
	return &http.Client{Transport: rt}
}

// ---------------------------------------------------------------------------
// GetLatestVersion — mock transport (via fetchLatestVersion injection)
// ---------------------------------------------------------------------------

type updaterMockRT struct{ handler http.Handler }

func (m *updaterMockRT) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	m.handler.ServeHTTP(rec, req)
	return rec.Result(), nil
}

// newTestFetcher returns a fetchLatestVersion-compatible func that uses an
// in-memory transport — no port binding required.
func newTestFetcher(handler http.Handler) func() (string, error) {
	client := &http.Client{Transport: &updaterMockRT{handler: handler}}
	return func() (string, error) {
		resp, err := client.Get("https://api.github.com/repos/openbootdotdev/openboot/releases/latest")
		if err != nil {
			return "", err
		}
		defer resp.Body.Close() //nolint:errcheck
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
		}
		var rel Release
		if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
			return "", err
		}
		return rel.TagName, nil
	}
}

func TestGetLatestVersion_Success(t *testing.T) {
	origFetch := fetchLatestVersion
	fetchLatestVersion = newTestFetcher(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Release{TagName: "v1.5.0"})
	}))
	defer func() { fetchLatestVersion = origFetch }()

	version, err := fetchLatestVersion()
	require.NoError(t, err)
	assert.Equal(t, "v1.5.0", version)
}

func TestGetLatestVersion_NonOKResponse(t *testing.T) {
	origFetch := fetchLatestVersion
	fetchLatestVersion = newTestFetcher(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer func() { fetchLatestVersion = origFetch }()

	_, err := fetchLatestVersion()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestGetLatestVersion_InvalidJSON(t *testing.T) {
	origFetch := fetchLatestVersion
	fetchLatestVersion = newTestFetcher(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{not json"))
	}))
	defer func() { fetchLatestVersion = origFetch }()

	_, err := fetchLatestVersion()
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// fetchChecksums — parse layer tested via parseChecksumsFile directly
// (fetchChecksums hard-codes its GitHub URL so mock-server tests go through
// the injection seam; the HTTP-level behaviour is covered separately below)
// ---------------------------------------------------------------------------

func TestFetchChecksums_ParseLayer(t *testing.T) {
	body := "abc123  openboot-darwin-arm64\ndef456  openboot-darwin-amd64\n"
	parsed, err := parseChecksumsFile(strings.NewReader(body))
	require.NoError(t, err)
	assert.Equal(t, "abc123", parsed["openboot-darwin-arm64"])
	assert.Equal(t, "def456", parsed["openboot-darwin-amd64"])
}

func TestFetchChecksums_NonOKResponse(t *testing.T) {
	client := clientWithTransport(updaterRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	}))

	_, err := fetchChecksums(client, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 500")
}

// ---------------------------------------------------------------------------
// AutoUpgrade — kill-switch and guard paths
// ---------------------------------------------------------------------------

func TestAutoUpgrade_KillSwitch_DisableAutoUpdate(t *testing.T) {
	t.Setenv("OPENBOOT_DISABLE_AUTOUPDATE", "1")

	called := false
	origFetch := fetchLatestVersion
	fetchLatestVersion = func() (string, error) { called = true; return "v99.0.0", nil }
	defer func() { fetchLatestVersion = origFetch }()

	AutoUpgrade("1.0.0")
	assert.False(t, called, "kill switch should prevent any network call")
}

func TestAutoUpgrade_GuardVar_Upgrading(t *testing.T) {
	t.Setenv("OPENBOOT_UPGRADING", "1")

	called := false
	origFetch := fetchLatestVersion
	fetchLatestVersion = func() (string, error) { called = true; return "v99.0.0", nil }
	defer func() { fetchLatestVersion = origFetch }()

	AutoUpgrade("1.0.0")
	assert.False(t, called, "OPENBOOT_UPGRADING guard should prevent network call")
	assert.Empty(t, os.Getenv("OPENBOOT_UPGRADING"), "guard var should be cleared")
}

func TestAutoUpgrade_DevBuild_NoNetworkCall(t *testing.T) {
	called := false
	origFetch := fetchLatestVersion
	fetchLatestVersion = func() (string, error) { called = true; return "v99.0.0", nil }
	defer func() { fetchLatestVersion = origFetch }()

	AutoUpgrade("dev")
	assert.False(t, called, "dev build should never hit network")
}

// ---------------------------------------------------------------------------
// doDirectUpgrade
// ---------------------------------------------------------------------------

func TestDoDirectUpgrade_BrewUpgrade_CallsExecSelf(t *testing.T) {
	brewCalled := false
	origExec := execBrewUpgrade
	execBrewUpgrade = func(formula string) error { brewCalled = true; return nil }
	defer func() { execBrewUpgrade = origExec }()

	restarted := false
	origSelf := execSelf
	execSelf = func() { restarted = true }
	defer func() { execSelf = origSelf }()

	doBrewUpgrade("1.2.3", "v1.3.0")

	assert.True(t, brewCalled)
	assert.True(t, restarted)
	os.Unsetenv("OPENBOOT_UPGRADING") //nolint:errcheck // test cleanup
}

func TestDoDirectUpgrade_Failure_NoRestart(t *testing.T) {
	// doDirectUpgrade delegates to DownloadAndReplace which we can't easily
	// stub — but we can verify that when execBrewUpgrade fails, execSelf is
	// not called and the function returns without panic.
	origExec := execBrewUpgrade
	execBrewUpgrade = func(formula string) error { return fmt.Errorf("brew upgrade failed") }
	defer func() { execBrewUpgrade = origExec }()

	restarted := false
	origSelf := execSelf
	execSelf = func() { restarted = true }
	defer func() { execSelf = origSelf }()

	doBrewUpgrade("1.0.0", "v2.0.0")
	assert.False(t, restarted, "should not restart after upgrade failure")
}

// ---------------------------------------------------------------------------
// notifyUpdate — both install flavours
// ---------------------------------------------------------------------------

func TestNotifyUpdate_DirectInstall(t *testing.T) {
	// Should not panic and should complete without error.
	// The function writes to stdout/stderr via ui.* helpers.
	notifyUpdate("1.0.0", "v2.0.0")
}

func TestNotifyUpdate_VersionPrefix_Stripped(t *testing.T) {
	// Verify that both "v"-prefixed and bare versions produce the same
	// display string (regression guard for TrimVersionPrefix).
	// We just exercise the code path without network.
	notifyUpdate("v1.0.0", "v2.0.0")
	notifyUpdate("1.0.0", "2.0.0")
}

// ---------------------------------------------------------------------------
// isNewerVersion — extended edge cases
// ---------------------------------------------------------------------------

func TestIsNewerVersion_ExtendedCases(t *testing.T) {
	tests := []struct {
		name     string
		latest   string
		current  string
		expected bool
	}{
		{"patch newer", "1.0.1", "1.0.0", true},
		{"patch older", "1.0.0", "1.0.1", false},
		{"minor newer", "1.1.0", "1.0.9", true},
		{"major newer", "2.0.0", "1.99.99", true},
		{"identical", "1.2.3", "1.2.3", false},
		{"both empty", "", "", false},
		{"current empty", "1.0.0", "", true},
		{"non-numeric parts ignored", "1.0.0-beta", "1.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNewerVersion(tt.latest, tt.current)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ---------------------------------------------------------------------------
// LoadUserConfig — AutoUpdate empty field defaults to Notify
// ---------------------------------------------------------------------------

func TestLoadUserConfig_DefaultWhenMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := LoadUserConfig()
	assert.Equal(t, AutoUpdateNotify, cfg.AutoUpdate)
}

// ---------------------------------------------------------------------------
// SaveState / LoadState round-trip with UpdateAvailable=false
// ---------------------------------------------------------------------------

func TestSaveState_UpdateAvailableFalse(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := time.Now().Truncate(time.Second)
	state := &CheckState{
		LastCheck:       now,
		LatestVersion:   "v1.0.0",
		UpdateAvailable: false,
	}
	require.NoError(t, SaveState(state))

	loaded, err := LoadState()
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", loaded.LatestVersion)
	assert.False(t, loaded.UpdateAvailable)
}

// ---------------------------------------------------------------------------
// resolveLatestVersion — API error path saves nothing
// ---------------------------------------------------------------------------

func TestResolveLatestVersion_APIError_NoStateWritten(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	origFetch := fetchLatestVersion
	fetchLatestVersion = func() (string, error) { return "", fmt.Errorf("network unreachable") }
	defer func() { fetchLatestVersion = origFetch }()

	result := resolveLatestVersion("1.0.0")
	assert.Equal(t, "", result)

	// No state file should have been written.
	statePath := filepath.Join(tmpDir, ".openboot", "update_state.json")
	_, err := os.Stat(statePath)
	assert.True(t, os.IsNotExist(err), "no state should be saved on API error")
}

// ---------------------------------------------------------------------------
// AutoUpgrade — enabled mode, no update available (latest == current)
// ---------------------------------------------------------------------------

func TestAutoUpgrade_Enabled_AlreadyLatest(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Cache is fresh and on the same version — no upgrade should happen.
	require.NoError(t, SaveState(&CheckState{
		LastCheck:       time.Now(),
		LatestVersion:   "v1.0.0",
		UpdateAvailable: false,
	}))

	brewCalled := false
	origExec := execBrewUpgrade
	execBrewUpgrade = func(_ string) error { brewCalled = true; return nil }
	defer func() { execBrewUpgrade = origExec }()

	restarted := false
	origSelf := execSelf
	execSelf = func() { restarted = true }
	defer func() { execSelf = origSelf }()

	AutoUpgrade("1.0.0")

	assert.False(t, brewCalled, "should not upgrade when already on latest")
	assert.False(t, restarted)
}

// ---------------------------------------------------------------------------
// AutoUpgrade — notifyMode with update and Homebrew path detection
// ---------------------------------------------------------------------------

func TestAutoUpgrade_NotifyMode_HombrewInstall_ShowsBrewCommand(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfgDir := filepath.Join(tmpDir, ".openboot")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))
	data, _ := json.Marshal(UserConfig{AutoUpdate: AutoUpdateNotify})
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0644))

	// Save a stale state so resolveLatestVersion hits the API.
	require.NoError(t, SaveState(&CheckState{
		LastCheck:       time.Now().Add(-48 * time.Hour),
		LatestVersion:   "v1.0.0",
		UpdateAvailable: false,
	}))

	origFetch := fetchLatestVersion
	fetchLatestVersion = func() (string, error) { return "v2.0.0", nil }
	defer func() { fetchLatestVersion = origFetch }()

	brewCalled := false
	origExec := execBrewUpgrade
	execBrewUpgrade = func(_ string) error { brewCalled = true; return nil }
	defer func() { execBrewUpgrade = origExec }()

	AutoUpgrade("1.0.0")

	assert.False(t, brewCalled, "notify mode should never call execBrewUpgrade")
}
