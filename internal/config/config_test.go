package config

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPreset(t *testing.T) {
	tests := []struct {
		name      string
		presetKey string
		found     bool
		hasName   bool
	}{
		{
			name:      "valid preset minimal",
			presetKey: "minimal",
			found:     true,
			hasName:   true,
		},
		{
			name:      "valid preset developer",
			presetKey: "developer",
			found:     true,
			hasName:   true,
		},
		{
			name:      "valid preset full",
			presetKey: "full",
			found:     true,
			hasName:   true,
		},
		{
			name:      "invalid preset",
			presetKey: "nonexistent",
			found:     false,
			hasName:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preset, found := GetPreset(tt.presetKey)
			assert.Equal(t, tt.found, found)
			if tt.hasName {
				assert.Equal(t, tt.presetKey, preset.Name)
			}
		})
	}
}

func TestGetPresetNames(t *testing.T) {
	names := GetPresetNames()

	assert.Equal(t, 3, len(names))
	assert.Equal(t, "minimal", names[0])
	assert.Equal(t, "developer", names[1])
	assert.Equal(t, "full", names[2])
}

func TestFetchRemoteConfig_PublicConfig_NoToken(t *testing.T) {
	mockConfig := RemoteConfig{
		Username:     "testuser",
		Slug:         "myconfig",
		Name:         "Test Config",
		Preset:       "developer",
		Packages:     []string{"git", "curl"},
		Casks:        []string{"firefox"},
		Taps:         []string{"homebrew/cask-fonts"},
		Npm:          []string{"typescript"},
		DotfilesRepo: "https://github.com/testuser/dotfiles",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/testuser/myconfig/config", r.URL.Path)
		assert.Empty(t, r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockConfig)
	}))
	defer server.Close()

	originalClient := remoteHTTPClient
	remoteHTTPClient = server.Client()
	defer func() { remoteHTTPClient = originalClient }()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	result, err := FetchRemoteConfig("testuser/myconfig", "")
	require.NoError(t, err)
	assert.Equal(t, "testuser", result.Username)
	assert.Equal(t, "myconfig", result.Slug)
	assert.Equal(t, "developer", result.Preset)
	assert.Len(t, result.Packages, 2)
}

func TestFetchRemoteConfig_PrivateConfig_NoToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/testuser/private/config", r.URL.Path)
		assert.Empty(t, r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	originalClient := remoteHTTPClient
	remoteHTTPClient = server.Client()
	defer func() { remoteHTTPClient = originalClient }()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	result, err := FetchRemoteConfig("testuser/private", "")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "config testuser/private is private")
	assert.Contains(t, err.Error(), "run 'openboot login' first")
}

func TestFetchRemoteConfig_PrivateConfig_WithValidToken(t *testing.T) {
	mockConfig := RemoteConfig{
		Username: "testuser",
		Slug:     "private",
		Name:     "Private Config",
		Preset:   "full",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/testuser/private/config", r.URL.Path)
		assert.Equal(t, "Bearer obt_valid_token", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockConfig)
	}))
	defer server.Close()

	originalClient := remoteHTTPClient
	remoteHTTPClient = server.Client()
	defer func() { remoteHTTPClient = originalClient }()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	result, err := FetchRemoteConfig("testuser/private", "obt_valid_token")
	require.NoError(t, err)
	assert.Equal(t, "testuser", result.Username)
	assert.Equal(t, "private", result.Slug)
	assert.Equal(t, "full", result.Preset)
}

func TestFetchRemoteConfig_PrivateConfig_WithInvalidToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/testuser/private/config", r.URL.Path)
		assert.Equal(t, "Bearer obt_invalid_token", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	originalClient := remoteHTTPClient
	remoteHTTPClient = server.Client()
	defer func() { remoteHTTPClient = originalClient }()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	result, err := FetchRemoteConfig("testuser/private", "obt_invalid_token")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "config testuser/private is private")
	assert.Contains(t, err.Error(), "you don't have access")
}

func TestFetchRemoteConfig_ConfigNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/testuser/nonexistent/config", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	originalClient := remoteHTTPClient
	remoteHTTPClient = server.Client()
	defer func() { remoteHTTPClient = originalClient }()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	result, err := FetchRemoteConfig("testuser/nonexistent", "")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "config not found: testuser/nonexistent")
}

func TestFetchRemoteConfig_DefaultSlug(t *testing.T) {
	mockConfig := RemoteConfig{
		Username: "testuser",
		Slug:     "default",
		Name:     "Default Config",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/configs/alias/testuser":
			w.WriteHeader(http.StatusNotFound)
		case "/testuser/default/config":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(mockConfig)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	originalClient := remoteHTTPClient
	remoteHTTPClient = server.Client()
	defer func() { remoteHTTPClient = originalClient }()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	result, err := FetchRemoteConfig("testuser", "")
	require.NoError(t, err)
	assert.Equal(t, "testuser", result.Username)
	assert.Equal(t, "default", result.Slug)
}

func TestFetchRemoteConfig_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	originalClient := remoteHTTPClient
	remoteHTTPClient = server.Client()
	defer func() { remoteHTTPClient = originalClient }()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	result, err := FetchRemoteConfig("testuser/myconfig", "")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "parse config")
}

func TestFetchRemoteConfig_NetworkError(t *testing.T) {
	t.Setenv("OPENBOOT_API_URL", "http://localhost:9999")

	result, err := FetchRemoteConfig("testuser/myconfig", "")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "fetch config")
}

func TestFetchRemoteConfig_AliasResolution(t *testing.T) {
	mockConfig := RemoteConfig{
		Username: "testuser",
		Slug:     "mysetup",
		Name:     "My Setup",
		Preset:   "developer",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/configs/alias/testuser":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(mockConfig)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	originalClient := remoteHTTPClient
	remoteHTTPClient = server.Client()
	defer func() { remoteHTTPClient = originalClient }()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	result, err := FetchRemoteConfig("testuser", "")
	require.NoError(t, err)
	assert.Equal(t, "testuser", result.Username)
	assert.Equal(t, "mysetup", result.Slug)
	assert.Equal(t, "developer", result.Preset)
}

func TestFetchRemoteConfig_NoAliasFallsBackToDefault(t *testing.T) {
	mockConfig := RemoteConfig{
		Username: "testuser",
		Slug:     "default",
		Name:     "Default Config",
		Preset:   "minimal",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/configs/alias/testuser":
			w.WriteHeader(http.StatusNotFound)
		case "/testuser/default/config":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(mockConfig)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	originalClient := remoteHTTPClient
	remoteHTTPClient = server.Client()
	defer func() { remoteHTTPClient = originalClient }()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	result, err := FetchRemoteConfig("testuser", "")
	require.NoError(t, err)
	assert.Equal(t, "testuser", result.Username)
	assert.Equal(t, "default", result.Slug)
}

func TestFetchRemoteConfig_NoAliasNoDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/configs/alias/testuser":
			w.WriteHeader(http.StatusNotFound)
		case "/testuser/default/config":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	originalClient := remoteHTTPClient
	remoteHTTPClient = server.Client()
	defer func() { remoteHTTPClient = originalClient }()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	result, err := FetchRemoteConfig("testuser", "")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "config not found: testuser/default")
}

func TestFetchRemoteConfig_ExplicitSlugNoFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/testuser/default/config", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	originalClient := remoteHTTPClient
	remoteHTTPClient = server.Client()
	defer func() { remoteHTTPClient = originalClient }()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	result, err := FetchRemoteConfig("testuser/default", "")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "config not found: testuser/default")
}
