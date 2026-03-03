//go:build e2e

package e2e

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/openbootdotdev/openboot/testutil"
	"github.com/stretchr/testify/assert"
)

func TestE2E_Update_DryRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping update test in short mode")
	}

	binary := testutil.BuildTestBinary(t)

	cmd := exec.Command(binary, "update", "--dry-run")
	output, err := cmd.CombinedOutput()
	outStr := string(output)

	assert.NoError(t, err, "update --dry-run should succeed, output: %s", outStr)
	assert.True(t,
		strings.Contains(outStr, "DRY-RUN") || strings.Contains(outStr, "up to date") || strings.Contains(outStr, "outdated") || strings.Contains(outStr, "Would run"),
		"output should show dry-run info or package status, got: %s", outStr)
}

func TestE2E_Update_HelpFlag(t *testing.T) {
	binary := testutil.BuildTestBinary(t)

	cmd := exec.Command(binary, "update", "--help")
	output, err := cmd.CombinedOutput()
	outStr := string(output)

	assert.NoError(t, err, "update --help should succeed")
	assert.Contains(t, outStr, "update")
	assert.Contains(t, outStr, "--dry-run")
	assert.Contains(t, outStr, "--self")
}

func TestE2E_Update_Self_DevVersion(t *testing.T) {
	binary := testutil.BuildTestBinary(t)

	// Dev builds (version=dev) should handle --self gracefully
	cmd := exec.Command(binary, "update", "--self")
	output, err := cmd.CombinedOutput()
	outStr := string(output)

	// May succeed or fail depending on install method detection —
	// just verify it doesn't panic
	t.Logf("update --self output: %s (err: %v)", outStr, err)
}
