package httputil

import (
	"context"
	"fmt"
	"net/http"
)

// Head issues a HEAD request and routes it through Do so 429 + Retry-After
// are handled uniformly. The caller must close the response body.
func Head(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build HEAD request: %w", err)
	}
	return Do(client, req)
}
