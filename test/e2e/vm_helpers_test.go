//go:build e2e && vm

package e2e

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/testutil"
)

const brewPath = "/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

// vmInstallViaBrew installs openboot via `brew tap && brew install` — no TTY required.
// Use this instead of vmInstallViaBrewTap when running over SSH without -t.
// Returns the installed openboot version string.
func vmInstallViaBrew(t *testing.T, vm *testutil.MacHost) string {
	t.Helper()
	vmInstallHomebrew(t, vm)

	script := strings.Join([]string{
		fmt.Sprintf("export PATH=%q", brewPath),
		"brew tap openbootdotdev/openboot 2>/dev/null || true",
		"brew install openboot",
	}, " && ")

	output, err := vm.Run(script)
	t.Logf("brew install openboot:\n%s", output)
	if err != nil {
		t.Fatalf("failed to install openboot via brew tap: %v", err)
	}

	version, _ := vm.Run(fmt.Sprintf("export PATH=%q && openboot version", brewPath))
	return strings.TrimSpace(version)
}

// vmInstallHomebrew ensures Homebrew is installed on the host.
// GitHub Actions macOS runners ship with Homebrew preinstalled, so this
// skips the install when brew is already on PATH and only bootstraps it
// on a genuinely bare host.
func vmInstallHomebrew(t *testing.T, vm *testutil.MacHost) {
	t.Helper()

	if _, err := vm.Run("command -v brew >/dev/null 2>&1"); err == nil {
		return
	}

	script := strings.Join([]string{
		"export NONINTERACTIVE=1",
		`/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`,
	}, " && ")

	output, err := vm.Run(script)
	t.Logf("homebrew install output:\n%s", output)
	if err != nil {
		t.Fatalf("failed to install Homebrew: %v", err)
	}
}

// vmCopyDevBinary builds the openboot binary on the host and copies it to the VM.
// Returns the remote binary path.
func vmCopyDevBinary(t *testing.T, vm *testutil.MacHost) string {
	t.Helper()

	binary := testutil.BuildTestBinary(t)
	remotePath := "/tmp/openboot"

	err := vm.CopyFile(binary, remotePath)
	require.NoError(t, err, "should copy binary to VM")

	_, err = vm.Run("chmod +x " + remotePath)
	require.NoError(t, err)

	return remotePath
}

// vmRunDevBinary runs the dev binary inside the VM with standard PATH and env.
func vmRunDevBinary(t *testing.T, vm *testutil.MacHost, binaryPath, args string) (string, error) {
	t.Helper()
	cmd := fmt.Sprintf("export PATH=%q && %s %s", brewPath, binaryPath, args)
	return vm.Run(cmd)
}

// vmRunDevBinaryWithGit runs the dev binary with git identity env vars set.
func vmRunDevBinaryWithGit(t *testing.T, vm *testutil.MacHost, binaryPath, args string) (string, error) {
	t.Helper()
	env := map[string]string{
		"PATH":               brewPath,
		"OPENBOOT_GIT_NAME":  "E2E Test User",
		"OPENBOOT_GIT_EMAIL": "e2e@openboot.test",
	}
	return vm.RunWithEnv(env, binaryPath+" "+args)
}

// installOhMyZsh installs Oh-My-Zsh non-interactively in the VM.
// Idempotent: skips if ~/.oh-my-zsh already exists.
func installOhMyZsh(t *testing.T, vm *testutil.MacHost) {
	t.Helper()
	if _, err := vm.Run("test -d ~/.oh-my-zsh"); err == nil {
		return
	}
	script := `sh -c "$(curl -fsSL https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh)" "" --unattended`
	output, err := vm.Run(script)
	t.Logf("oh-my-zsh install: %s", output)
	require.NoError(t, err, "should install oh-my-zsh")
}

// vmBrewList returns the list of installed Homebrew formulae in the VM.
func vmBrewList(t *testing.T, vm *testutil.MacHost) []string {
	t.Helper()
	output, err := vm.Run(fmt.Sprintf("export PATH=%q && brew list --formula -1 2>/dev/null", brewPath))
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// vmBrewCaskList returns the list of installed Homebrew casks in the VM.
func vmBrewCaskList(t *testing.T, vm *testutil.MacHost) []string {
	t.Helper()
	output, err := vm.Run(fmt.Sprintf("export PATH=%q && brew list --cask -1 2>/dev/null", brewPath))
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// writeFile is a helper to write a string to a file.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

