package system

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func HomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return home, nil
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
