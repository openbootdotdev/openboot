package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/openbootdotdev/openboot/internal/httputil"
	"github.com/openbootdotdev/openboot/internal/system"
)

// knownOMZInstallHash is the SHA256 of the Oh-My-Zsh install script pinned on
// 2026-04-19 (ohmyzsh/ohmyzsh master, commit circa that date). Update this
// constant whenever the installer script changes upstream.
const knownOMZInstallHash = "21043aec5b791ce4835479dc33ba2f92155946aeafd54604a8c83522627cc803"

// omzInstallURL is a var so tests can redirect it without a real server.
var omzInstallURL = "https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh"

// omzHTTPClient is a var so tests can inject a mock transport.
var omzHTTPClient *http.Client = http.DefaultClient

var shellIdentifierRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func validateShellIdentifier(value, label string) error {
	if value == "" {
		return nil
	}
	if !shellIdentifierRe.MatchString(value) {
		return fmt.Errorf("invalid %s: %q (only alphanumerics, hyphens, underscores, dots allowed)", label, value)
	}
	return nil
}

func IsOhMyZshInstalled() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".oh-my-zsh"))
	return err == nil
}

func InstallOhMyZsh(dryRun bool) error {
	if IsOhMyZshInstalled() {
		return nil
	}

	if dryRun {
		fmt.Println("[DRY-RUN] Would install Oh-My-Zsh")
		return nil
	}

	// Download the installer via httputil.Do so rate-limit handling is applied.
	req, err := http.NewRequest(http.MethodGet, omzInstallURL, nil)
	if err != nil {
		return fmt.Errorf("create omz install request: %w", err)
	}
	resp, err := httputil.Do(omzHTTPClient, req)
	if err != nil {
		return fmt.Errorf("download omz install script: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download omz install script: unexpected status %d", resp.StatusCode)
	}

	scriptBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read omz install script: %w", err)
	}

	// Verify SHA256 before executing anything.
	sum := sha256.Sum256(scriptBytes)
	got := hex.EncodeToString(sum[:])
	if got != knownOMZInstallHash {
		return fmt.Errorf("Oh-My-Zsh install script hash mismatch: download may be compromised (got %s, want %s)", got, knownOMZInstallHash)
	}

	// Write verified script to a temp file, execute, then clean up.
	tmpFile, err := os.CreateTemp("", "omz-install-*.sh")
	if err != nil {
		return fmt.Errorf("create temp file for omz install: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(scriptBytes); err != nil {
		tmpFile.Close() //nolint:gosec,errcheck // error-path cleanup; original write error takes precedence
		return fmt.Errorf("write omz install script: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close omz install script: %w", err)
	}

	if err := os.Chmod(tmpFile.Name(), 0700); err != nil { //nolint:gosec // install script must be executable
		return fmt.Errorf("chmod omz install script: %w", err)
	}

	return system.RunCommand(tmpFile.Name(), "--unattended")
}

const brewShellenvLine = `eval "$(/opt/homebrew/bin/brew shellenv)"`

// EnsureBrewShellenv adds the Homebrew shellenv eval to ~/.zshrc on Apple
// Silicon if it isn't already present. This is required because /opt/homebrew
// is not in the default PATH.
func EnsureBrewShellenv(dryRun bool) error {
	if _, err := os.Stat("/opt/homebrew/bin/brew"); os.IsNotExist(err) {
		return nil // Intel Mac or no Homebrew — not needed
	}

	home, err := system.HomeDir()
	if err != nil {
		return fmt.Errorf("ensure brew shellenv: %w", err)
	}
	zshrcPath := filepath.Join(home, ".zshrc")

	// Create .zshrc if it doesn't exist.
	if _, err := os.Stat(zshrcPath); os.IsNotExist(err) {
		if dryRun {
			fmt.Printf("[DRY-RUN] Would create %s with Homebrew shellenv\n", zshrcPath)
			return nil
		}
		if err := os.WriteFile(zshrcPath, []byte(brewShellenvLine+"\n"), 0600); err != nil {
			return fmt.Errorf("create .zshrc: %w", err)
		}
		return nil
	}

	raw, err := os.ReadFile(zshrcPath)
	if err != nil {
		return fmt.Errorf("read .zshrc: %w", err)
	}
	if strings.Contains(string(raw), "brew shellenv") {
		return nil // already present
	}

	if dryRun {
		fmt.Println("[DRY-RUN] Would add Homebrew shellenv to .zshrc")
		return nil
	}

	content := string(raw)
	if len(content) > 0 && content[len(content)-1] != '\n' {
		content += "\n"
	}
	content = brewShellenvLine + "\n" + content
	return os.WriteFile(zshrcPath, []byte(content), 0600) //nolint:gosec // path derived from os.UserHomeDir, not user input
}

func SetDefaultShell(dryRun bool) error {
	zshPath := "/bin/zsh"
	if _, err := os.Stat(zshPath); os.IsNotExist(err) {
		zshPath = "/usr/bin/zsh"
	}

	if dryRun {
		fmt.Printf("[DRY-RUN] Would set default shell to %s\n", zshPath)
		return nil
	}

	cmd := exec.Command("chsh", "-s", zshPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	tty, opened := system.OpenTTY()
	if opened {
		defer tty.Close() //nolint:errcheck // best-effort TTY cleanup
	}
	cmd.Stdin = tty
	return cmd.Run()
}

const restoreBlockStart = "# >>> OpenBoot-Restore"
const restoreBlockEnd = "# <<< OpenBoot-Restore"

var restoreBlockRe = regexp.MustCompile(`(?s)# >>> OpenBoot-Restore\n.*?# <<< OpenBoot-Restore\n?`)

func buildRestoreBlock(theme string, plugins []string) (string, error) {
	if err := validateShellIdentifier(theme, "ZSH_THEME"); err != nil {
		return "", err
	}
	for _, p := range plugins {
		if err := validateShellIdentifier(p, "plugin"); err != nil {
			return "", err
		}
	}

	var sb strings.Builder
	sb.WriteString(restoreBlockStart + "\n")
	if theme != "" {
		fmt.Fprintf(&sb, "ZSH_THEME=\"%s\"\n", theme)
	}
	if len(plugins) > 0 {
		fmt.Fprintf(&sb, "plugins=(%s)\n", strings.Join(plugins, " "))
	}
	sb.WriteString(restoreBlockEnd + "\n")
	return sb.String(), nil
}

var (
	looseThemeRe   = regexp.MustCompile(`(?m)^ZSH_THEME="[^"]*"\n?`)
	loosePluginsRe = regexp.MustCompile(`(?m)^plugins=\((?s:.*?)\)\n?`)
)

func patchZshrcBlock(zshrcPath, theme string, plugins []string) error {
	raw, err := os.ReadFile(zshrcPath)
	if err != nil {
		return fmt.Errorf("read .zshrc: %w", err)
	}

	block, err := buildRestoreBlock(theme, plugins)
	if err != nil {
		return fmt.Errorf("build restore block: %w", err)
	}
	content := string(raw)

	if restoreBlockRe.MatchString(content) {
		content = restoreBlockRe.ReplaceAllString(content, block)
	} else {
		if theme != "" {
			content = looseThemeRe.ReplaceAllString(content, "")
		}
		if len(plugins) > 0 {
			content = loosePluginsRe.ReplaceAllString(content, "")
		}
		if len(content) > 0 && content[len(content)-1] != '\n' {
			content = content + "\n"
		}
		content = content + block
	}

	tmpPath := zshrcPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0600); err != nil { //nolint:gosec // path derived from os.UserHomeDir, not user input
		return fmt.Errorf("write .zshrc: %w", err)
	}
	if err := os.Rename(tmpPath, zshrcPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename .zshrc: %w", err)
	}
	return nil
}

func RestoreFromSnapshot(ohMyZsh bool, theme string, plugins []string, dryRun bool) error {
	if !ohMyZsh {
		return nil
	}

	if !IsOhMyZshInstalled() {
		if dryRun {
			fmt.Println("[DRY-RUN] Would install Oh-My-Zsh")
		} else {
			if err := InstallOhMyZsh(dryRun); err != nil {
				return fmt.Errorf("install oh-my-zsh: %w", err)
			}
		}
	}

	home, err := system.HomeDir()
	if err != nil {
		return fmt.Errorf("configure zshrc: %w", err)
	}
	zshrcPath := filepath.Join(home, ".zshrc")

	if _, err := os.Stat(zshrcPath); os.IsNotExist(err) {
		if dryRun {
			fmt.Printf("[DRY-RUN] Would create %s\n", zshrcPath)
			return nil
		}
		if err := validateShellIdentifier(theme, "ZSH_THEME"); err != nil {
			return fmt.Errorf("validate theme: %w", err)
		}
		for _, p := range plugins {
			if err := validateShellIdentifier(p, "plugin"); err != nil {
				return fmt.Errorf("validate plugin: %w", err)
			}
		}
		template := fmt.Sprintf(`export ZSH="$HOME/.oh-my-zsh"
ZSH_THEME="%s"
plugins=(%s)
source $ZSH/oh-my-zsh.sh
`, theme, strings.Join(plugins, " "))
		if err := os.WriteFile(zshrcPath, []byte(template), 0600); err != nil {
			return fmt.Errorf("create .zshrc: %w", err)
		}
		return nil
	}

	if dryRun {
		if theme != "" {
			fmt.Printf("[DRY-RUN] Would set ZSH_THEME=\"%s\"\n", theme)
		}
		if len(plugins) > 0 {
			fmt.Printf("[DRY-RUN] Would set plugins=(%s)\n", strings.Join(plugins, " "))
		}
		return nil
	}

	if theme == "" && len(plugins) == 0 {
		return nil
	}

	return patchZshrcBlock(zshrcPath, theme, plugins)
}
