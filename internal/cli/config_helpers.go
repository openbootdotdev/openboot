package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/openbootdotdev/openboot/internal/httputil"
	"github.com/openbootdotdev/openboot/internal/ui"
)

type remoteConfigSummary struct {
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	Visibility string `json:"visibility"`
}

// fetchUserConfigs calls GET /api/configs and returns the user's existing configs.
func fetchUserConfigs(token, apiBase string) ([]remoteConfigSummary, error) {
	req, err := http.NewRequest(http.MethodGet, apiBase+"/api/configs", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := httputil.Do(client, req)
	if err != nil {
		return nil, fmt.Errorf("fetch configs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var result struct {
		Configs []remoteConfigSummary `json:"configs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil
	}
	return result.Configs, nil
}

func promptPushDetails(defaultName string) (string, string, string, error) {
	var (
		name string
		err  error
	)

	fmt.Fprintln(os.Stderr)
	if defaultName != "" {
		name, err = ui.InputWithDefault("Config name", "My Mac Setup", defaultName)
	} else {
		name, err = ui.Input("Config name", "My Mac Setup")
	}
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", "", "", huh.ErrUserAborted
		}
		return "", "", "", fmt.Errorf("get config name: %w", err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "My Mac Setup"
	}

	fmt.Fprintln(os.Stderr)
	desc, err := ui.Input("Description (optional)", "")
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", "", "", huh.ErrUserAborted
		}
		return "", "", "", fmt.Errorf("get description: %w", err)
	}
	desc = strings.TrimSpace(desc)

	fmt.Fprintln(os.Stderr)
	options := []string{
		"Public - Anyone can discover and use this config",
		"Unlisted - Only people with the link can access",
		"Private - Only you can see this config",
	}
	choice, err := ui.SelectOption("Who can see this config?", options)
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", "", "", huh.ErrUserAborted
		}
		return "", "", "", fmt.Errorf("select visibility: %w", err)
	}

	visibility := "unlisted"
	switch {
	case strings.HasPrefix(choice, "Public"):
		visibility = "public"
	case strings.HasPrefix(choice, "Private"):
		visibility = "private"
	}

	return name, desc, visibility, nil
}
