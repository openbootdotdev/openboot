package auth

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"time"

	"github.com/openbootdotdev/openboot/internal/httputil"
	"github.com/openbootdotdev/openboot/internal/system"
	"github.com/openbootdotdev/openboot/internal/ui"
)

// httpClient uses HTTP/1.1 to avoid HTTP/2 compatibility issues with Cloudflare Workers
var httpClient = &http.Client{
	Transport: &http.Transport{
		TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
	},
	Timeout: 30 * time.Second,
}

const DefaultAPIBase = "https://openboot.dev"

// isAllowedAPIURL delegates to the shared implementation in system package.
var isAllowedAPIURL = system.IsAllowedAPIURL

func GetAPIBase() string {
	if base := os.Getenv("OPENBOOT_API_URL"); base != "" {
		if isAllowedAPIURL(base) {
			return base
		}
		ui.Warn(fmt.Sprintf("Ignoring insecure OPENBOOT_API_URL=%q (only https or http://localhost allowed)", base))
	}
	return DefaultAPIBase
}

type cliStartResponse struct {
	CodeID string `json:"code_id"`
	Code   string `json:"code"`
}

type cliPollResponse struct {
	Status    string `json:"status"`
	Token     string `json:"token,omitempty"`
	Username  string `json:"username,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

func LoginInteractive(ctx context.Context, apiBase string) (*StoredAuth, error) {
	codeID, code, err := startAuthSession(apiBase)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "\n")
	ui.Info(fmt.Sprintf("Your one-time code is: %s", ui.Green(code)))
	fmt.Fprintf(os.Stderr, "\n")
	ui.Info("Opening browser to approve...")

	approvalURL := fmt.Sprintf("%s/cli-auth?code=%s", apiBase, code)
	if err := openBrowser(approvalURL); err != nil {
		ui.Warn(fmt.Sprintf("Could not open browser automatically. Please visit:\n  %s", approvalURL))
	}

	fmt.Fprintf(os.Stderr, "\nWaiting for approval...\n")

	result, err := pollForApproval(ctx, apiBase, codeID)
	if err != nil {
		return nil, err
	}

	expiresAt, err := time.Parse(time.RFC3339, result.ExpiresAt)
	if err != nil {
		// Fallback for SQLite datetime format (YYYY-MM-DD HH:MM:SS)
		// Assume UTC if no timezone info
		t, err2 := time.Parse("2006-01-02 15:04:05", result.ExpiresAt)
		if err2 == nil {
			expiresAt = t
		} else {
			return nil, fmt.Errorf("parse expiration %q: %w (sqlite fallback: %v)", result.ExpiresAt, err, err2)
		}
	}

	stored := &StoredAuth{
		Token:     result.Token,
		Username:  result.Username,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}

	if err := SaveToken(stored); err != nil {
		return nil, fmt.Errorf("save auth token: %w", err)
	}

	ui.Success(fmt.Sprintf("Authenticated as %s", stored.Username))
	return stored, nil
}

func startAuthSession(apiBase string) (codeID, code string, err error) {
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/auth/cli/start", apiBase), http.NoBody)
	if err != nil {
		return "", "", fmt.Errorf("create request: %w", err)
	}

	resp, err := httputil.Do(httpClient, req)
	if err != nil {
		return "", "", fmt.Errorf("start auth session: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort; body already read

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("auth start failed with status %d", resp.StatusCode)
	}

	var result cliStartResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return "", "", fmt.Errorf("parse auth response: %w", err)
	}

	if result.CodeID == "" || result.Code == "" {
		return "", "", fmt.Errorf("server returned incomplete auth session")
	}

	return result.CodeID, result.Code, nil
}

// pollTimeout and pollInterval are package-level defaults, overridable in tests.
var (
	pollTimeout  = 5 * time.Minute
	pollInterval = 2 * time.Second
)

func pollForApproval(ctx context.Context, apiBase, codeID string) (*cliPollResponse, error) {
	pollURL := fmt.Sprintf("%s/api/auth/cli/poll?code_id=%s", apiBase, url.QueryEscape(codeID))
	timeout := time.After(pollTimeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("login cancelled")
		case <-timeout:
			return nil, fmt.Errorf("authentication timed out after 5 minutes")
		case <-ticker.C:
			result, done, err := pollOnce(pollURL)
			if err != nil {
				return nil, err
			}
			if done {
				return result, nil
			}
		}
	}
}

func pollOnce(pollURL string) (*cliPollResponse, bool, error) {
	req, err := http.NewRequest("GET", pollURL, nil)
	if err != nil {
		return nil, false, nil // malformed URL is transient; keep polling
	}
	resp, err := httputil.Do(httpClient, req)
	if err != nil {
		return nil, false, nil // transient network error; keep polling
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort; body already read

	var result cliPollResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return nil, false, nil
	}

	switch result.Status {
	case "approved":
		return &result, true, nil
	case "pending", "processing":
		// Still waiting — keep polling
		return nil, false, nil
	case "used":
		return nil, false, fmt.Errorf("this login code has already been used; please run 'openboot login' again")
	case "expired":
		return nil, false, fmt.Errorf("authorization code expired; please run 'openboot login' again")
	default:
		return nil, false, fmt.Errorf("unexpected auth status %q; please try again or run 'openboot login'", result.Status)
	}
}

var openBrowserFunc = func(url string) error {
	return exec.Command("open", url).Start() //nolint:gosec // "open" is a macOS system binary; url is the oauth redirect URI
}

func openBrowser(url string) error {
	return openBrowserFunc(url)
}
