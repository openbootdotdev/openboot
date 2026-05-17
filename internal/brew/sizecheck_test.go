package brew

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFetchCaskSizesReturnsBytesPerCask(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/docker.dmg":
			w.Header().Set("Content-Length", "450000000")
		case "/rectangle.dmg":
			w.Header().Set("Content-Length", "12000000")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	withFakeBrew(t, func(args []string) ([]byte, error) {
		// Real brew rejects bare --json with --cask (it prints help text);
		// the correct invocation is --json=v2 with a nested {"casks":[...]}
		// envelope. Mirror that exactly here so tests reflect reality.
		if len(args) >= 3 && args[0] == "info" && args[1] == "--json=v2" && args[2] == "--cask" {
			return []byte(fmt.Sprintf(
				`{"formulae":[],"casks":[{"token":"docker","url":"%s/docker.dmg"},{"token":"rectangle","url":"%s/rectangle.dmg"}]}`,
				srv.URL, srv.URL,
			)), nil
		}
		return nil, fmt.Errorf("unexpected args: %v", args)
	})

	sizes := FetchCaskSizes(context.Background(), []string{"docker", "rectangle"})
	assert.Equal(t, int64(450000000), sizes["docker"])
	assert.Equal(t, int64(12000000), sizes["rectangle"])
}

func TestFetchCaskSizesSkipsHeadFailures(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		return []byte(`{"formulae":[],"casks":[{"token":"broken","url":"http://127.0.0.1:1/nope"}]}`), nil
	})

	sizes := FetchCaskSizes(context.Background(), []string{"broken"})
	// Failed HEAD leaves entry at 0 (caller treats as "unknown size").
	assert.Equal(t, int64(0), sizes["broken"])
}

func TestFetchFormulaSizesReturnsBytesPerFormula(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GHCR HEAD requires the anonymous Bearer token; refuse otherwise so
		// the test pins both the URL plumbing and the auth header.
		if r.Header.Get("Authorization") != "Bearer QQ==" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/jq.tar.gz":
			w.Header().Set("Content-Length", "424157")
		case "/ripgrep.tar.gz":
			w.Header().Set("Content-Length", "2300000")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	withFakeBrew(t, func(args []string) ([]byte, error) {
		// Formula info uses bare --json (v1, top-level array).
		if len(args) >= 2 && args[0] == "info" && args[1] == "--json" {
			return []byte(fmt.Sprintf(
				`[{"name":"jq","bottle":{"stable":{"files":{"arm64_sequoia":{"url":"%s/jq.tar.gz"}}}}},`+
					`{"name":"ripgrep","bottle":{"stable":{"files":{"arm64_sequoia":{"url":"%s/ripgrep.tar.gz"}}}}}]`,
				srv.URL, srv.URL,
			)), nil
		}
		return nil, fmt.Errorf("unexpected args: %v", args)
	})

	sizes := FetchFormulaSizes(context.Background(), []string{"jq", "ripgrep"})
	assert.Equal(t, int64(424157), sizes["jq"])
	assert.Equal(t, int64(2300000), sizes["ripgrep"])
}

func TestFetchFormulaSizesSkipsMissingBottle(t *testing.T) {
	withFakeBrew(t, func(args []string) ([]byte, error) {
		// No bottle.stable.files — formula has no bottle (e.g. head-only).
		return []byte(`[{"name":"nobottle","bottle":{"stable":{"files":{}}}}]`), nil
	})

	sizes := FetchFormulaSizes(context.Background(), []string{"nobottle"})
	assert.Equal(t, int64(0), sizes["nobottle"])
}
