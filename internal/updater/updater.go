package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openbootdotdev/openboot/internal/ui"
)

const CheckInterval = 24 * time.Hour

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

func LoadUserConfig() UserConfig {
	cfg := UserConfig{AutoUpdate: AutoUpdateEnabled}
	path, err := getUserConfigPath()
	if err != nil {
		return cfg
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg
	}
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

func isHomebrewPath(binPath string) bool {
	return strings.Contains(binPath, "/Cellar/") ||
		strings.HasPrefix(binPath, "/opt/homebrew/") ||
		strings.HasPrefix(binPath, "/usr/local/Homebrew/") ||
		strings.HasPrefix(binPath, "/home/linuxbrew/")
}

func IsHomebrewInstall() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return false
	}
	return isHomebrewPath(exe)
}

// AutoUpgrade checks for a newer version and upgrades if appropriate.
//
// Flow:
//  1. Kill switch: OPENBOOT_DISABLE_AUTOUPDATE=1
//  2. Dev guard: currentVersion == "dev"
//  3. UserConfig (applies to ALL install methods):
//     disabled → exit, notify → show message, enabled → upgrade
//  4. resolveLatestVersion: uses 24h cache, falls back to sync GitHub API
//  5. Upgrade method: Homebrew → brew upgrade, Direct → download binary
func AutoUpgrade(currentVersion string) {
	if os.Getenv("OPENBOOT_DISABLE_AUTOUPDATE") == "1" {
		return
	}
	if currentVersion == "dev" {
		return
	}

	cfg := LoadUserConfig()
	if cfg.AutoUpdate == AutoUpdateDisabled {
		return
	}

	latest := resolveLatestVersion(currentVersion)
	if latest == "" || !isNewerVersion(latest, currentVersion) {
		return
	}

	if cfg.AutoUpdate == AutoUpdateNotify {
		notifyUpdate(currentVersion, latest)
		return
	}

	if IsHomebrewInstall() {
		doBrewUpgrade(currentVersion, latest)
	} else {
		doDirectUpgrade(currentVersion, latest)
	}
}

// fetchLatestVersion is a package-level variable to allow test injection.
var fetchLatestVersion = GetLatestVersion

// resolveLatestVersion returns the latest known version, using a 24h cache
// to avoid excessive GitHub API calls. Falls back to a synchronous API call
// when the cache is missing or stale.
func resolveLatestVersion(currentVersion string) string {
	state, err := LoadState()
	if err == nil && time.Since(state.LastCheck) < CheckInterval {
		return state.LatestVersion
	}

	latest, err := fetchLatestVersion()
	if err != nil {
		return ""
	}

	if err := SaveState(&CheckState{
		LastCheck:       time.Now(),
		LatestVersion:   latest,
		UpdateAvailable: isNewerVersion(latest, currentVersion),
	}); err != nil {
		ui.Muted(fmt.Sprintf("Warning: could not cache update state: %v", err))
	}
	return latest
}

func notifyUpdate(currentVersion, latestVersion string) {
	latestClean := trimVersionPrefix(latestVersion)
	currentClean := trimVersionPrefix(currentVersion)
	ui.Warn(fmt.Sprintf("New version available: v%s (current: v%s)", latestClean, currentClean))
	if IsHomebrewInstall() {
		ui.Muted("Run 'brew upgrade openboot' to upgrade")
	} else {
		ui.Muted("Run 'openboot update --self' to upgrade")
	}
	fmt.Println()
}

const brewTap = "openbootdotdev/tap"
const brewFormula = brewTap + "/openboot"

// execBrewUpgrade is a package-level variable to allow test injection.
var execBrewUpgrade = func(formula string) error {
	script := fmt.Sprintf(
		`git -C "$(brew --repo %s)" pull --ff-only && brew upgrade %s`,
		brewTap, formula,
	)
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1")
	return cmd.Run()
}

func doBrewUpgrade(currentVersion, latestVersion string) {
	latestClean := trimVersionPrefix(latestVersion)
	currentClean := trimVersionPrefix(currentVersion)
	ui.Info(fmt.Sprintf("Updating OpenBoot v%s → v%s via Homebrew...", currentClean, latestClean))
	if err := execBrewUpgrade(brewFormula); err != nil {
		ui.Warn(fmt.Sprintf("Auto-update failed: %v", err))
		ui.Muted("Run 'brew upgrade openboot' to update manually")
		fmt.Println()
	} else {
		ui.Success(fmt.Sprintf("Updated to v%s. Restart openboot to use the new version.", latestClean))
		fmt.Println()
	}
}

func doDirectUpgrade(currentVersion, latestVersion string) {
	latestClean := trimVersionPrefix(latestVersion)
	currentClean := trimVersionPrefix(currentVersion)
	ui.Info(fmt.Sprintf("Updating OpenBoot v%s → v%s...", currentClean, latestClean))
	if err := DownloadAndReplace(); err != nil {
		ui.Warn(fmt.Sprintf("Auto-update failed: %v", err))
		ui.Muted("Run 'openboot update --self' to update manually")
		fmt.Println()
		return
	}
	ui.Success(fmt.Sprintf("Updated to v%s. Restart openboot to use the new version.", latestClean))
	fmt.Println()
}

func DownloadAndReplace() error {
	if IsHomebrewInstall() {
		return fmt.Errorf("openboot is managed by Homebrew — run 'brew upgrade openboot' instead")
	}

	arch := runtime.GOARCH
	if arch == "" {
		arch = "arm64"
	}

	// Uses /latest/ redirect — always downloads whatever GitHub considers the
	// current latest release. The displayed version (from resolveLatestVersion)
	// may differ by one patch if a release lands between the check and download.
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
		return fmt.Errorf("binary path: %w", err)
	}

	binPath, err = filepath.EvalSymlinks(binPath)
	if err != nil {
		return fmt.Errorf("resolve binary path: %w", err)
	}

	tmpPath := binPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write binary: %w", err)
	}
	f.Close()

	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("chmod: %w", err)
	}

	if err := os.Rename(tmpPath, binPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("replace binary: %w", err)
	}

	return nil
}

// --- Version comparison ---

func isNewerVersion(latest, current string) bool {
	if latest == "" {
		return false
	}
	if current == "dev" {
		return false
	}
	latestClean := trimVersionPrefix(latest)
	currentClean := trimVersionPrefix(current)
	return compareSemver(latestClean, currentClean) > 0
}

func compareSemver(a, b string) int {
	aParts := parseSemver(a)
	bParts := parseSemver(b)
	for i := 0; i < 3; i++ {
		if aParts[i] != bParts[i] {
			if aParts[i] > bParts[i] {
				return 1
			}
			return -1
		}
	}
	return 0
}

func parseSemver(v string) [3]int {
	var result [3]int
	parts := strings.SplitN(v, ".", 3)
	for i, p := range parts {
		if i >= 3 {
			break
		}
		n, err := strconv.Atoi(p)
		if err == nil {
			result[i] = n
		}
	}
	return result
}

func trimVersionPrefix(v string) string {
	if len(v) > 0 && v[0] == 'v' {
		return v[1:]
	}
	return v
}

// --- Network ---

func getHTTPClient() *http.Client {
	httpClientOnce.Do(func() {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	})
	return httpClient
}

func GetLatestVersion() (string, error) {
	client := getHTTPClient()
	req, err := http.NewRequest("GET", "https://api.github.com/repos/openbootdotdev/openboot/releases/latest", nil)
	if err != nil {
		return "", err
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
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

// --- State persistence ---

func getCheckFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".openboot", "update_state.json"), nil
}

func LoadState() (*CheckState, error) {
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

func SaveState(state *CheckState) error {
	path, err := getCheckFilePath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
