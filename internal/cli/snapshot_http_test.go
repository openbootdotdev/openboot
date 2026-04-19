package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/snapshot"
)

// ---------------------------------------------------------------------------
// downloadSnapshotBytes — real TLS path and size-cap invariant.
//
// output_test.go covers the mock-transport paths (success, 404, bad URL).
// These tests add: a real httptest.TLSServer (exercising the full TLS stack
// through ts.Client()) and the 10 MiB LimitReader cap.
// ---------------------------------------------------------------------------

func TestDownloadSnapshotBytes_TLSServer_Success(t *testing.T) {
	want := `{"version":1,"packages":{"formulae":["git"],"casks":[],"taps":[],"npm":[]}}`
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(want)) //nolint:errcheck // test helper
	}))
	defer ts.Close()

	got, err := downloadSnapshotBytes(ts.URL, ts.Client())
	require.NoError(t, err)
	assert.JSONEq(t, want, string(got))
}

func TestDownloadSnapshotBytes_TLSServer_NotFound(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer ts.Close()

	_, err := downloadSnapshotBytes(ts.URL, ts.Client())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestDownloadSnapshotBytes_SizeCappedAt10MiB(t *testing.T) {
	// LimitReader silently truncates oversized responses so a rogue server
	// can't trigger an OOM. The caller gets at most 10 MiB.
	bigPayload := make([]byte, 11<<20) // 11 MiB
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bigPayload) //nolint:errcheck // test helper
	}))
	defer ts.Close()

	got, err := downloadSnapshotBytes(ts.URL, ts.Client())
	require.NoError(t, err)
	assert.Equal(t, 10<<20, len(got), "response must be capped at 10 MiB")
}

// ---------------------------------------------------------------------------
// postSnapshotToAPI — no prior test coverage existed for this function.
//
// Uses httptest.NewServer (plain HTTP) because postSnapshotToAPI constructs
// its own http.Client with the default (non-TLS) transport, which happily
// connects to http:// test servers.
// ---------------------------------------------------------------------------

func TestPostSnapshotToAPI_NewConfig_POSTReturnsSlug(t *testing.T) {
	withNoSnapshotBrowser(t)

	var gotMethod, gotAuth, gotContentType string
	var gotBody map[string]interface{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody) //nolint:errcheck // test helper
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"slug": "my-new-config"}) //nolint:errcheck // test helper
	}))
	defer ts.Close()

	snap := &snapshot.Snapshot{}
	resultSlug, err := postSnapshotToAPI(snap, "My Setup", "desc", "public", "obt_token", ts.URL, "")
	require.NoError(t, err)
	assert.Equal(t, "my-new-config", resultSlug)
	assert.Equal(t, http.MethodPost, gotMethod, "new config must use POST")
	assert.Equal(t, "Bearer obt_token", gotAuth, "must include Bearer token")
	assert.Equal(t, "application/json", gotContentType)
	assert.Nil(t, gotBody["config_slug"], "POST body must not include config_slug")
	assert.Equal(t, "My Setup", gotBody["name"])
	assert.Equal(t, "public", gotBody["visibility"])
}

func TestPostSnapshotToAPI_UpdateConfig_PUTSendsSlug(t *testing.T) {
	withNoSnapshotBrowser(t)

	var gotMethod string
	var gotBody map[string]interface{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody) //nolint:errcheck // test helper
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"slug": "existing-config"}) //nolint:errcheck // test helper
	}))
	defer ts.Close()

	snap := &snapshot.Snapshot{}
	resultSlug, err := postSnapshotToAPI(snap, "", "", "", "obt_token", ts.URL, "existing-config")
	require.NoError(t, err)
	assert.Equal(t, "existing-config", resultSlug)
	assert.Equal(t, http.MethodPut, gotMethod, "update must use PUT")
	assert.Equal(t, "existing-config", gotBody["config_slug"])
}

func TestPostSnapshotToAPI_ConflictError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"message": "slug already exists"}) //nolint:errcheck // test helper
	}))
	defer ts.Close()

	snap := &snapshot.Snapshot{}
	_, err := postSnapshotToAPI(snap, "name", "desc", "public", "tok", ts.URL, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "slug already exists")
}

func TestPostSnapshotToAPI_ConflictMaxConfigs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"message": "maximum number of configs reached"}) //nolint:errcheck // test helper
	}))
	defer ts.Close()

	snap := &snapshot.Snapshot{}
	_, err := postSnapshotToAPI(snap, "n", "d", "private", "tok", ts.URL, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max 20")
}

func TestPostSnapshotToAPI_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server meltdown")) //nolint:errcheck // test helper
	}))
	defer ts.Close()

	snap := &snapshot.Snapshot{}
	_, err := postSnapshotToAPI(snap, "n", "d", "public", "tok", ts.URL, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestPostSnapshotToAPI_SlugFallbackWhenResponseEmpty(t *testing.T) {
	// When the server returns 200 but omits the slug field, fall back to the
	// slug we passed in (subsequent update of an existing config).
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{}) //nolint:errcheck // test helper
	}))
	defer ts.Close()

	snap := &snapshot.Snapshot{}
	resultSlug, err := postSnapshotToAPI(snap, "", "", "", "tok", ts.URL, "fallback-slug")
	require.NoError(t, err)
	assert.Equal(t, "fallback-slug", resultSlug, "must fall back to the passed-in slug")
}
