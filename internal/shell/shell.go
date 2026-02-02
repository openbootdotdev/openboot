package shell

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func IsOhMyZshInstalled() bool {
	home, _ := os.UserHomeDir()
	_, err := os.Stat(filepath.Join(home, ".oh-my-zsh"))
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

	home, _ := os.UserHomeDir()
	zshrcPath := filepath.Join(home, ".zshrc")
	os.Remove(zshrcPath)

	return nil
}

func ConfigureZshrc(dryRun bool) error {
	home, _ := os.UserHomeDir()
	zshrcPath := filepath.Join(home, ".zshrc")

	additions := `
# OpenBoot additions
export PATH="$HOME/.local/bin:$PATH"

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

	if dryRun {
		fmt.Println("[DRY-RUN] Would add to .zshrc:")
		fmt.Println(additions)
		return nil
	}

	f, err := os.OpenFile(zshrcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open .zshrc: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(additions); err != nil {
		return fmt.Errorf("failed to write to .zshrc: %w", err)
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
