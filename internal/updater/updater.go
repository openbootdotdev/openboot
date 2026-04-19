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
	"regexp"
	"runtime"
	"sort"
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

// LoadUserConfig returns the user's update preference from
// ~/.openboot/config.json. The default is AutoUpdateNotify: we surface a
// one-line "new version available" message but never auto-upgrade the binary
// during a normal command. Users who want silent upgrades can opt in by
// setting "autoupdate": "true"; users who want silence can set "false".
func LoadUserConfig() UserConfig {
	cfg := UserConfig{AutoUpdate: AutoUpdateNotify}
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
		cfg.AutoUpdate = AutoUpdateNotify
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

// AutoUpgrade checks for a newer version and, by default, only prints a notice.
// Silent self-upgrades are opt-in via ~/.openboot/config.json.
//
// Flow:
//  1. Kill switch: OPENBOOT_DISABLE_AUTOUPDATE=1
//  2. Dev guard: currentVersion == "dev"
//  3. UserConfig (applies to ALL install methods):
//     disabled → exit, notify (default) → show message, enabled → upgrade
//  4. resolveLatestVersion: uses 24h cache, falls back to sync GitHub API
//  5. Upgrade method (enabled mode only): Homebrew → brew upgrade, Direct → download binary
func AutoUpgrade(currentVersion string) {
	if os.Getenv("OPENBOOT_DISABLE_AUTOUPDATE") == "1" {
		return
	}
	// Guard against infinite re-exec: after an upgrade, execSelf sets this
	// env var so the new process skips AutoUpgrade on the first run.
	if os.Getenv("OPENBOOT_UPGRADING") == "1" {
		os.Unsetenv("OPENBOOT_UPGRADING") //nolint:errcheck // best-effort env cleanup; failure is non-critical
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
	latestClean := TrimVersionPrefix(latestVersion)
	currentClean := TrimVersionPrefix(currentVersion)
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
	// Step 1: resolve the tap repository path.
	repoOut, err := exec.Command("brew", "--repo", brewTap).Output()
	if err != nil {
		return fmt.Errorf("brew --repo %s: %w", brewTap, err)
	}
	repoPath := strings.TrimSpace(string(repoOut))

	// Step 2: fast-forward the tap to pick up the new formula revision.
	gitCmd := exec.Command("git", "-C", repoPath, "pull", "--ff-only") //nolint:gosec // "git" is hardcoded; repoPath is derived from brew --repository
	gitCmd.Env = append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1")
	if err := gitCmd.Run(); err != nil {
		return fmt.Errorf("git pull tap: %w", err)
	}

	// Step 3: upgrade the formula.
	upgradeCmd := exec.Command("brew", "upgrade", formula) //nolint:gosec // "brew" is hardcoded; formula is validated before this call
	upgradeCmd.Env = append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1")
	if err := upgradeCmd.Run(); err != nil {
		return fmt.Errorf("brew upgrade %s: %w", formula, err)
	}
	return nil
}

func doBrewUpgrade(currentVersion, latestVersion string) {
	latestClean := TrimVersionPrefix(latestVersion)
	currentClean := TrimVersionPrefix(currentVersion)
	ui.Info(fmt.Sprintf("Updating OpenBoot v%s → v%s via Homebrew...", currentClean, latestClean))
	if err := execBrewUpgrade(brewFormula); err != nil {
		ui.Warn(fmt.Sprintf("Auto-update failed: %v", err))
		ui.Muted("Run 'brew upgrade openboot' to update manually")
		fmt.Println()
		return
	}

	ui.Success(fmt.Sprintf("Updated to v%s. Restarting...", latestClean))
	fmt.Println()
	os.Setenv("OPENBOOT_UPGRADING", "1") //nolint:errcheck,gosec // non-critical guard env var; failure leaves upgrade loop protection off
	execSelf()
}

func doDirectUpgrade(currentVersion, latestVersion string) {
	latestClean := TrimVersionPrefix(latestVersion)
	currentClean := TrimVersionPrefix(currentVersion)
	ui.Info(fmt.Sprintf("Updating OpenBoot v%s → v%s...", currentClean, latestClean))
	if err := DownloadAndReplace(latestVersion, currentVersion); err != nil {
		ui.Warn(fmt.Sprintf("Auto-update failed: %v", err))
		ui.Muted("Run 'openboot update --self' to update manually")
		fmt.Println()
		return
	}
	ui.Success(fmt.Sprintf("Updated to v%s. Restarting...", latestClean))
	fmt.Println()
	os.Setenv("OPENBOOT_UPGRADING", "1") //nolint:errcheck,gosec // non-critical guard env var; failure leaves upgrade loop protection off
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
	if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil { //nolint:gosec // exe is the current process path; this is an intentional self-restart
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
	defer f.Close() //nolint:errcheck // read-only file; close error is non-critical

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
// alongside a GitHub release. If version is empty, the /latest/ redirect is
// used (back-compat with AutoUpgrade); otherwise the exact release path
// /releases/download/v<version>/ is used.
func fetchChecksums(client *http.Client, version string) (map[string]string, error) {
	url := checksumsURL(version)
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download checksums: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // standard HTTP body cleanup
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download checksums: HTTP %d", resp.StatusCode)
	}
	// Cap the body to 1 MiB; a checksums file is tiny.
	return parseChecksumsFile(io.LimitReader(resp.Body, 1<<20))
}

// checksumsURL returns the correct checksums.txt URL for the given version.
// Empty version means use /latest/.
func checksumsURL(version string) string {
	if version == "" {
		return "https://github.com/openbootdotdev/openboot/releases/latest/download/checksums.txt"
	}
	return fmt.Sprintf("https://github.com/openbootdotdev/openboot/releases/download/v%s/checksums.txt", TrimVersionPrefix(version))
}

// binaryURL returns the correct binary download URL for the given version and
// filename. Empty version means use /latest/.
func binaryURL(version, filename string) string {
	if version == "" {
		return fmt.Sprintf("https://github.com/openbootdotdev/openboot/releases/latest/download/%s", filename)
	}
	return fmt.Sprintf("https://github.com/openbootdotdev/openboot/releases/download/v%s/%s", TrimVersionPrefix(version), filename)
}

// DownloadAndReplace downloads the openboot binary for the given target
// version and atomically replaces the currently-running binary.
//
// targetVersion controls which release to fetch: empty → GitHub /latest/
// redirect (back-compat with AutoUpgrade), non-empty → exact release URL.
// currentVersion is recorded in the backup filename for rollback UX; it may
// be empty (renders as "unknown").
//
// Before the atomic rename, the current binary is copied to the backup
// directory (see GetBackupDir) for rollback via Rollback(). The newest
// backupRetention backups are kept; older ones are pruned.
func DownloadAndReplace(targetVersion, currentVersion string) error {
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
	checksums, err := fetchChecksums(client, targetVersion)
	if err != nil {
		return fmt.Errorf("verify update integrity: %w", err)
	}

	url := binaryURL(targetVersion, filename)

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // standard HTTP body cleanup

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
			os.Remove(tmpPath) //nolint:errcheck,gosec // best-effort cleanup
		}
	}()

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close() //nolint:errcheck,gosec // already returning a more descriptive error
		return fmt.Errorf("write binary: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Verify checksum BEFORE chmod/rename so a tampered or truncated download
	// never replaces the running binary.
	if err := verifyChecksum(tmpPath, filename, checksums); err != nil {
		return fmt.Errorf("integrity check failed: %w", err)
	}

	if err := os.Chmod(tmpPath, 0755); err != nil { //nolint:gosec // downloaded binary must be executable
		return fmt.Errorf("chmod: %w", err)
	}

	// Back up the currently-running binary before we overwrite it. If backup
	// fails, we still proceed — the user asked to update and integrity is
	// already verified. Warn via UI so the user knows rollback is unavailable
	// for this upgrade.
	if err := backupCurrentBinary(binPath, currentVersion); err != nil {
		ui.Warn(fmt.Sprintf("could not create backup (rollback unavailable for this upgrade): %v", err))
	}

	if err := os.Rename(tmpPath, binPath); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}

	needsCleanup = false
	return nil
}

// --- Backup & rollback ---

// backupDirOverride is set by tests to redirect the backup directory.
// When empty, GetBackupDir returns ~/.openboot/backup.
var backupDirOverride string

// backupRetention is the maximum number of backups to keep. Older ones are
// pruned after each successful backup.
const backupRetention = 5

// GetBackupDir returns the directory where pre-upgrade binary backups are
// stored. Defaults to ~/.openboot/backup; tests may override via
// SetBackupDirForTesting.
func GetBackupDir() (string, error) {
	if backupDirOverride != "" {
		return backupDirOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".openboot", "backup"), nil
}

// SetBackupDirForTesting overrides the backup directory. Tests should defer a
// reset to "". It is exported for use by the cli package tests.
func SetBackupDirForTesting(dir string) {
	backupDirOverride = dir
}

// backupCurrentBinary copies the binary at binPath into the backup directory
// with a timestamped filename labeled by currentVersion (or "unknown" if
// empty), and prunes old backups beyond backupRetention.
func backupCurrentBinary(binPath, currentVersion string) error {
	dir, err := GetBackupDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("mkdir backup dir: %w", err)
	}

	version := currentVersion
	if version == "" {
		version = "unknown"
	}
	version = TrimVersionPrefix(version)

	ts := time.Now().UTC().Format("20060102T150405Z")
	dst := filepath.Join(dir, fmt.Sprintf("openboot-%s-%s", version, ts))

	if err := copyFile(binPath, dst, 0755); err != nil {
		return fmt.Errorf("copy binary: %w", err)
	}

	if err := pruneBackups(dir, backupRetention); err != nil {
		// Pruning failure is not fatal — backup succeeded.
		ui.Muted(fmt.Sprintf("Warning: could not prune old backups: %v", err))
	}
	return nil
}

// copyFile copies src to dst, preserving the given permission mode on dst.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer in.Close() //nolint:errcheck // read-only

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode) //nolint:gosec // backup files must be executable
	if err != nil {
		return fmt.Errorf("create dst: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close() //nolint:errcheck,gosec // returning the more descriptive error
		return fmt.Errorf("copy: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close dst: %w", err)
	}
	return nil
}

// listBackupsSorted returns backup file entries sorted by modification time,
// newest first.
func listBackupsSorted(dir string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	// Filter to files only and sort by modtime descending.
	files := entries[:0]
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		files = append(files, e)
	}
	sort.Slice(files, func(i, j int) bool {
		ii, _ := files[i].Info()
		jj, _ := files[j].Info()
		if ii == nil || jj == nil {
			return files[i].Name() > files[j].Name()
		}
		return ii.ModTime().After(jj.ModTime())
	})
	return files, nil
}

// pruneBackups deletes the oldest backup files in dir so that at most keep
// files remain.
func pruneBackups(dir string, keep int) error {
	files, err := listBackupsSorted(dir)
	if err != nil {
		return err
	}
	if len(files) <= keep {
		return nil
	}
	for _, f := range files[keep:] {
		if err := os.Remove(filepath.Join(dir, f.Name())); err != nil {
			return fmt.Errorf("remove %s: %w", f.Name(), err)
		}
	}
	return nil
}

// ListBackups returns the names of backup files in the backup directory,
// newest first. Returns an empty slice (not an error) if the directory does
// not exist.
func ListBackups() ([]string, error) {
	dir, err := GetBackupDir()
	if err != nil {
		return nil, err
	}
	files, err := listBackupsSorted(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read backup dir: %w", err)
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.Name())
	}
	return names, nil
}

// Rollback restores the most recent backup over the currently-running
// binary. Fails if no backup exists or openboot is managed by Homebrew.
func Rollback() error {
	if IsHomebrewInstall() {
		return fmt.Errorf("openboot is managed by Homebrew — rollback is not supported (use 'brew' commands)")
	}
	dir, err := GetBackupDir()
	if err != nil {
		return err
	}
	files, err := listBackupsSorted(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no backups found in %s", dir)
		}
		return fmt.Errorf("read backup dir: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no backups found in %s", dir)
	}

	binPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("binary path: %w", err)
	}
	binPath, err = filepath.EvalSymlinks(binPath)
	if err != nil {
		return fmt.Errorf("resolve binary path: %w", err)
	}

	src := filepath.Join(dir, files[0].Name())
	tmpPath := binPath + ".rollback.tmp"
	if err := copyFile(src, tmpPath, 0755); err != nil {
		return fmt.Errorf("stage rollback: %w", err)
	}
	needsCleanup := true
	defer func() {
		if needsCleanup {
			os.Remove(tmpPath) //nolint:errcheck,gosec // best-effort cleanup
		}
	}()

	if err := os.Rename(tmpPath, binPath); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}
	needsCleanup = false
	return nil
}

// --- Semver validation ---

// semverRe accepts plain X.Y.Z form (with optional leading v). Pre-release and
// build metadata are intentionally not supported here — the releases this
// tool pins to use plain semver.
var semverRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)

// ValidateSemver returns nil if v is a valid X.Y.Z version (with or without a
// leading v) and an error otherwise.
func ValidateSemver(v string) error {
	if v == "" {
		return fmt.Errorf("version is empty")
	}
	if !semverRe.MatchString(v) {
		return fmt.Errorf("invalid version %q: must be X.Y.Z (e.g. 0.25.0)", v)
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
	latestClean := TrimVersionPrefix(latest)
	currentClean := TrimVersionPrefix(current)
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
			result[i] = n //nolint:gosec // bounds checked: SplitN(v, ".", 3) ensures i < 3; result is [3]int
		}
	}
	return result
}

func TrimVersionPrefix(v string) string {
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
	defer resp.Body.Close() //nolint:errcheck // standard HTTP body cleanup

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
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}
