package system

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

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
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func GetGitConfig(key string) string {
	output, err := RunCommandSilent("git", "config", "--global", key)
	if err != nil {
		return ""
	}
	return output
}

func GetExistingGitConfig() (name, email string) {
	name = GetGitConfig("user.name")
	email = GetGitConfig("user.email")
	return
}

func ConfigureGit(name, email string) error {
	if err := RunCommand("git", "config", "--global", "user.name", name); err != nil {
		return fmt.Errorf("failed to set git name: %w", err)
	}
	if err := RunCommand("git", "config", "--global", "user.email", email); err != nil {
		return fmt.Errorf("failed to set git email: %w", err)
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

func SSHKeyExists() bool {
	home, _ := os.UserHomeDir()
	keys := []string{
		home + "/.ssh/id_ed25519",
		home + "/.ssh/id_rsa",
	}
	for _, key := range keys {
		if _, err := os.Stat(key); err == nil {
			return true
		}
	}
	return false
}

func GetSSHKeyPath() string {
	home, _ := os.UserHomeDir()
	if _, err := os.Stat(home + "/.ssh/id_ed25519"); err == nil {
		return home + "/.ssh/id_ed25519"
	}
	if _, err := os.Stat(home + "/.ssh/id_rsa"); err == nil {
		return home + "/.ssh/id_rsa"
	}
	return ""
}

func GenerateSSHKey(email string) (string, error) {
	home, _ := os.UserHomeDir()
	sshDir := home + "/.ssh"
	keyPath := sshDir + "/id_ed25519"

	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create .ssh directory: %w", err)
	}

	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-C", email, "-f", keyPath, "-N", "")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to generate SSH key: %w", err)
	}

	pubKey, err := os.ReadFile(keyPath + ".pub")
	if err != nil {
		return "", fmt.Errorf("failed to read public key: %w", err)
	}

	return strings.TrimSpace(string(pubKey)), nil
}

func GetSSHPublicKey() (string, error) {
	keyPath := GetSSHKeyPath()
	if keyPath == "" {
		return "", fmt.Errorf("no SSH key found")
	}

	pubKey, err := os.ReadFile(keyPath + ".pub")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(pubKey)), nil
}

func ConfigureGitSSHSigning(keyPath string) error {
	if err := RunCommand("git", "config", "--global", "gpg.format", "ssh"); err != nil {
		return fmt.Errorf("failed to set gpg.format: %w", err)
	}

	if err := RunCommand("git", "config", "--global", "user.signingkey", keyPath+".pub"); err != nil {
		return fmt.Errorf("failed to set signing key: %w", err)
	}

	if err := RunCommand("git", "config", "--global", "commit.gpgsign", "true"); err != nil {
		return fmt.Errorf("failed to enable commit signing: %w", err)
	}

	return nil
}

func IsGitSigningConfigured() bool {
	signingKey := GetGitConfig("user.signingkey")
	return signingKey != ""
}
