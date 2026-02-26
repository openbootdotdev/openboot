package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistentPreRunE_SilentEnvOverrides(t *testing.T) {
	oldCfg := cfg
	cfg = &config.Config{Silent: true}
	t.Cleanup(func() { cfg = oldCfg })

	t.Setenv("OPENBOOT_GIT_NAME", "Test User")
	t.Setenv("OPENBOOT_GIT_EMAIL", "test@example.com")
	t.Setenv("OPENBOOT_PRESET", "developer")

	err := rootCmd.PersistentPreRunE(rootCmd, []string{})
	require.NoError(t, err)
	assert.Equal(t, "Test User", cfg.GitName)
	assert.Equal(t, "test@example.com", cfg.GitEmail)
	assert.Equal(t, "developer", cfg.Preset)
}

func TestPersistentPreRunE_UserFetchesRemoteConfig(t *testing.T) {
	response := config.RemoteConfig{
		Username: "testuser",
		Slug:     "default",
		Preset:   "developer",
		Packages: []string{"git"},
		Casks:    []string{"firefox"},
		Npm:      []string{"typescript"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/configs/alias/testuser":
			// No alias set — return 404 to fall through to default
			w.WriteHeader(http.StatusNotFound)
		case "/testuser/default/config":
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(response))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	oldCfg := cfg
	cfg = &config.Config{}
	t.Cleanup(func() { cfg = oldCfg })

	t.Setenv("OPENBOOT_USER", "testuser")
	t.Setenv("OPENBOOT_API_URL", server.URL)
	t.Setenv("HOME", t.TempDir())

	err := rootCmd.PersistentPreRunE(rootCmd, []string{})
	require.NoError(t, err)
	require.NotNil(t, cfg.RemoteConfig)
	assert.Equal(t, "testuser", cfg.User)
	assert.Equal(t, "developer", cfg.Preset)
	assert.Equal(t, "developer", cfg.RemoteConfig.Preset)
}
