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

func TestRevisionPackagesToRemoteConfig(t *testing.T) {
	tests := []struct {
		name     string
		pkgs     []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		}
		username string
		slug     string
		wantRC   *config.RemoteConfig
	}{
		{
			name:     "empty packages",
			pkgs:     nil,
			username: "alice",
			slug:     "my-setup",
			wantRC: &config.RemoteConfig{
				Username: "alice",
				Slug:     "my-setup",
			},
		},
		{
			name: "formula only",
			pkgs: []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			}{
				{Name: "git", Type: "formula"},
				{Name: "go", Type: "formula"},
			},
			username: "bob",
			slug:     "dev",
			wantRC: &config.RemoteConfig{
				Username: "bob",
				Slug:     "dev",
				Packages: config.PackageEntryList{
					{Name: "git"},
					{Name: "go"},
				},
			},
		},
		{
			name: "all package types",
			pkgs: []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			}{
				{Name: "git", Type: "formula"},
				{Name: "docker", Type: "cask"},
				{Name: "typescript", Type: "npm"},
				{Name: "homebrew/cask-fonts", Type: "tap"},
			},
			username: "carol",
			slug:     "full",
			wantRC: &config.RemoteConfig{
				Username: "carol",
				Slug:     "full",
				Packages: config.PackageEntryList{{Name: "git"}},
				Casks:    config.PackageEntryList{{Name: "docker"}},
				Npm:      config.PackageEntryList{{Name: "typescript"}},
				Taps:     []string{"homebrew/cask-fonts"},
			},
		},
		{
			name: "unknown type is silently skipped",
			pkgs: []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			}{
				{Name: "git", Type: "formula"},
				{Name: "unknown-thing", Type: "ruby"},
			},
			username: "dave",
			slug:     "slim",
			wantRC: &config.RemoteConfig{
				Username: "dave",
				Slug:     "slim",
				Packages: config.PackageEntryList{{Name: "git"}},
			},
		},
		{
			name: "multiple taps",
			pkgs: []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			}{
				{Name: "hashicorp/tap", Type: "tap"},
				{Name: "homebrew/core", Type: "tap"},
			},
			username: "eve",
			slug:     "infra",
			wantRC: &config.RemoteConfig{
				Username: "eve",
				Slug:     "infra",
				Taps:     []string{"hashicorp/tap", "homebrew/core"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := revisionPackagesToRemoteConfig(tt.pkgs, tt.username, tt.slug)

			assert.Equal(t, tt.wantRC.Username, rc.Username)
			assert.Equal(t, tt.wantRC.Slug, rc.Slug)

			if len(tt.wantRC.Packages) == 0 {
				assert.Empty(t, rc.Packages)
			} else {
				assert.Equal(t, tt.wantRC.Packages, rc.Packages)
			}

			if len(tt.wantRC.Casks) == 0 {
				assert.Empty(t, rc.Casks)
			} else {
				assert.Equal(t, tt.wantRC.Casks, rc.Casks)
			}

			if len(tt.wantRC.Npm) == 0 {
				assert.Empty(t, rc.Npm)
			} else {
				assert.Equal(t, tt.wantRC.Npm, rc.Npm)
			}

			if len(tt.wantRC.Taps) == 0 {
				assert.Empty(t, rc.Taps)
			} else {
				assert.Equal(t, tt.wantRC.Taps, rc.Taps)
			}
		})
	}
}

// ── runRestore error-path unit tests ─────────────────────────────────────────
// These test the auth, slug-resolution, and HTTP-error branches.
// The success path (ComputeDiff → sync pipeline) is covered by E2E VM tests.

func TestRunRestore_NotAuthenticated(t *testing.T) {
	setupTestAuth(t, false)
	t.Setenv("OPENBOOT_API_URL", "http://localhost:9999")

	err := runRestore("rev_abc123", "my-config", false, true)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not logged in")
}

func TestRunRestore_NoSlug_NoSyncSource(t *testing.T) {
	setupTestAuth(t, true)
	t.Setenv("OPENBOOT_API_URL", "http://localhost:9999")

	err := runRestore("rev_abc123", "", false, true)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no config slug")
}

func TestRunRestore_ActualRestore_NotFound(t *testing.T) {
	setupTestAuth(t, true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"revision not found"}`))
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	err := runRestore("rev_missing", "my-config", false, true)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "revision not found")
}

func TestRunRestore_ActualRestore_ServerError(t *testing.T) {
	setupTestAuth(t, true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`internal error`))
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	err := runRestore("rev_abc123", "my-config", false, true)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestRunRestore_DryRun_RevisionNotFound(t *testing.T) {
	setupTestAuth(t, true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "dry-run must use GET, not POST")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	err := runRestore("rev_missing", "my-config", true, true)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRunRestore_DryRun_ServerError(t *testing.T) {
	setupTestAuth(t, true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	err := runRestore("rev_abc123", "my-config", true, true)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestRunRestore_DryRun_DoesNotCallRestoreEndpoint(t *testing.T) {
	setupTestAuth(t, true)

	postCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			postCalled = true
		}
		// Always return 500 so the test ends early (before ComputeDiff).
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	_ = runRestore("rev_abc123", "my-config", true, true)

	assert.False(t, postCalled, "restore endpoint must NOT be called during dry-run")
}

func TestRunRestore_AuthorizationHeader(t *testing.T) {
	setupTestAuth(t, true)

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNotFound) // stop early, before ComputeDiff
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	_ = runRestore("rev_abc123", "my-config", false, true)

	assert.Equal(t, "Bearer obt_test_token_123", gotAuth)
}

func TestRunRestore_SlugFromSyncSource(t *testing.T) {
	tmpDir := setupTestAuth(t, true)
	writeSyncSource(t, tmpDir, "source-slug")

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	_ = runRestore("rev_abc123", "", false, true)

	assert.Contains(t, gotPath, "source-slug")
}

func TestRunRestore_RestoreEndpointPayload(t *testing.T) {
	setupTestAuth(t, true)

	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			gotBody = make([]byte, r.ContentLength)
			r.Body.Read(gotBody)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	_ = runRestore("rev_abc123", "my-config", false, true)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(gotBody, &payload))
	// Body should be valid JSON (the empty object sent as the POST body)
	assert.NotNil(t, payload)
}

func TestRestoreCmd_CommandStructure(t *testing.T) {
	assert.Equal(t, "restore <revision-id>", restoreCmd.Use)
	assert.NotEmpty(t, restoreCmd.Short)
	assert.NotEmpty(t, restoreCmd.Long)
	assert.NotEmpty(t, restoreCmd.Example)
	assert.NotNil(t, restoreCmd.RunE)

	assert.NotNil(t, restoreCmd.Flags().Lookup("slug"))
	assert.NotNil(t, restoreCmd.Flags().Lookup("dry-run"))

	yesFlag := restoreCmd.Flags().Lookup("yes")
	assert.NotNil(t, yesFlag)
	assert.Equal(t, "y", yesFlag.Shorthand)
}

func TestRevisionPackagesToRemoteConfig_PreservesOrder(t *testing.T) {
	pkgs := []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}{
		{Name: "zzz", Type: "formula"},
		{Name: "aaa", Type: "formula"},
		{Name: "mmm", Type: "formula"},
	}

	rc := revisionPackagesToRemoteConfig(pkgs, "user", "slug")

	assert.Equal(t, "zzz", rc.Packages[0].Name)
	assert.Equal(t, "aaa", rc.Packages[1].Name)
	assert.Equal(t, "mmm", rc.Packages[2].Name)
}
