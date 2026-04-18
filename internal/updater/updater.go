package updater

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
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
	"syscall"
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
	// Guard against infinite re-exec: after an upgrade, execSelf sets this
	// env var so the new process skips AutoUpgrade on the first run.
	if os.Getenv("OPENBOOT_UPGRADING") == "1" {
		os.Unsetenv("OPENBOOT_UPGRADING")
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
		return
	}

	ui.Success(fmt.Sprintf("Updated to v%s. Restarting...", latestClean))
	fmt.Println()
	os.Setenv("OPENBOOT_UPGRADING", "1")
	execSelf()
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
	ui.Success(fmt.Sprintf("Updated to v%s. Restarting...", latestClean))
	fmt.Println()
	os.Setenv("OPENBOOT_UPGRADING", "1")
	execSelf()
}

// execSelf is a package-level variable to allow test injection.
var execSelf = func() {
	exe, err := os.Executable()
	if err != nil {
		ui.Warn(fmt.Sprintf("Could not restart: %v", err))
		ui.Muted("Please run the command again to use the new version.")
		return
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		ui.Warn(fmt.Sprintf("Could not restart: %v", err))
		ui.Muted("Please run the command again to use the new version.")
		return
	}
	if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
		ui.Warn(fmt.Sprintf("Could not restart: %v", err))
		ui.Muted("Please run the command again to use the new version.")
	}
}

// parseChecksumsFile parses a shasum-style checksums file ("<hex>  <filename>"
// or "<hex> *<filename>") into a map of filename → lowercase hex digest.
// Blank lines and comment lines (prefixed with '#') are ignored.
func parseChecksumsFile(r io.Reader) (map[string]string, error) {
	out := make(map[string]string)
	scanner := bufio.NewScanner(r)
	// Allow long lines up to 1 MiB for safety.
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Split into (hash, filename) on the first run of whitespace.
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		hash := strings.ToLower(fields[0])
		name := fields[len(fields)-1]
		// Strip the leading "*" marker used by some checksum tools for binary mode.
		name = strings.TrimPrefix(name, "*")
		// Strip any leading "./" that sha256sum may emit.
		name = strings.TrimPrefix(name, "./")
		out[name] = hash
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read checksums: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("checksums file is empty or unparseable")
	}
	return out, nil
}

// verifyChecksum returns nil if the SHA-256 of the file at path matches the
// entry for filename in the provided checksums map. It fails loudly if the
// entry is missing or the hash does not match.
func verifyChecksum(path, filename string, checksums map[string]string) error {
	expected, ok := checksums[filename]
	if !ok {
		return fmt.Errorf("no checksum entry for %q in checksums file", filename)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s for checksum: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash %s: %w", path, err)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", filename, expected, actual)
	}
	return nil
}

// fetchChecksums downloads and parses the checksums.txt file published
// alongside a GitHub release. Uses the /latest/ redirect so it matches the
// binary downloaded in DownloadAndReplace.
func fetchChecksums(client *http.Client) (map[string]string, error) {
	url := "https://github.com/openbootdotdev/openboot/releases/latest/download/checksums.txt"
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download checksums: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download checksums: HTTP %d", resp.StatusCode)
	}
	// Cap the body to 1 MiB; a checksums file is tiny.
	return parseChecksumsFile(io.LimitReader(resp.Body, 1<<20))
}

func DownloadAndReplace() error {
	if IsHomebrewInstall() {
		return fmt.Errorf("openboot is managed by Homebrew — run 'brew upgrade openboot' instead")
	}

	arch := runtime.GOARCH
	if arch == "" {
		arch = "arm64"
	}

	filename := fmt.Sprintf("openboot-darwin-%s", arch)

	client := getHTTPClient()

	// Fetch checksums first so we can verify integrity before overwriting the
	// running binary. If the checksums file is missing or malformed, abort.
	checksums, err := fetchChecksums(client)
	if err != nil {
		return fmt.Errorf("verify update integrity: %w", err)
	}

	// Uses /latest/ redirect — always downloads whatever GitHub considers the
	// current latest release. The displayed version (from resolveLatestVersion)
	// may differ by one patch if a release lands between the check and download.
	url := fmt.Sprintf("https://github.com/openbootdotdev/openboot/releases/latest/download/%s", filename)

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

	// Ensure tmp file is cleaned up on any failure (including panics).
	// Set to false after successful rename.
	needsCleanup := true
	defer func() {
		if needsCleanup {
			os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup
		}
	}()

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return fmt.Errorf("write binary: %w", err)
	}
	f.Close()

	// Verify checksum BEFORE chmod/rename so a tampered or truncated download
	// never replaces the running binary.
	if err := verifyChecksum(tmpPath, filename, checksums); err != nil {
		return fmt.Errorf("integrity check failed: %w", err)
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	if err := os.Rename(tmpPath, binPath); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}

	needsCleanup = false
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
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
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
