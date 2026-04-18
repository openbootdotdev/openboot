package push

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteConfigToAPIPackages(t *testing.T) {
	tests := []struct {
		name     string
		rc       *config.RemoteConfig
		expected []APIPackage
	}{
		{
			name:     "empty config",
			rc:       &config.RemoteConfig{},
			expected: []APIPackage{},
		},
		{
			name: "formulae only",
			rc: &config.RemoteConfig{
				Packages: config.PackageEntryList{{Name: "git"}, {Name: "go"}},
			},
			expected: []APIPackage{
				{Name: "git", Type: "formula"},
				{Name: "go", Type: "formula"},
			},
		},
		{
			name: "all types including taps",
			rc: &config.RemoteConfig{
				Packages: config.PackageEntryList{{Name: "git"}},
				Casks:    config.PackageEntryList{{Name: "docker"}},
				Npm:      config.PackageEntryList{{Name: "typescript"}},
				Taps:     []string{"homebrew/cask-fonts", "hashicorp/tap"},
			},
			expected: []APIPackage{
				{Name: "git", Type: "formula"},
				{Name: "docker", Type: "cask"},
				{Name: "typescript", Type: "npm"},
				{Name: "homebrew/cask-fonts", Type: "tap"},
				{Name: "hashicorp/tap", Type: "tap"},
			},
		},
		{
			name: "desc preserved",
			rc: &config.RemoteConfig{
				Packages: config.PackageEntryList{{Name: "git", Desc: "Version control"}},
			},
			expected: []APIPackage{
				{Name: "git", Type: "formula", Desc: "Version control"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RemoteConfigToAPIPackages(tt.rc)
			if len(tt.expected) == 0 {
				assert.Empty(t, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestRemoteConfigToAPIPackages_DoesNotMutateInput(t *testing.T) {
	rc := &config.RemoteConfig{
		Packages: config.PackageEntryList{{Name: "git"}},
		Taps:     []string{"homebrew/core"},
	}
	originalPackages := make(config.PackageEntryList, len(rc.Packages))
	copy(originalPackages, rc.Packages)
	originalTaps := make([]string, len(rc.Taps))
	copy(originalTaps, rc.Taps)

	_ = RemoteConfigToAPIPackages(rc)

	assert.Equal(t, originalPackages, rc.Packages)
	assert.Equal(t, originalTaps, rc.Taps)
}

func TestFetchUserConfigs_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/configs", r.URL.Path)
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"configs": []map[string]any{
				{"slug": "a", "name": "A"},
				{"slug": "b", "name": "B"},
			},
		})
	}))
	defer server.Close()

	out, err := FetchUserConfigs(context.Background(), "tok", server.URL)
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.Equal(t, "a", out[0].Slug)
	assert.Equal(t, "B", out[1].Name)
}

func TestFetchUserConfigs_NonOKReturnsNilNoError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	out, err := FetchUserConfigs(context.Background(), "tok", server.URL)
	assert.NoError(t, err)
	assert.Nil(t, out)
}

func TestUploadSnapshot_CreateSendsPOSTWithName(t *testing.T) {
	var gotMethod string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"slug": "new-slug"})
	}))
	defer server.Close()

	result, err := UploadSnapshot(context.Background(), SnapshotOptions{
		Snapshot:   &snapshot.Snapshot{},
		Name:       "My Setup",
		Desc:       "desc",
		Visibility: "public",
		Token:      "t",
		APIBase:    server.URL,
	})
	require.NoError(t, err)
	assert.Equal(t, "new-slug", result.Slug)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "My Setup", gotBody["name"])
	assert.NotContains(t, gotBody, "config_slug")
}

func TestUploadSnapshot_UpdateSendsPUTWithSlug(t *testing.T) {
	var gotMethod string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"slug": "existing"})
	}))
	defer server.Close()

	result, err := UploadSnapshot(context.Background(), SnapshotOptions{
		Snapshot: &snapshot.Snapshot{},
		Slug:     "existing",
		Message:  "update notes",
		Token:    "t",
		APIBase:  server.URL,
	})
	require.NoError(t, err)
	assert.Equal(t, "existing", result.Slug)
	assert.Equal(t, http.MethodPut, gotMethod)
	assert.Equal(t, "existing", gotBody["config_slug"])
	assert.Equal(t, "update notes", gotBody["message"])
}

func TestUploadSnapshot_409MaximumReachedMappedToFriendlyError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "You have reached the maximum"})
	}))
	defer server.Close()

	_, err := UploadSnapshot(context.Background(), SnapshotOptions{
		Snapshot: &snapshot.Snapshot{},
		Token:    "t",
		APIBase:  server.URL,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config limit reached")
}

func TestUploadSnapshot_NonOKReturnsStatusAndBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "bad input")
	}))
	defer server.Close()

	_, err := UploadSnapshot(context.Background(), SnapshotOptions{
		Snapshot: &snapshot.Snapshot{},
		Token:    "t",
		APIBase:  server.URL,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
	assert.Contains(t, err.Error(), "bad input")
}

func TestUploadConfig_CreateIncludesPackages(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.HasSuffix(r.URL.Path, "/api/configs"))
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"slug": "cfg"})
	}))
	defer server.Close()

	rc := &config.RemoteConfig{
		Packages: config.PackageEntryList{{Name: "git"}},
		Taps:     []string{"hashicorp/tap"},
	}

	result, err := UploadConfig(context.Background(), ConfigOptions{
		RemoteConfig: rc,
		Name:         "x",
		Desc:         "y",
		Visibility:   "public",
		Token:        "t",
		APIBase:      server.URL,
	})
	require.NoError(t, err)
	assert.Equal(t, "cfg", result.Slug)

	// Packages field should be an array of {name,type}.
	packages, ok := gotBody["packages"].([]any)
	require.True(t, ok, "packages should be an array")
	require.Len(t, packages, 2)
	// No top-level "taps" field (they go under packages with type=tap).
	assert.NotContains(t, gotBody, "taps")
}

func TestUploadConfig_UpdateHitsSlugPath(t *testing.T) {
	var gotPath, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		_ = json.NewEncoder(w).Encode(map[string]any{"slug": "my-cfg"})
	}))
	defer server.Close()

	_, err := UploadConfig(context.Background(), ConfigOptions{
		RemoteConfig: &config.RemoteConfig{},
		Slug:         "my-cfg",
		Token:        "t",
		APIBase:      server.URL,
	})
	require.NoError(t, err)
	assert.Equal(t, "/api/configs/my-cfg", gotPath)
	assert.Equal(t, http.MethodPut, gotMethod)
}
