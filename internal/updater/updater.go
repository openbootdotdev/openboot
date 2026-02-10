package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/openbootdotdev/openboot/internal/ui"
)

const checkInterval = 24 * time.Hour

type Release struct {
	TagName string `json:"tag_name"`
}

type CheckState struct {
	LastCheck       time.Time `json:"last_check"`
	LatestVersion   string    `json:"latest_version"`
	UpdateAvailable bool      `json:"update_available"`
}

func ShowUpdateNotificationIfAvailable(currentVersion string) {
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

func CheckForUpdatesAsync(currentVersion string) {
	go func() {
		state, _ := loadState()

		if state != nil && time.Since(state.LastCheck) < checkInterval {
			return
		}

		latestVersion, err := getLatestVersion()
		if err != nil {
			return
		}

		updateAvailable := isNewerVersion(latestVersion, currentVersion)

		saveState(&CheckState{
			LastCheck:       time.Now(),
			LatestVersion:   latestVersion,
			UpdateAvailable: updateAvailable,
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

func getLatestVersion() (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
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

func getCheckFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openboot", "update_state.json")
}

func loadState() (*CheckState, error) {
	data, err := os.ReadFile(getCheckFilePath())
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
	path := getCheckFilePath()
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0755)

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
