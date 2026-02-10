package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func BuildTestBinary(t *testing.T) string {
	tmpDir, err := os.MkdirTemp("", "openboot-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	binaryPath := filepath.Join(tmpDir, "openboot")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/openboot")

	projectRoot := findProjectRoot(t)
	cmd.Dir = projectRoot

	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to build test binary: %v", err)
	}

	return binaryPath
}

func findProjectRoot(t *testing.T) string {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatalf("could not find project root (go.mod)")
		}
		wd = parent
	}
}

type MockExecCommand struct {
	Called bool
	Args   []string
	Err    error
}

func NewMockExecCommand() *MockExecCommand {
	return &MockExecCommand{
		Called: false,
		Args:   []string{},
		Err:    nil,
	}
}

type CommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Err      error
}

func AssertCommandSuccess(t *testing.T, result CommandResult) {
	if result.Err != nil {
		t.Errorf("expected command to succeed, got error: %v", result.Err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
}

func AssertCommandFailure(t *testing.T, result CommandResult) {
	if result.ExitCode == 0 {
		t.Errorf("expected command to fail, but exit code was 0")
	}
}

func IsPackageInstalled(packageName string) bool {
	cmd := exec.Command("which", packageName)
	err := cmd.Run()
	return err == nil
}

func UninstallPackage(t *testing.T, packageName string) {
	if !IsPackageInstalled(packageName) {
		return
	}
	cmd := exec.Command("brew", "uninstall", "--force", packageName)
	if err := cmd.Run(); err != nil {
		t.Logf("warning: failed to uninstall %s: %v", packageName, err)
	}
}

func EnsurePackageNotInstalled(t *testing.T, packageName string) {
	UninstallPackage(t, packageName)
	if IsPackageInstalled(packageName) {
		t.Fatalf("failed to ensure %s is not installed", packageName)
	}
}

func GetInstalledBrewPackages() ([]string, error) {
	cmd := exec.Command("brew", "list", "--formula", "-1")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	outStr := string(output)
	lines := strings.Split(strings.TrimSpace(outStr), "\n")

	result := []string{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result, nil
}
