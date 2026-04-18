// Package push implements the transport layer for uploading configs and
// snapshots to openboot.dev. The CLI layer is responsible for gathering
// credentials and interactive input; this package only speaks HTTP/JSON.
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/httputil"
	"github.com/openbootdotdev/openboot/internal/snapshot"
)

const (
	defaultTimeout = 30 * time.Second
	listTimeout    = 10 * time.Second
)

// APIPackage is the wire format the openboot.dev API expects for each
// package entry in a config upload.
type APIPackage struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Desc string `json:"desc,omitempty"`
}

// RemoteConfigSummary is a trimmed-down view of a user's existing configs,
// used to populate the "Push to which config?" picker.
type RemoteConfigSummary struct {
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	Visibility string `json:"visibility"`
}

// UploadResult captures just enough to render success output.
type UploadResult struct {
	Slug string
}

// SnapshotOptions describe a single snapshot upload.
type SnapshotOptions struct {
	Snapshot   *snapshot.Snapshot
	Slug       string // empty = create, non-empty = update existing
	Message    string // optional revision message for updates
	Name       string // required when Slug is empty
	Desc       string
	Visibility string
	Token      string
	APIBase    string
}

// ConfigOptions describe a single config upload.
type ConfigOptions struct {
	RemoteConfig *config.RemoteConfig
	Slug         string
	Name         string
	Desc         string
	Visibility   string
	Token        string
	APIBase      string
}

// UploadSnapshot POSTs (or PUTs) a snapshot to /api/configs/from-snapshot.
func UploadSnapshot(ctx context.Context, opts SnapshotOptions) (*UploadResult, error) {
	if opts.Snapshot == nil {
		return nil, errors.New("push: snapshot is required")
	}
	reqBody := map[string]interface{}{
		"snapshot":   opts.Snapshot,
		"visibility": opts.Visibility,
	}
	if opts.Name != "" {
		reqBody["name"] = opts.Name
	}
	if opts.Desc != "" {
		reqBody["description"] = opts.Desc
	}
	if opts.Slug != "" {
		reqBody["config_slug"] = opts.Slug
	}
	if opts.Message != "" {
		reqBody["message"] = opts.Message
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	uploadURL := fmt.Sprintf("%s/api/configs/from-snapshot", opts.APIBase)
	return doUpload(ctx, uploadURL, bodyBytes, opts.Token, opts.Slug)
}

// UploadConfig POSTs (create) or PUTs (update) a RemoteConfig to /api/configs.
func UploadConfig(ctx context.Context, opts ConfigOptions) (*UploadResult, error) {
	if opts.RemoteConfig == nil {
		return nil, errors.New("push: remote config is required")
	}

	reqBody := map[string]interface{}{
		"name":        opts.Name,
		"description": opts.Desc,
		"packages":    RemoteConfigToAPIPackages(opts.RemoteConfig),
		"visibility":  opts.Visibility,
	}
	if opts.RemoteConfig.DotfilesRepo != "" {
		reqBody["dotfiles_repo"] = opts.RemoteConfig.DotfilesRepo
	}
	if len(opts.RemoteConfig.PostInstall) > 0 {
		reqBody["custom_script"] = strings.Join(opts.RemoteConfig.PostInstall, "\n")
	}
	if opts.RemoteConfig.Preset != "" {
		reqBody["base_preset"] = opts.RemoteConfig.Preset
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	var uploadURL string
	if opts.Slug != "" {
		uploadURL = fmt.Sprintf("%s/api/configs/%s", opts.APIBase, url.PathEscape(opts.Slug))
	} else {
		uploadURL = fmt.Sprintf("%s/api/configs", opts.APIBase)
	}
	return doUpload(ctx, uploadURL, bodyBytes, opts.Token, opts.Slug)
}

// RemoteConfigToAPIPackages flattens a RemoteConfig's package lists into the
// API-compatible [{name, type}] array. Exposed for testability.
func RemoteConfigToAPIPackages(rc *config.RemoteConfig) []APIPackage {
	totalCap := len(rc.Packages) + len(rc.Casks) + len(rc.Npm) + len(rc.Taps)
	pkgs := make([]APIPackage, 0, totalCap)
	appendEntries := func(entries config.PackageEntryList, typeName string) {
		for _, e := range entries {
			pkgs = append(pkgs, APIPackage{Name: e.Name, Type: typeName, Desc: e.Desc})
		}
	}
	appendEntries(rc.Packages, "formula")
	appendEntries(rc.Casks, "cask")
	appendEntries(rc.Npm, "npm")
	for _, t := range rc.Taps {
		pkgs = append(pkgs, APIPackage{Name: t, Type: "tap"})
	}
	return pkgs
}

// FetchUserConfigs calls GET /api/configs and returns the user's existing configs.
// A non-2xx response returns (nil, nil) so callers can fall through to
// the create-new flow without surfacing a transport error.
func FetchUserConfigs(ctx context.Context, token, apiBase string) ([]RemoteConfigSummary, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/api/configs", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: listTimeout}
	resp, err := httputil.Do(client, req)
	if err != nil {
		return nil, fmt.Errorf("fetch configs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil // non-fatal — fall through to create-new flow
	}

	var result struct {
		Configs []RemoteConfigSummary `json:"configs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil
	}
	return result.Configs, nil
}

// doUpload executes POST (create) or PUT (update) and returns the resulting slug.
// 409 responses with "maximum" in the message are translated to a friendly
// config-limit error.
func doUpload(ctx context.Context, uploadURL string, body []byte, token, slug string) (*UploadResult, error) {
	method := http.MethodPost
	if slug != "" {
		method = http.MethodPut
	}

	req, err := http.NewRequestWithContext(ctx, method, uploadURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: defaultTimeout}
	resp, err := httputil.Do(client, req)
	if err != nil {
		return nil, fmt.Errorf("upload: %w", err)
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
		return nil, fmt.Errorf("parse response: %w", err)
	}

	resultSlug := result.Slug
	if resultSlug == "" {
		resultSlug = slug
	}
	return &UploadResult{Slug: resultSlug}, nil
}
