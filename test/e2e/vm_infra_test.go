//go:build e2e && vm

package e2e

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/testutil"
)

// TestVM_Infra sanity-checks the host the E2E suite runs on: we can shell
// out, we're on darwin/arm64, openboot isn't leaking in from a prior run,
// and the tools install.sh depends on (curl, git) are available.
func TestVM_Infra(t *testing.T) {
	vm := testutil.NewMacHost(t)

	t.Run("echo", func(t *testing.T) {
		output, err := vm.Run("echo hello-from-host")
		require.NoError(t, err)
		assert.Contains(t, strings.TrimSpace(output), "hello-from-host")
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
