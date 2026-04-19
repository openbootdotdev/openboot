package search

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Transport mocks — no port binding.
// ---------------------------------------------------------------------------

type searchMockRT struct{ handler http.Handler }

func (m *searchMockRT) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	m.handler.ServeHTTP(rec, req)
	return rec.Result(), nil
}

type searchErrRT struct{ err error }

func (e *searchErrRT) RoundTrip(*http.Request) (*http.Response, error) { return nil, e.err }

// withMockSearchClient patches the package-level httpClient for one test.
func withMockSearchClient(t *testing.T, handler http.Handler) {
	t.Helper()
	orig := httpClient
	httpClient = &http.Client{Transport: &searchMockRT{handler: handler}}
	t.Cleanup(func() { httpClient = orig })
}

func withFixedResponse(t *testing.T, statusCode int, body string) {
	t.Helper()
	withMockSearchClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
	}))
}

// marshalResponse builds a JSON searchResponse body.
func marshalResponse(t *testing.T, results []searchResult) string {
	t.Helper()
	b, err := json.Marshal(searchResponse{Results: results})
	require.NoError(t, err)
	return string(b)
}

// ---------------------------------------------------------------------------
// queryAPI tests
// ---------------------------------------------------------------------------

func TestQueryAPI_200_ValidJSON(t *testing.T) {
	results := []searchResult{
		{Name: "git", Desc: "Distributed version control", Type: "formula"},
		{Name: "iterm2", Desc: "Terminal emulator", Type: "cask"},
		{Name: "typescript", Desc: "TypeScript compiler", Type: "npm"},
	}
	withFixedResponse(t, http.StatusOK, marshalResponse(t, results))

	pkgs, err := queryAPI("homebrew", "git")

	require.NoError(t, err)
	require.Len(t, pkgs, 3)

	assert.Equal(t, "git", pkgs[0].Name)
	assert.Equal(t, "Distributed version control", pkgs[0].Description)
	assert.False(t, pkgs[0].IsCask, "formula should not be a cask")
	assert.False(t, pkgs[0].IsNpm, "formula should not be npm")

	assert.Equal(t, "iterm2", pkgs[1].Name)
	assert.True(t, pkgs[1].IsCask, "type=cask should set IsCask")
	assert.False(t, pkgs[1].IsNpm)

	assert.Equal(t, "typescript", pkgs[2].Name)
	assert.True(t, pkgs[2].IsNpm, "type=npm should set IsNpm")
	assert.False(t, pkgs[2].IsCask)
}

func TestQueryAPI_200_EmptyResults(t *testing.T) {
	withFixedResponse(t, http.StatusOK, marshalResponse(t, []searchResult{}))

	pkgs, err := queryAPI("homebrew", "nonexistent-package-xyz")

	require.NoError(t, err)
	assert.Nil(t, pkgs, "empty results should return nil slice")
}

func TestQueryAPI_429_RateLimited(t *testing.T) {
	withFixedResponse(t, http.StatusTooManyRequests, "rate limited")

	pkgs, err := queryAPI("homebrew", "git")

	require.Error(t, err, "429 should produce a non-nil error")
	assert.Nil(t, pkgs)
	assert.Contains(t, err.Error(), "Rate limited", "error message should indicate rate limiting")
}

func TestQueryAPI_500_ServerError(t *testing.T) {
	withFixedResponse(t, http.StatusInternalServerError, "internal server error")

	pkgs, err := queryAPI("homebrew", "git")

	require.Error(t, err, "500 should produce a non-nil error")
	assert.Nil(t, pkgs)
	assert.Contains(t, err.Error(), "500", "error message should mention 500")
}

func TestQueryAPI_InvalidJSON(t *testing.T) {
	withFixedResponse(t, http.StatusOK, `{not valid json}`)

	pkgs, err := queryAPI("homebrew", "git")

	require.Error(t, err, "invalid JSON should produce a non-nil error")
	assert.Nil(t, pkgs)
}

func TestQueryAPI_NetworkError(t *testing.T) {
	orig := httpClient
	httpClient = &http.Client{Transport: &searchErrRT{err: errors.New("connection refused")}}
	t.Cleanup(func() { httpClient = orig })

	pkgs, err := queryAPI("homebrew", "git")

	require.Error(t, err, "transport error should produce a non-nil error")
	assert.Nil(t, pkgs)
}

// ---------------------------------------------------------------------------
// SearchOnline tests
// ---------------------------------------------------------------------------

func TestSearchOnline_EmptyQuery(t *testing.T) {
	pkgs, err := SearchOnline("")

	require.NoError(t, err)
	assert.Nil(t, pkgs)
}

func TestSearchOnline_CombinesBrewAndNpm(t *testing.T) {
	result := searchResult{Name: "ripgrep", Desc: "Fast grep", Type: "formula"}
	withFixedResponse(t, http.StatusOK, marshalResponse(t, []searchResult{result}))

	pkgs, err := SearchOnline("ripgrep")

	require.NoError(t, err)
	// Two requests (homebrew + npm), each returns one result → 2 total.
	assert.Len(t, pkgs, 2)
}

func TestSearchOnline_BothEndpointsError_ReturnsError(t *testing.T) {
	withFixedResponse(t, http.StatusInternalServerError, "error")

	pkgs, err := SearchOnline("git")

	require.Error(t, err)
	assert.Empty(t, pkgs)
	assert.Contains(t, err.Error(), "500")
}

func TestSearchOnline_PartialSuccess_ReturnsResults(t *testing.T) {
	brewResult := searchResult{Name: "wget", Desc: "HTTP client", Type: "formula"}
	brewBody := marshalResponse(t, []searchResult{brewResult})

	withMockSearchClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "homebrew") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(brewBody))
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))

	pkgs, err := SearchOnline("wget")

	require.NoError(t, err, "partial success should not propagate error when results exist")
	require.Len(t, pkgs, 1)
	assert.Equal(t, "wget", pkgs[0].Name)
}

// ---------------------------------------------------------------------------
// IsCask / IsNpm flag tests
// ---------------------------------------------------------------------------

func TestQueryAPI_CaskAndNpmFlags(t *testing.T) {
	tests := []struct {
		name       string
		resultType string
		wantCask   bool
		wantNpm    bool
	}{
		{"formula", "formula", false, false},
		{"cask", "cask", true, false},
		{"npm", "npm", false, true},
		{"unknown type", "unknown", false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := searchResult{Name: "pkg", Desc: "desc", Type: tc.resultType}
			withFixedResponse(t, http.StatusOK, marshalResponse(t, []searchResult{result}))

			pkgs, err := queryAPI("homebrew", "pkg")
			require.NoError(t, err)
			require.Len(t, pkgs, 1)

			got := pkgs[0]
			assert.Equal(t, tc.wantCask, got.IsCask, "IsCask mismatch for type=%s", tc.resultType)
			assert.Equal(t, tc.wantNpm, got.IsNpm, "IsNpm mismatch for type=%s", tc.resultType)
		})
	}
}

// ---------------------------------------------------------------------------
// getAPIBase env var handling
// ---------------------------------------------------------------------------

func TestGetAPIBase_DefaultURL(t *testing.T) {
	t.Setenv("OPENBOOT_API_URL", "")
	base := getAPIBase()
	assert.Equal(t, "https://openboot.dev/api", base)
}

func TestGetAPIBase_ValidHTTPSOverride(t *testing.T) {
	t.Setenv("OPENBOOT_API_URL", "https://staging.openboot.dev")
	base := getAPIBase()
	assert.Equal(t, "https://staging.openboot.dev/api", base)
}

func TestGetAPIBase_ValidLocalhostOverride(t *testing.T) {
	t.Setenv("OPENBOOT_API_URL", "http://localhost:8080")
	base := getAPIBase()
	assert.Equal(t, "http://localhost:8080/api", base)
}

func TestGetAPIBase_DisallowedURLFallsBack(t *testing.T) {
	// Plain HTTP to a non-loopback host must not be accepted.
	t.Setenv("OPENBOOT_API_URL", "http://evil.example.com")
	base := getAPIBase()
	assert.Equal(t, "https://openboot.dev/api", base)
}

// ---------------------------------------------------------------------------
// Package field mapping
// ---------------------------------------------------------------------------

func TestQueryAPI_PackageFieldMapping(t *testing.T) {
	result := searchResult{Name: "fzf", Desc: "Fuzzy finder", Type: "formula"}
	withFixedResponse(t, http.StatusOK, marshalResponse(t, []searchResult{result}))

	pkgs, err := queryAPI("homebrew", "fzf")
	require.NoError(t, err)
	require.Len(t, pkgs, 1)

	pkg := pkgs[0]
	assert.Equal(t, "fzf", pkg.Name)
	assert.Equal(t, "Fuzzy finder", pkg.Description)
	_ = pkg
}
