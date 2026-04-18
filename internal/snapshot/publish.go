package snapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/openbootdotdev/openboot/internal/httputil"
)

// PublishOptions describe a single upload of a snapshot to openboot.dev.
// Business logic only — the CLI layer is responsible for gathering these
// values (auth, prompts, TTY handling) before calling Publish.
type PublishOptions struct {
	Snapshot    *Snapshot
	Name        string
	Description string
	Visibility  string
	Token       string
	APIBase     string
	// Slug, when non-empty, turns the call into an update (PUT) of that config.
	// When empty, a new config is created (POST).
	Slug string
}

// PublishResult is what the caller needs to render a success message.
type PublishResult struct {
	Slug    string
	Updated bool // true when an existing config was updated
}

// defaultPublishTimeout is the HTTP timeout for /api/configs/from-snapshot.
const defaultPublishTimeout = 30 * time.Second

// Publish uploads a snapshot to openboot.dev.
//
// Behavior is identical to the previous inline CLI implementation:
//   - POST when Slug is empty, PUT with config_slug body field otherwise.
//   - 409 responses containing "maximum" are translated to a friendly
//     "config limit reached" error; other 409s pass the server message through.
//   - Non-2xx responses include the status code and up to 64KB of the body.
func Publish(ctx context.Context, opts PublishOptions) (*PublishResult, error) {
	if opts.Snapshot == nil {
		return nil, errors.New("publish: snapshot is required")
	}
	if opts.Token == "" {
		return nil, errors.New("publish: auth token is required")
	}
	if opts.APIBase == "" {
		return nil, errors.New("publish: api base is required")
	}

	method := http.MethodPost
	reqBody := map[string]interface{}{
		"name":        opts.Name,
		"description": opts.Description,
		"snapshot":    opts.Snapshot,
		"visibility":  opts.Visibility,
	}
	if opts.Slug != "" {
		method = http.MethodPut
		reqBody["config_slug"] = opts.Slug
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	uploadURL := fmt.Sprintf("%s/api/configs/from-snapshot", opts.APIBase)
	req, err := http.NewRequestWithContext(ctx, method, uploadURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+opts.Token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: defaultPublishTimeout}
	resp, err := httputil.Do(client, req)
	if err != nil {
		return nil, fmt.Errorf("upload snapshot: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if readErr != nil {
			return nil, fmt.Errorf("upload failed (status %d): read response: %w", resp.StatusCode, readErr)
		}
		if resp.StatusCode == http.StatusConflict {
			var errResp struct {
				Message string `json:"message"`
				Error   string `json:"error"`
			}
			if jsonErr := json.Unmarshal(respBody, &errResp); jsonErr == nil {
				msg := errResp.Message
				if msg == "" {
					msg = errResp.Error
				}
				if msg != "" && strings.Contains(strings.ToLower(msg), "maximum") {
					return nil, errors.New("config limit reached (max 20): delete an existing config with 'openboot delete <slug>' first")
				}
				if msg != "" {
					return nil, errors.New(msg)
				}
			}
			return nil, fmt.Errorf("conflict: %s", string(respBody))
		}
		return nil, fmt.Errorf("upload failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse upload response: %w", err)
	}
	resultSlug := result.Slug
	if resultSlug == "" {
		resultSlug = opts.Slug
	}
	return &PublishResult{
		Slug:    resultSlug,
		Updated: opts.Slug != "",
	}, nil
}

// SlugToTitle converts a URL slug like "my-mac-setup" to "My Mac Setup".
// Exposed here because it is a pure transform on snapshot slugs and has no
// CLI-specific state.
func SlugToTitle(slug string) string {
	words := strings.Split(slug, "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
