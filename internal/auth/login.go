package auth

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

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

// GetAPIBase returns the API base URL, checking the OPENBOOT_API_URL
// environment variable first and falling back to DefaultAPIBase.
func GetAPIBase() string {
	if base := os.Getenv("OPENBOOT_API_URL"); base != "" {
		return base
	}
	return DefaultAPIBase
}

type cliStartRequest struct {
	Code string `json:"code"`
}

type cliStartResponse struct {
	CodeID string `json:"code_id"`
}

type cliPollResponse struct {
	Status    string `json:"status"`
	Token     string `json:"token,omitempty"`
	Username  string `json:"username,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// LoginInteractive performs the full CLI-to-browser authentication flow:
// generates a code, starts the auth session, opens the browser for approval,
// and polls until approved or timed out.
func LoginInteractive(apiBase string) (*StoredAuth, error) {
	code := GenerateCode()

	codeID, err := startAuthSession(apiBase, code)
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

	result, err := pollForApproval(apiBase, codeID)
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
			return nil, fmt.Errorf("failed to parse expiration time '%s': %w (also tried SQLite format: %v)", result.ExpiresAt, err, err2)
		}
	}

	stored := &StoredAuth{
		Token:     result.Token,
		Username:  result.Username,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}

	if err := SaveToken(stored); err != nil {
		return nil, fmt.Errorf("failed to save auth token: %w", err)
	}

	ui.Success(fmt.Sprintf("Authenticated as %s", stored.Username))
	return stored, nil
}

func startAuthSession(apiBase, code string) (string, error) {
	body, err := json.Marshal(cliStartRequest{Code: code})
	if err != nil {
		return "", fmt.Errorf("failed to marshal start request: %w", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/auth/cli/start", apiBase), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create start request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to start auth session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth start failed with status %d", resp.StatusCode)
	}

	var result cliStartResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse auth start response: %w", err)
	}

	return result.CodeID, nil
}

func pollForApproval(apiBase, codeID string) (*cliPollResponse, error) {
	pollURL := fmt.Sprintf("%s/api/auth/cli/poll?code_id=%s", apiBase, codeID)
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
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
	resp, err := httpClient.Get(pollURL)
	if err != nil {
		return nil, false, nil
	}
	defer resp.Body.Close()

	var result cliPollResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, false, nil
	}

	if result.Status == "approved" {
		return &result, true, nil
	}
	if result.Status == "expired" {
		return nil, false, fmt.Errorf("authorization code expired or already used")
	}
	return nil, false, nil
}

func openBrowser(url string) error {
	return exec.Command("open", url).Start()
}
