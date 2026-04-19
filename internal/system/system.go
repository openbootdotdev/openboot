package system

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/openbootdotdev/openboot/internal/httputil"
)

func HomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return home, nil
}

func Architecture() string {
	return runtime.GOARCH
}

func HomebrewPrefix() string {
	if Architecture() == "arm64" {
		return "/opt/homebrew"
	}
	return "/usr/local"
}

func IsHomebrewInstalled() bool {
	_, err := exec.LookPath("brew")
	return err == nil
}

func IsXcodeCliInstalled() bool {
	cmd := exec.Command("xcode-select", "-p")
	return cmd.Run() == nil
}

func IsGumInstalled() bool {
	_, err := exec.LookPath("gum")
	return err == nil
}

func RunCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...) //nolint:gosec // intentional generic runner; callers are responsible for validating name and args
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func RunCommandSilent(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...) //nolint:gosec // intentional generic runner; callers are responsible for validating name and args
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

// RunCommandOutput runs name with args and returns stdout only (not stderr).
// Use when stderr output must not contaminate parsed stdout (e.g. version probes, list commands).
func RunCommandOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...) //nolint:gosec // intentional generic runner; callers are responsible for validating name and args
	output, err := cmd.Output()
	return strings.TrimSpace(string(output)), err
}

// knownBrewInstallHash is the SHA256 of the Homebrew install script pinned on
// 2026-04-19 (Homebrew/install HEAD as of that date). Update this constant
// whenever the installer script changes upstream by running:
//
//	curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh | sha256sum
const knownBrewInstallHash = "dfd5145fe2aa5956a600e35848765273f5798ce6def01bd08ecec088a1268d91"

// brewInstallURL is a var so tests can redirect it without a real server.
var brewInstallURL = "https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh"

// brewHTTPClient is a var so tests can inject a mock transport.
var brewHTTPClient *http.Client = http.DefaultClient

func InstallHomebrew() error {
	// Download the installer via httputil.Do so rate-limit handling is applied.
	req, err := http.NewRequest(http.MethodGet, brewInstallURL, nil)
	if err != nil {
		return fmt.Errorf("create homebrew install request: %w", err)
	}
	resp, err := httputil.Do(brewHTTPClient, req)
	if err != nil {
		return fmt.Errorf("download homebrew install script: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download homebrew install script: unexpected status %d", resp.StatusCode)
	}

	scriptBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read homebrew install script: %w", err)
	}

	// Verify SHA256 before executing anything.
	sum := sha256.Sum256(scriptBytes)
	got := hex.EncodeToString(sum[:])
	if got != knownBrewInstallHash {
		return fmt.Errorf("homebrew installer SHA256 mismatch: refusing to execute")
	}

	// Write verified script to a temp file, execute, then clean up.
	tmpFile, err := os.CreateTemp("", "homebrew-install-*.sh")
	if err != nil {
		return fmt.Errorf("create temp file for homebrew install: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(scriptBytes); err != nil {
		tmpFile.Close() //nolint:gosec,errcheck // error-path cleanup; original write error takes precedence
		return fmt.Errorf("write homebrew install script: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close homebrew install script: %w", err)
	}

	if err := os.Chmod(tmpFile.Name(), 0700); err != nil { //nolint:gosec // install script must be executable
		return fmt.Errorf("chmod homebrew install script: %w", err)
	}

	tty, opened := OpenTTY()
	if opened {
		defer tty.Close() //nolint:errcheck // best-effort TTY cleanup
	}

	cmd := exec.Command(tmpFile.Name()) //nolint:gosec // script content is SHA256-verified before execution
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = tty
	return cmd.Run()
}

func GetGitConfig(key string) string {
	// Try global first (most common)
	output, err := RunCommandSilent("git", "config", "--global", key)
	if err == nil && output != "" {
		return output
	}

	// Fall back to any available config (local, system, etc.)
	output, err = RunCommandSilent("git", "config", key)
	if err == nil {
		return output
	}

	return ""
}

func GetExistingGitConfig() (name, email string) {
	name = GetGitConfig("user.name")
	email = GetGitConfig("user.email")
	return
}

func ConfigureGit(name, email string) error {
	if err := RunCommand("git", "config", "--global", "user.name", name); err != nil {
		return fmt.Errorf("set git name: %w", err)
	}
	if err := RunCommand("git", "config", "--global", "user.email", email); err != nil {
		return fmt.Errorf("set git email: %w", err)
	}
	return nil
}

func HasTTY() bool {
	f, err := os.Open("/dev/tty")
	if err != nil {
		return false
	}
	f.Close() //nolint:errcheck,gosec // probe-only open; close error is non-critical
	return true
}

// OpenTTY opens /dev/tty for interactive input, falling back to os.Stdin.
// Use this instead of os.Stdin when a subprocess needs a terminal (e.g. sudo
// password prompts), because os.Stdin may not be a TTY after curl|bash piping.
// The caller should close the returned file when done; closing is safe even
// when os.Stdin is returned (it is a no-op duplicate in that case).
func OpenTTY() (tty *os.File, opened bool) {
	f, err := os.Open("/dev/tty")
	if err != nil {
		return os.Stdin, false
	}
	return f, true
}
