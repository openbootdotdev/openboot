package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func BuildTestBinary(t *testing.T) string {
	t.Helper()

	// If a pre-built binary exists at project root (e.g. CI release artifact),
	// use it instead of rebuilding from source.
	projectRoot := findProjectRoot(t)
	prebuilt := filepath.Join(projectRoot, "openboot")
	if info, err := os.Stat(prebuilt); err == nil && !info.IsDir() {
		t.Logf("using pre-built binary: %s", prebuilt)
		return prebuilt
	}

	binaryPath := filepath.Join(t.TempDir(), "openboot")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/openboot")
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

