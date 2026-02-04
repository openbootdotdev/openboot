package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/openbootdotdev/openboot/internal/ui"
)

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
		return nil, fmt.Errorf("failed to parse expiration time: %w", err)
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

	resp, err := http.Post(
		fmt.Sprintf("%s/api/auth/cli/start", apiBase),
		"application/json",
		bytes.NewReader(body),
	)
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
			resp, err := http.Get(pollURL)
			if err != nil {
				continue
			}

			var result cliPollResponse
			decodeErr := json.NewDecoder(resp.Body).Decode(&result)
			resp.Body.Close()
			if decodeErr != nil {
				continue
			}

			if result.Status == "approved" {
				return &result, nil
			}
		}
	}
}

func openBrowser(url string) error {
	return exec.Command("open", url).Start()
}
