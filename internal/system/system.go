package system

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
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
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func RunCommandSilent(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func InstallHomebrew() error {
	script := `/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`
	cmd := exec.Command("bash", "-c", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	tty, opened := OpenTTY()
	if opened {
		defer tty.Close()
	}
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
	f.Close()
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
