package shell

import (
	"context"
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
	"time"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/httputil"
	"github.com/openbootdotdev/openboot/internal/system"
	"github.com/openbootdotdev/openboot/internal/ui"
)

// knownOMZInstallHash is the SHA256 of the Oh-My-Zsh install script pinned on
// 2026-04-19 (ohmyzsh/ohmyzsh master, commit circa that date). Update this
// constant whenever the installer script changes upstream.
const knownOMZInstallHash = "21043aec5b791ce4835479dc33ba2f92155946aeafd54604a8c83522627cc803"

const omzInstallTimeout = 10 * time.Minute

// omzInstallURL is a var so tests can redirect it without a real server.
var omzInstallURL = "https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh"

// omzHTTPClient is a var so tests can inject a mock transport.
var omzHTTPClient = &http.Client{Timeout: 30 * time.Second}

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
		ui.DryRunMsg("Would install Oh-My-Zsh")
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

	ctx, cancel := context.WithTimeout(context.Background(), omzInstallTimeout)
	defer cancel()
	return system.RunCommandContext(ctx, tmpFile.Name(), "--unattended")
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
			ui.DryRunMsg("Would create %s with Homebrew shellenv", zshrcPath)
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
		ui.DryRunMsg("Would add Homebrew shellenv to .zshrc")
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
		ui.DryRunMsg("Would set default shell to %s", zshPath)
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
		return "", fmt.Errorf("build restore block: %w", err)
	}
	for _, p := range plugins {
		if err := validateShellIdentifier(p, "plugin"); err != nil {
			return "", fmt.Errorf("build restore block: %w", err)
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

func patchZshrcBlock(zshrcPath, theme string, plugins []string, dryRun bool) error {
	if dryRun {
		return nil
	}
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

// resolvePluginURL maps a plugins=() entry to its external git repo URL.
// It is a var so tests can inject a fake catalog without the embedded data.
var resolvePluginURL = config.ZshPluginRepoURL

// cloneRunner clones an external plugin repo into dest. It is a var so tests
// can record invocations without a real git binary or network. The only exec
// call lives in internal/system, so no execAllowedPaths baseline change is
// needed.
var cloneRunner = func(url, dest string) error {
	return system.RunCommand("git", "clone", "--depth", "1", url, dest)
}

// cloneExternalPlugins git-clones any plugins in the list that are known
// external oh-my-zsh plugins (present in the catalog with a repo URL) into
// $ZSH_CUSTOM/plugins. Built-in and unknown plugins are left untouched — they
// stay as bare names in plugins=(). A failed clone is non-fatal: it warns and
// continues so one bad repo never aborts the whole restore and the plugins=()
// block is still written.
func cloneExternalPlugins(plugins []string, dryRun bool) error {
	home, err := system.HomeDir()
	if err != nil {
		return fmt.Errorf("clone external plugins: %w", err)
	}
	customPlugins := filepath.Join(home, ".oh-my-zsh", "custom", "plugins")

	for _, name := range plugins {
		url, ok := resolvePluginURL(name)
		if !ok {
			continue // built-in or unknown — leave untouched
		}
		// Defense in depth: the catalog is curated, but a server overlay may
		// one day supply URLs. Only ever clone over https.
		if !strings.HasPrefix(url, "https://") {
			ui.Warn(fmt.Sprintf("Skipping plugin %s: non-https repo URL %q", name, url))
			continue
		}

		dest := filepath.Join(customPlugins, name)
		if _, err := os.Stat(dest); err == nil {
			continue // already cloned — idempotent skip
		}

		if dryRun {
			ui.DryRunMsg("Would clone %s to %s", url, dest)
			continue
		}

		if err := os.MkdirAll(customPlugins, 0700); err != nil {
			return fmt.Errorf("create %s: %w", customPlugins, err)
		}
		if err := cloneRunner(url, dest); err != nil {
			ui.Warn(fmt.Sprintf("Failed to clone plugin %s: %v", name, err))
			continue
		}
	}
	return nil
}

// zshrcPluginsRe extracts the names inside a plugins=(...) declaration from a
// .zshrc. Mirrors snapshot.zshPluginsRe but tolerates leading whitespace so it
// also matches indented declarations in user-authored dotfiles.
var zshrcPluginsRe = regexp.MustCompile(`(?m)^\s*plugins=\((?s:(.*?))\)`)

// CloneExternalPluginsFromZshrc reads ~/.zshrc, extracts its plugins=() list,
// and git-clones any external (catalog) plugins not already present. It exists
// for the dotfiles path: when a user's shell setup comes entirely from a
// stowed .zshrc (the remote config carries no shell block), the plugin list
// never flows through RestoreFromSnapshot, so the external plugins it names are
// never cloned and oh-my-zsh logs "plugin '...' not found" at startup.
//
// It is a no-op when oh-my-zsh isn't installed or .zshrc is absent. Built-in
// and unknown plugins are left untouched; a failed clone is non-fatal (see
// cloneExternalPlugins).
func CloneExternalPluginsFromZshrc(dryRun bool) error {
	if !IsOhMyZshInstalled() {
		return nil
	}
	home, err := system.HomeDir()
	if err != nil {
		return fmt.Errorf("clone plugins from .zshrc: %w", err)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read .zshrc: %w", err)
	}
	m := zshrcPluginsRe.FindSubmatch(raw)
	if len(m) < 2 {
		return nil // no plugins=() declaration
	}
	plugins := strings.Fields(string(m[1]))
	if len(plugins) == 0 {
		return nil
	}
	return cloneExternalPlugins(plugins, dryRun)
}

func RestoreFromSnapshot(ohMyZsh bool, theme string, plugins []string, dryRun bool) error {
	if !ohMyZsh {
		return nil
	}

	if !IsOhMyZshInstalled() {
		if dryRun {
			ui.DryRunMsg("Would install Oh-My-Zsh")
		} else {
			if err := InstallOhMyZsh(dryRun); err != nil {
				return fmt.Errorf("install oh-my-zsh: %w", err)
			}
		}
	}

	// External plugins (zsh-autosuggestions, ...) are not bundled with OMZ and
	// must be git-cloned into $ZSH_CUSTOM/plugins before plugins=() references
	// them. Built-in plugins are left untouched.
	if err := cloneExternalPlugins(plugins, dryRun); err != nil {
		return fmt.Errorf("clone external plugins: %w", err)
	}

	home, err := system.HomeDir()
	if err != nil {
		return fmt.Errorf("configure zshrc: %w", err)
	}
	zshrcPath := filepath.Join(home, ".zshrc")

	if _, err := os.Stat(zshrcPath); os.IsNotExist(err) {
		if dryRun {
			ui.DryRunMsg("Would create %s", zshrcPath)
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
			ui.DryRunMsg("Would set ZSH_THEME=\"%s\"", theme)
		}
		if len(plugins) > 0 {
			ui.DryRunMsg("Would set plugins=(%s)", strings.Join(plugins, " "))
		}
		return nil
	}

	if theme == "" && len(plugins) == 0 {
		return nil
	}

	return patchZshrcBlock(zshrcPath, theme, plugins, dryRun)
}
