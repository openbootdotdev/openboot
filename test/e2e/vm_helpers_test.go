//go:build e2e && vm

package e2e

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/openbootdotdev/openboot/testutil"
	"github.com/stretchr/testify/require"
)

const brewPath = "/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

// vmInstallViaBrewTap installs Homebrew and openboot via brew tap in the VM.
// This mirrors the real `curl | bash` user journey.
// Returns the installed openboot version string.
func vmInstallViaBrewTap(t *testing.T, vm *testutil.TartVM) string {
	t.Helper()

	script := strings.Join([]string{
		"export NONINTERACTIVE=1",
		fmt.Sprintf("export PATH=%q", brewPath),
		`/bin/bash -c "$(curl -fsSL https://openboot.dev/install.sh)" -- --help`,
	}, " && ")

	output, err := vm.Run(script)
	t.Logf("install output:\n%s", output)
	if err != nil {
		t.Fatalf("failed to install openboot via brew: %v", err)
	}

	version, _ := vm.Run(fmt.Sprintf("export PATH=%q && openboot version", brewPath))
	return strings.TrimSpace(version)
}

// vmInstallHomebrew installs only Homebrew in the VM (no openboot).
func vmInstallHomebrew(t *testing.T, vm *testutil.TartVM) {
	t.Helper()

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
func vmCopyDevBinary(t *testing.T, vm *testutil.TartVM) string {
	t.Helper()

	binary := testutil.BuildTestBinary(t)
	remotePath := "/tmp/openboot"

	err := vm.CopyFile(binary, remotePath)
	require.NoError(t, err, "should copy binary to VM")

	_, err = vm.Run("chmod +x " + remotePath)
	require.NoError(t, err)

	return remotePath
}

// vmRunOpenboot runs an openboot command inside the VM with standard PATH and env.
func vmRunOpenboot(t *testing.T, vm *testutil.TartVM, args string) (string, error) {
	t.Helper()
	cmd := fmt.Sprintf("export PATH=%q && openboot %s", brewPath, args)
	return vm.Run(cmd)
}

// vmRunDevBinary runs the dev binary inside the VM with standard PATH and env.
func vmRunDevBinary(t *testing.T, vm *testutil.TartVM, binaryPath, args string) (string, error) {
	t.Helper()
	cmd := fmt.Sprintf("export PATH=%q && %s %s", brewPath, binaryPath, args)
	return vm.Run(cmd)
}

// vmRunOpenbootWithGit runs openboot with git identity env vars set.
func vmRunOpenbootWithGit(t *testing.T, vm *testutil.TartVM, args string) (string, error) {
	t.Helper()
	env := map[string]string{
		"PATH":               brewPath,
		"OPENBOOT_GIT_NAME":  "E2E Test User",
		"OPENBOOT_GIT_EMAIL": "e2e@openboot.test",
	}
	return vm.RunWithEnv(env, "openboot "+args)
}

// vmRunDevBinaryWithGit runs the dev binary with git identity env vars set.
func vmRunDevBinaryWithGit(t *testing.T, vm *testutil.TartVM, binaryPath, args string) (string, error) {
	t.Helper()
	env := map[string]string{
		"PATH":               brewPath,
		"OPENBOOT_GIT_NAME":  "E2E Test User",
		"OPENBOOT_GIT_EMAIL": "e2e@openboot.test",
	}
	return vm.RunWithEnv(env, binaryPath+" "+args)
}

// installOhMyZsh installs Oh-My-Zsh non-interactively in the VM.
func installOhMyZsh(t *testing.T, vm *testutil.TartVM) {
	t.Helper()
	script := `sh -c "$(curl -fsSL https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh)" "" --unattended`
	output, err := vm.Run(script)
	t.Logf("oh-my-zsh install: %s", output)
	require.NoError(t, err, "should install oh-my-zsh")
}

// vmBrewList returns the list of installed Homebrew formulae in the VM.
func vmBrewList(t *testing.T, vm *testutil.TartVM) []string {
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
func vmBrewCaskList(t *testing.T, vm *testutil.TartVM) []string {
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

// vmIsInstalled checks if a command is available in the VM's PATH.
func vmIsInstalled(t *testing.T, vm *testutil.TartVM, cmd string) bool {
	t.Helper()
	_, err := vm.Run(fmt.Sprintf("export PATH=%q && which %s", brewPath, cmd))
	return err == nil
}

// writeFile is a helper to write a string to a file.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// vmWriteTestSnapshot writes a minimal valid snapshot JSON to a path on the VM.
func vmWriteTestSnapshot(t *testing.T, vm *testutil.TartVM, remotePath string, formulae, casks, npm []string) {
	t.Helper()

	toJSON := func(ss []string) string {
		if len(ss) == 0 {
			return "[]"
		}
		quoted := make([]string, len(ss))
		for i, s := range ss {
			quoted[i] = fmt.Sprintf("%q", s)
		}
		return "[" + strings.Join(quoted, ",") + "]"
	}

	json := fmt.Sprintf(
		`{"version":1,"captured_at":"2026-01-01T00:00:00Z","hostname":"test-vm","packages":{"formulae":%s,"casks":%s,"taps":[],"npm":%s},"macos_prefs":[],"shell":{},"git":{},"dotfiles":{},"dev_tools":[],"matched_preset":"","catalog_match":{},"health":{"failed_steps":[],"partial":false}}`,
		toJSON(formulae), toJSON(casks), toJSON(npm),
	)

	_, err := vm.Run(fmt.Sprintf("cat > %s << 'SNAPEOF'\n%s\nSNAPEOF", remotePath, json))
	require.NoError(t, err, "failed to write test snapshot to VM")
}
