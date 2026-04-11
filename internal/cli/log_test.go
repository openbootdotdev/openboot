package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSyncSource writes a sync_source.json into the temp HOME directory so
// runLog/runRestore can resolve the config slug without a --slug flag.
func writeSyncSource(t *testing.T, tmpDir, slug string) {
	t.Helper()
	dir := filepath.Join(tmpDir, ".openboot")
	require.NoError(t, os.MkdirAll(dir, 0700))
	src := syncpkg.SyncSource{
		Slug:        slug,
		Username:    "testuser",
		InstalledAt: time.Now(),
	}
	data, err := json.MarshalIndent(src, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sync_source.json"), data, 0600))
}

// revisionsResponse builds the JSON body returned by the mock revisions endpoint.
func revisionsResponse(revs []map[string]any) string {
	body, _ := json.Marshal(map[string]any{"revisions": revs})
	return string(body)
}

func TestRunLog_NotAuthenticated(t *testing.T) {
	setupTestAuth(t, false)
	t.Setenv("OPENBOOT_API_URL", "http://localhost:9999")

	err := runLog("my-config")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not logged in")
}

func TestRunLog_NoSlug_NoSyncSource(t *testing.T) {
	setupTestAuth(t, true)
	t.Setenv("OPENBOOT_API_URL", "http://localhost:9999")
	// HOME is already set to a temp dir with no sync_source.json by setupTestAuth.

	err := runLog("") // no flag slug, no sync source

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no config slug")
}

func TestRunLog_SlugFromFlag(t *testing.T) {
	setupTestAuth(t, true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/configs/flag-slug/revisions", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(revisionsResponse(nil)))
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	err := runLog("flag-slug")
	assert.NoError(t, err)
}

func TestRunLog_SlugFromSyncSource(t *testing.T) {
	tmpDir := setupTestAuth(t, true)
	writeSyncSource(t, tmpDir, "source-slug")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/configs/source-slug/revisions", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(revisionsResponse(nil)))
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	err := runLog("") // no flag — should resolve from sync source
	assert.NoError(t, err)
}

func TestRunLog_ShowsRevisions(t *testing.T) {
	setupTestAuth(t, true)

	msg := "before adding rust"
	revs := []map[string]any{
		{
			"id":            "rev_abc1234567890123",
			"message":       msg,
			"created_at":    "2026-01-10T10:00:00Z",
			"package_count": 5,
		},
		{
			"id":            "rev_def4567890123456",
			"message":       nil,
			"created_at":    "2026-01-05T09:00:00Z",
			"package_count": 3,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(revisionsResponse(revs)))
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	err := runLog("my-config")
	assert.NoError(t, err)
}

func TestRunLog_EmptyRevisions(t *testing.T) {
	setupTestAuth(t, true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(revisionsResponse([]map[string]any{})))
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	err := runLog("my-config")
	assert.NoError(t, err)
}

func TestRunLog_ConfigNotFound(t *testing.T) {
	setupTestAuth(t, true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	err := runLog("missing-config")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRunLog_ServerError(t *testing.T) {
	setupTestAuth(t, true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	err := runLog("my-config")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestRunLog_AuthorizationHeader(t *testing.T) {
	setupTestAuth(t, true)

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(revisionsResponse(nil)))
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	err := runLog("my-config")
	require.NoError(t, err)
	assert.Equal(t, "Bearer obt_test_token_123", gotAuth)
}

func TestLogCmd_CommandStructure(t *testing.T) {
	assert.Equal(t, "log", logCmd.Use)
	assert.NotEmpty(t, logCmd.Short)
	assert.NotEmpty(t, logCmd.Long)
	assert.NotEmpty(t, logCmd.Example)
	assert.NotNil(t, logCmd.RunE)

	flag := logCmd.Flags().Lookup("slug")
	assert.NotNil(t, flag, "should have --slug flag")
	assert.Equal(t, "", flag.DefValue)
}
