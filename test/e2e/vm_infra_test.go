//go:build e2e && vm

package e2e

import (
	"strings"
	"testing"

	"github.com/openbootdotdev/openboot/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVM_Infra validates the VM infrastructure pipeline:
// clone OCI image → boot → SSH → run commands → destroy.
// This must pass before any other VM test can be trusted.
func TestVM_Infra(t *testing.T) {
	vm := testutil.NewTartVM(t)

	t.Run("echo", func(t *testing.T) {
		output, err := vm.Run("echo hello-from-vm")
		require.NoError(t, err)
		assert.Contains(t, strings.TrimSpace(output), "hello-from-vm")
	})

	t.Run("macos_version", func(t *testing.T) {
		output, err := vm.Run("sw_vers --productVersion")
		require.NoError(t, err)
		version := strings.TrimSpace(output)
		t.Logf("macOS: %s", version)
		assert.True(t, len(version) > 0)
	})

	t.Run("arch", func(t *testing.T) {
		output, err := vm.Run("uname -m")
		require.NoError(t, err)
		assert.Contains(t, output, "arm64")
	})

	t.Run("no_brew_preinstalled", func(t *testing.T) {
		output, err := vm.Run("which brew 2>/dev/null || echo no-brew")
		require.NoError(t, err)
		assert.Contains(t, output, "no-brew", "fresh VM should not have Homebrew")
	})

	t.Run("no_openboot_preinstalled", func(t *testing.T) {
		output, err := vm.Run("which openboot 2>/dev/null || echo no-openboot")
		require.NoError(t, err)
		assert.Contains(t, output, "no-openboot")
	})

	t.Run("curl_available", func(t *testing.T) {
		_, err := vm.Run("curl --version")
		require.NoError(t, err, "curl must be available for install.sh")
	})

	t.Run("git_available", func(t *testing.T) {
		_, err := vm.Run("git --version")
		require.NoError(t, err, "git must be available")
	})

	t.Run("copy_file", func(t *testing.T) {
		// Verify scp works by copying a small file
		tmpFile := t.TempDir() + "/test.txt"
		require.NoError(t, writeFile(tmpFile, "vm-scp-test"))

		err := vm.CopyFile(tmpFile, "/tmp/test.txt")
		require.NoError(t, err)

		output, err := vm.Run("cat /tmp/test.txt")
		require.NoError(t, err)
		assert.Contains(t, output, "vm-scp-test")
	})
}
