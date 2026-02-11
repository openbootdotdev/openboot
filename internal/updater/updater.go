package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/openbootdotdev/openboot/internal/ui"
)

const checkInterval = 24 * time.Hour

var (
	httpClient     *http.Client
	httpClientOnce sync.Once
)

type Release struct {
	TagName string `json:"tag_name"`
}

type CheckState struct {
	LastCheck       time.Time `json:"last_check"`
	LatestVersion   string    `json:"latest_version"`
	UpdateAvailable bool      `json:"update_available"`
}

type AutoUpdateMode string

const (
	AutoUpdateEnabled  AutoUpdateMode = "true"
	AutoUpdateNotify   AutoUpdateMode = "notify"
	AutoUpdateDisabled AutoUpdateMode = "false"
)

type UserConfig struct {
	AutoUpdate AutoUpdateMode `json:"autoupdate"`
}

func loadUserConfig() UserConfig {
	cfg := UserConfig{AutoUpdate: AutoUpdateEnabled}
	path, err := getUserConfigPath()
	if err != nil {
		return cfg
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	json.Unmarshal(data, &cfg)
	if cfg.AutoUpdate == "" {
		cfg.AutoUpdate = AutoUpdateEnabled
	}
	return cfg
}

func getUserConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".openboot", "config.json"), nil
}

func AutoUpgrade(currentVersion string) {
	if os.Getenv("OPENBOOT_DISABLE_AUTOUPDATE") == "1" {
		return
	}

	cfg := loadUserConfig()

	switch cfg.AutoUpdate {
	case AutoUpdateDisabled:
		return
	case AutoUpdateNotify:
		notifyIfUpdateAvailable(currentVersion)
		checkForUpdatesAsync(currentVersion)
		return
	default:
		latest, err := getLatestVersion()
		if err != nil {
			return
		}
		if !isNewerVersion(latest, currentVersion) {
			return
		}

		latestClean := trimVersionPrefix(latest)
		ui.Info(fmt.Sprintf("Updating OpenBoot v%s → v%s...", currentVersion, latestClean))
		if err := DownloadAndReplace(); err != nil {
			ui.Warn(fmt.Sprintf("Auto-update failed: %v", err))
			ui.Muted("Run 'openboot update --self' to update manually")
			fmt.Println()
			return
		}
		ui.Success(fmt.Sprintf("Updated to v%s. Restart openboot to use the new version.", latestClean))
		fmt.Println()
	}
}

func DownloadAndReplace() error {
	arch := runtime.GOARCH
	if arch == "" {
		arch = "arm64"
	}

	url := fmt.Sprintf("https://github.com/openbootdotdev/openboot/releases/latest/download/openboot-darwin-%s", arch)

	client := getHTTPClient()
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	binPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine binary path: %w", err)
	}

	binPath, err = filepath.EvalSymlinks(binPath)
	if err != nil {
		return fmt.Errorf("cannot resolve binary path: %w", err)
	}

	tmpPath := binPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write binary: %w", err)
	}
	f.Close()

	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	if err := os.Rename(tmpPath, binPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	return nil
}

func notifyIfUpdateAvailable(currentVersion string) {
	state, err := loadState()
	if err != nil {
		return
	}

	if state.UpdateAvailable && isNewerVersion(state.LatestVersion, currentVersion) {
		ui.Warn(fmt.Sprintf("New version available: %s (current: v%s)", state.LatestVersion, currentVersion))
		ui.Muted("Run 'openboot update --self' to upgrade")
		fmt.Println()
	}
}

func checkForUpdatesAsync(currentVersion string) {
	go func() {
		state, _ := loadState()
		if state != nil && time.Since(state.LastCheck) < checkInterval {
			return
		}

		latestVersion, err := getLatestVersion()
		if err != nil {
			return
		}

		saveState(&CheckState{
			LastCheck:       time.Now(),
			LatestVersion:   latestVersion,
			UpdateAvailable: isNewerVersion(latestVersion, currentVersion),
		})
	}()
}

func isNewerVersion(latest, current string) bool {
	if latest == "" {
		return false
	}
	latestClean := trimVersionPrefix(latest)
	currentClean := trimVersionPrefix(current)
	return latestClean != currentClean && latestClean > currentClean
}

func trimVersionPrefix(v string) string {
	if len(v) > 0 && v[0] == 'v' {
		return v[1:]
	}
	return v
}

func getHTTPClient() *http.Client {
	httpClientOnce.Do(func() {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	})
	return httpClient
}

func getLatestVersion() (string, error) {
	client := getHTTPClient()
	resp, err := client.Get("https://api.github.com/repos/openbootdotdev/openboot/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	return release.TagName, nil
}

func getCheckFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".openboot", "update_state.json"), nil
}

func loadState() (*CheckState, error) {
	path, err := getCheckFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var state CheckState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

func saveState(state *CheckState) error {
	path, err := getCheckFilePath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
