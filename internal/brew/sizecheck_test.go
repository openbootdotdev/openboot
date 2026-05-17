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
		// Expect: brew info --json --cask docker rectangle
		if len(args) >= 3 && args[0] == "info" && args[1] == "--json" && args[2] == "--cask" {
			return []byte(fmt.Sprintf(
				`[{"token":"docker","url":"%s/docker.dmg"},{"token":"rectangle","url":"%s/rectangle.dmg"}]`,
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
		return []byte(`[{"token":"broken","url":"http://127.0.0.1:1/nope"}]`), nil
	})

	sizes := FetchCaskSizes(context.Background(), []string{"broken"})
	// Failed HEAD leaves entry at 0 (caller treats as "unknown size").
	assert.Equal(t, int64(0), sizes["broken"])
}
