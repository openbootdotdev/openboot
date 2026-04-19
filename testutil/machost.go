//go:build e2e && vm

// MacHost runs destructive openboot E2E tests directly against the current
// macOS host — no Tart VM, no SSH. It's intended for ephemeral CI runners
// (GitHub Actions macos-latest) that can be thrown away after the run.
//
// A host refuses to activate unless CI=true or OPENBOOT_E2E_DESTRUCTIVE=1
// is set, so `go test -tags="e2e,vm"` on a developer machine is a no-op
// rather than a foot-gun.

package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// MacHost wraps the current macOS host for an E2E test.
type MacHost struct {
	t       *testing.T
	tempDir string
}

// NewMacHost returns a handle on the current host. It skips the test
// unless the environment has opted in to destructive execution.
func NewMacHost(t *testing.T) *MacHost {
	t.Helper()
	requireEphemeralHost(t)
	requireMacOS(t)

	return &MacHost{
		t:       t,
		tempDir: t.TempDir(),
	}
}

// Run executes a shell command and returns combined output.
func (h *MacHost) Run(command string) (string, error) {
	cmd := exec.Command("bash", "-c", command)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// RunWithEnv executes a command with additional environment variables.
func (h *MacHost) RunWithEnv(env map[string]string, command string) (string, error) {
	cmd := exec.Command("bash", "-c", command)
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// ExpectStep describes one interaction step: wait for a pattern, then send input.
type ExpectStep struct {
	Expect string
	Send   string
}

// RunInteractive drives a TUI command via `expect(1)`. Each step waits for
// its Expect pattern, then sends Send.
func (h *MacHost) RunInteractive(command string, steps []ExpectStep, timeoutSec int) (string, error) {
	if timeoutSec == 0 {
		timeoutSec = 300
	}

	var script strings.Builder
	fmt.Fprintf(&script, "set timeout %d\n", timeoutSec)
	fmt.Fprintf(&script, "spawn bash -c %s\n", shellescape(command))
	for _, step := range steps {
		fmt.Fprintf(&script, "expect %q\n", step.Expect)
		fmt.Fprintf(&script, "send %q\n", step.Send)
	}
	script.WriteString("expect eof\n")

	tmpFile := filepath.Join(h.tempDir, fmt.Sprintf("interact-%d.exp", time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, []byte(script.String()), 0644); err != nil {
		return "", fmt.Errorf("write expect script: %w", err)
	}

	cmd := exec.Command("expect", tmpFile)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// CopyFile copies a local file to a destination path on the same host.
// Preserved for API compatibility with the old Tart-based helper.
func (h *MacHost) CopyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	if err := os.WriteFile(dst, data, 0755); err != nil {
		return fmt.Errorf("write destination: %w", err)
	}
	return nil
}

// Destroy is a no-op — the CI runner is the sandbox.
func (h *MacHost) Destroy() {}

func shellescape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func requireEphemeralHost(t *testing.T) {
	t.Helper()
	if os.Getenv("CI") == "true" || os.Getenv("OPENBOOT_E2E_DESTRUCTIVE") == "1" {
		return
	}
	t.Skip("destructive macOS E2E tests require CI=true or OPENBOOT_E2E_DESTRUCTIVE=1")
}

func requireMacOS(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("macOS E2E tests require darwin host")
	}
}
