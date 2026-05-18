//go:build e2e && !vm

// Basic binary behavior tests that require the compiled binary but no system
// state or VM — pure argument validation and version checks.

package e2e

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/testutil"
)

// TestSmoke_VersionMatchesBuild verifies the compiled binary reports a version.
func TestSmoke_VersionMatchesBuild(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	home := t.TempDir()

	stdout, stderr, err := runBinary(t, binary, isolatedEnv(home, ""), "version")
	output := stdout + stderr
	require.NoError(t, err, "version command should succeed")
	assert.Contains(t, output, "OpenBoot v", "version output should contain version prefix")
}

// TestE2E_InvalidPreset verifies that an unrecognised preset name causes a
// non-zero exit and an error message mentioning the bad value.
// This is pure argument validation — the binary exits before touching any
// system state, so no VM is needed.
func TestE2E_InvalidPreset(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	home := t.TempDir()

	stdout, stderr, err := runBinary(t, binary, isolatedEnv(home, ""),
		"install", "--preset", "invalid-preset-xyz", "--dry-run", "--silent")
	output := stdout + stderr
	assert.Error(t, err, "invalid preset should cause command to fail")
	assert.True(t,
		strings.Contains(output, "invalid") || strings.Contains(output, "unknown") || strings.Contains(output, "error"),
		"error output should mention invalid preset, got: %s", output)
}
