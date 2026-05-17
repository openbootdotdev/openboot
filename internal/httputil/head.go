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

// HeadWithBearer is Head with an Authorization: Bearer <token> header. Used
// for Homebrew bottle blobs hosted on ghcr.io, which return 401 to anonymous
// HEAD requests but accept the well-known anonymous token "QQ==" (base64
// "A") for public reads — the same trick brew uses internally.
func HeadWithBearer(ctx context.Context, client *http.Client, url, token string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build HEAD request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return Do(client, req)
}
