//go:build e2e && vm

package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	ociImage       = "ghcr.io/cirruslabs/macos-sequoia-base:latest"
	vmUser         = "admin"
	vmPassword     = "admin"
	vmPrefix       = "openboot-e2e-"
	sshTimeout     = 180 * time.Second
	sshPollInterval = 5 * time.Second
)

// TartVM manages the lifecycle of a Tart virtual machine for E2E testing.
type TartVM struct {
	Name      string
	IP        string
	User      string
	sshKeyDir string
	t         *testing.T
	destroyed bool
}

// NewTartVM clones a fresh VM from the OCI base image, starts it, and waits for SSH.
// The VM is automatically destroyed when the test completes (via t.Cleanup).
func NewTartVM(t *testing.T) *TartVM {
	t.Helper()

	requireTart(t)
	requireSSHPass(t)

	name := fmt.Sprintf("%s%d", vmPrefix, time.Now().UnixNano())

	vm := &TartVM{
		Name: name,
		User: vmUser,
		t:    t,
	}

	// Always clean up, even on panic
	t.Cleanup(vm.Destroy)

	t.Logf("cloning VM %s from %s", name, ociImage)
	runCmd(t, "tart", "clone", ociImage, name)

	t.Logf("starting VM %s (headless)", name)
	startCmd := exec.Command("tart", "run", "--no-graphics", name)
	if err := startCmd.Start(); err != nil {
		t.Fatalf("failed to start VM: %v", err)
	}

	vm.waitForIP(t)
	vm.setupSSHKey(t)
	vm.waitForSSH(t)

	return vm
}

// Run executes a command inside the VM via SSH and returns combined output.
func (vm *TartVM) Run(command string) (string, error) {
	args := vm.sshArgs()
	args = append(args, vm.sshTarget(), command)

	cmd := exec.Command("ssh", args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// RunWithEnv executes a command with environment variables set.
func (vm *TartVM) RunWithEnv(env map[string]string, command string) (string, error) {
	var exports []string
	for k, v := range env {
		exports = append(exports, fmt.Sprintf("export %s=%q", k, v))
	}
	exports = append(exports, command)
	return vm.Run(strings.Join(exports, " && "))
}

// ExpectStep describes one interaction step: wait for a pattern, then send input.
type ExpectStep struct {
	Expect string // text pattern to wait for
	Send   string // text to send (e.g., "\r" for Enter, "y\r" for y+Enter)
}

// RunInteractive executes a command inside the VM using expect for TUI interaction.
// Each step waits for the Expect pattern, then sends the Send string.
// Returns the full session output.
func (vm *TartVM) RunInteractive(command string, steps []ExpectStep, timeoutSec int) (string, error) {
	if timeoutSec == 0 {
		timeoutSec = 300
	}

	keyPath := filepath.Join(vm.sshKeyDir, "id_ed25519")

	// Build expect script
	var script strings.Builder
	script.WriteString(fmt.Sprintf("set timeout %d\n", timeoutSec))
	script.WriteString(fmt.Sprintf(
		"spawn ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR %s %s\n",
		keyPath, vm.sshTarget(), shellescape(command),
	))

	for _, step := range steps {
		script.WriteString(fmt.Sprintf("expect %q\n", step.Expect))
		script.WriteString(fmt.Sprintf("send %q\n", step.Send))
	}

	script.WriteString("expect eof\n")

	// Write expect script to temp file
	tmpFile := filepath.Join(vm.sshKeyDir, "interact.exp")
	if err := os.WriteFile(tmpFile, []byte(script.String()), 0644); err != nil {
		return "", fmt.Errorf("failed to write expect script: %w", err)
	}

	cmd := exec.Command("expect", tmpFile)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// shellescape wraps a string in single quotes for shell safety.
func shellescape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// CopyFile copies a local file into the VM via scp.
func (vm *TartVM) CopyFile(localPath, remotePath string) error {
	args := vm.scpArgs()
	args = append(args, localPath, fmt.Sprintf("%s@%s:%s", vm.User, vm.IP, remotePath))

	cmd := exec.Command("scp", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("scp failed: %w, output: %s", err, string(output))
	}
	return nil
}

// Destroy stops and deletes the VM. Safe to call multiple times.
func (vm *TartVM) Destroy() {
	if vm.destroyed {
		return
	}
	vm.destroyed = true
	vm.t.Logf("destroying VM %s", vm.Name)

	stop := exec.Command("tart", "stop", vm.Name)
	_ = stop.Run() // ignore error if already stopped

	del := exec.Command("tart", "delete", vm.Name)
	if output, err := del.CombinedOutput(); err != nil {
		vm.t.Logf("warning: failed to delete VM %s: %v, output: %s", vm.Name, err, string(output))
	}
}

func (vm *TartVM) waitForIP(t *testing.T) {
	t.Helper()
	t.Logf("waiting for VM IP...")

	deadline := time.Now().Add(sshTimeout)
	for time.Now().Before(deadline) {
		cmd := exec.Command("tart", "ip", vm.Name)
		output, err := cmd.Output()
		if err == nil {
			ip := strings.TrimSpace(string(output))
			if ip != "" {
				vm.IP = ip
				t.Logf("VM IP: %s", ip)
				return
			}
		}
		time.Sleep(sshPollInterval)
	}
	t.Fatalf("timed out waiting for VM IP after %v", sshTimeout)
}

func (vm *TartVM) setupSSHKey(t *testing.T) {
	t.Helper()

	vm.sshKeyDir = t.TempDir()
	keyPath := filepath.Join(vm.sshKeyDir, "id_ed25519")

	// Generate ephemeral SSH key pair
	genCmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", keyPath, "-N", "", "-C", "openboot-vm-test")
	if output, err := genCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to generate SSH key: %v, output: %s", err, string(output))
	}

	// Copy public key to VM using sshpass for initial auth.
	// Retry because password auth may not be ready immediately after boot.
	pubKey, err := os.ReadFile(keyPath + ".pub")
	if err != nil {
		t.Fatalf("failed to read public key: %v", err)
	}

	const maxRetries = 12
	for attempt := 1; attempt <= maxRetries; attempt++ {
		copyCmd := exec.Command("sshpass", "-p", vmPassword,
			"ssh",
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"-o", "PreferredAuthentications=password",
			"-o", "PubkeyAuthentication=no",
			"-o", "ConnectTimeout=10",
			fmt.Sprintf("%s@%s", vm.User, vm.IP),
			fmt.Sprintf("mkdir -p ~/.ssh && echo %q >> ~/.ssh/authorized_keys && chmod 700 ~/.ssh && chmod 600 ~/.ssh/authorized_keys", string(pubKey)),
		)
		output, err := copyCmd.CombinedOutput()
		if err == nil {
			t.Logf("SSH key installed in VM (attempt %d)", attempt)
			return
		}
		if attempt == maxRetries {
			t.Fatalf("failed to copy SSH key to VM after %d attempts: %v, output: %s", maxRetries, err, string(output))
		}
		t.Logf("SSH key copy attempt %d failed, retrying in 5s: %s", attempt, strings.TrimSpace(string(output)))
		time.Sleep(5 * time.Second)
	}
}

func (vm *TartVM) waitForSSH(t *testing.T) {
	t.Helper()
	t.Logf("waiting for SSH readiness...")

	deadline := time.Now().Add(sshTimeout)
	for time.Now().Before(deadline) {
		args := vm.sshArgs()
		args = append(args, "-o", "ConnectTimeout=5", vm.sshTarget(), "echo ok")

		cmd := exec.Command("ssh", args...)
		output, err := cmd.CombinedOutput()
		if err == nil && strings.TrimSpace(string(output)) == "ok" {
			t.Logf("SSH is ready")
			return
		}
		time.Sleep(sshPollInterval)
	}
	t.Fatalf("timed out waiting for SSH after %v", sshTimeout)
}

func (vm *TartVM) sshArgs() []string {
	keyPath := filepath.Join(vm.sshKeyDir, "id_ed25519")
	return []string{
		"-i", keyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
	}
}

func (vm *TartVM) scpArgs() []string {
	keyPath := filepath.Join(vm.sshKeyDir, "id_ed25519")
	return []string{
		"-i", keyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
	}
}

func (vm *TartVM) sshTarget() string {
	return fmt.Sprintf("%s@%s", vm.User, vm.IP)
}

func requireTart(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tart"); err != nil {
		t.Skip("tart not found; install with: brew install cirruslabs/cli/tart")
	}
}

func requireSSHPass(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sshpass"); err != nil {
		t.Skip("sshpass not found; install with: brew install hudochenkov/sshpass/sshpass")
	}
}

func runCmd(t *testing.T, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v, output: %s", name, args, err, string(output))
	}
	return string(output)
}
