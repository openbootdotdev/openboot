package shell

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/openbootdotdev/openboot/internal/system"
)

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

	script := `sh -c "$(curl -fsSL https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh)" "" --unattended`
	cmd := exec.Command("bash", "-c", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	home, err := system.HomeDir()
	if err != nil {
		return err
	}
	zshrcPath := filepath.Join(home, ".zshrc")
	os.Remove(zshrcPath)

	return nil
}

const openbootZshrcSentinel = "# OpenBoot additions"

const openbootZshrcBlock = `
# OpenBoot additions
# Homebrew (must come before /usr/bin)
if [ -f /opt/homebrew/bin/brew ]; then
  eval "$(/opt/homebrew/bin/brew shellenv)"
elif [ -f /usr/local/bin/brew ]; then
  eval "$(/usr/local/bin/brew shellenv)"
fi
export PATH="$HOME/.openboot/bin:$HOME/.local/bin:$PATH"

# Modern CLI aliases
alias ls="eza --icons"
alias ll="eza -la --icons"
alias cat="bat"
alias find="fd"
alias grep="rg"
alias top="btop"

# Git aliases
alias gs="git status"
alias gd="git diff"
alias gl="lazygit"

# Zoxide (smart cd)
eval "$(zoxide init zsh)"

# fzf integration
[ -f ~/.fzf.zsh ] && source ~/.fzf.zsh
`

func ConfigureZshrc(dryRun bool) error {
	home, err := system.HomeDir()
	if err != nil {
		return err
	}
	zshrcPath := filepath.Join(home, ".zshrc")

	if dryRun {
		fmt.Println("[DRY-RUN] Would add to .zshrc:")
		fmt.Print(openbootZshrcBlock)
		return nil
	}

	existing, _ := os.ReadFile(zshrcPath)
	if strings.Contains(string(existing), openbootZshrcSentinel) {
		return nil
	}

	f, err := os.OpenFile(zshrcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open .zshrc: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(openbootZshrcBlock); err != nil {
		return fmt.Errorf("write .zshrc: %w", err)
	}

	return nil
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
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

const restoreBlockStart = "# >>> OpenBoot-Restore"
const restoreBlockEnd = "# <<< OpenBoot-Restore"

var restoreBlockRe = regexp.MustCompile(`(?s)# >>> OpenBoot-Restore\n.*?# <<< OpenBoot-Restore\n?`)

func buildRestoreBlock(theme string, plugins []string) string {
	var sb strings.Builder
	sb.WriteString(restoreBlockStart + "\n")
	if theme != "" {
		sb.WriteString(fmt.Sprintf("ZSH_THEME=\"%s\"\n", theme))
	}
	if len(plugins) > 0 {
		sb.WriteString(fmt.Sprintf("plugins=(%s)\n", strings.Join(plugins, " ")))
	}
	sb.WriteString(restoreBlockEnd + "\n")
	return sb.String()
}

var (
	looseThemeRe   = regexp.MustCompile(`(?m)^ZSH_THEME="[^"]*"\n?`)
	loosePluginsRe = regexp.MustCompile(`(?m)^plugins=\([^)]*\)\n?`)
)

func patchZshrcBlock(zshrcPath, theme string, plugins []string) error {
	raw, err := os.ReadFile(zshrcPath)
	if err != nil {
		return fmt.Errorf("read .zshrc: %w", err)
	}

	block := buildRestoreBlock(theme, plugins)
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

	if err := os.WriteFile(zshrcPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write .zshrc: %w", err)
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
		return err
	}
	zshrcPath := filepath.Join(home, ".zshrc")

	if _, err := os.Stat(zshrcPath); os.IsNotExist(err) {
		if dryRun {
			fmt.Printf("[DRY-RUN] Would create %s\n", zshrcPath)
			return nil
		}
		template := fmt.Sprintf(`export ZSH="$HOME/.oh-my-zsh"
ZSH_THEME="%s"
plugins=(%s)
source $ZSH/oh-my-zsh.sh
`, theme, strings.Join(plugins, " "))
		if err := os.WriteFile(zshrcPath, []byte(template), 0644); err != nil {
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
