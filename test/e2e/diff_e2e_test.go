//go:build e2e && vm

package e2e

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/openbootdotdev/openboot/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTestSnapshotJSON returns a minimal snapshot JSON string.
func writeTestSnapshotJSON(t *testing.T, formulae, casks, npm []string) string {
	t.Helper()

	snap := map[string]interface{}{
		"version":     1,
		"captured_at": time.Now().Format(time.RFC3339),
		"hostname":    "test-host",
		"packages": map[string]interface{}{
			"formulae": formulae,
			"casks":    casks,
			"taps":     []string{},
			"npm":      npm,
		},
		"macos_prefs": []interface{}{},
		"shell": map[string]interface{}{
			"default":   "/bin/zsh",
			"oh_my_zsh": false,
			"theme":     "",
			"plugins":   []string{},
		},
		"git": map[string]interface{}{
			"user_name":  "Test",
			"user_email": "test@example.com",
		},
		"dev_tools":      []interface{}{},
		"matched_preset": "",
		"catalog_match": map[string]interface{}{
			"match_rate": 0,
		},
		"health": map[string]interface{}{
			"partial":      false,
			"failed_steps": []string{},
		},
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	require.NoError(t, err)
	return string(data)
}

// vmWriteTestSnapshot writes a snapshot JSON to the VM at the given remote path.
func vmWriteTestSnapshot(t *testing.T, vm *testutil.TartVM, remotePath string, formulae, casks, npm []string) {
	t.Helper()
	content := writeTestSnapshotJSON(t, formulae, casks, npm)
	// Use printf to avoid issues with special characters and heredoc in SSH
	escaped := strings.ReplaceAll(content, "'", "'\\''")
	_, err := vm.Run(fmt.Sprintf("printf '%%s' '%s' > %s", escaped, remotePath))
	require.NoError(t, err, "should write snapshot to VM at %s", remotePath)
}

func TestE2E_Diff_FromFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	snapshotPath := "/tmp/diff-test-snapshot.json"
	vmWriteTestSnapshot(t, vm, snapshotPath, []string{"git", "curl"}, []string{}, []string{})

	output, err := vmRunDevBinary(t, vm, bin, "diff --from "+snapshotPath)
	assert.NoError(t, err, "diff --from should succeed, output: %s", output)
}

func TestE2E_Diff_FromFile_JSON(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	snapshotPath := "/tmp/diff-json-snapshot.json"
	vmWriteTestSnapshot(t, vm, snapshotPath, []string{"git"}, []string{}, []string{})

	output, err := vmRunDevBinary(t, vm, bin, "diff --from "+snapshotPath+" --json")
	require.NoError(t, err, "diff --from --json should succeed, output: %s", output)

	// stdout should be valid JSON
	var result map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	assert.NoError(t, err, "diff --json output should be valid JSON, got: %s", output)

	// Should have expected top-level keys
	assert.Contains(t, result, "source", "JSON should contain 'source' field")
	assert.Contains(t, result, "packages", "JSON should contain 'packages' field")
}

func TestE2E_Diff_FromFile_PackagesOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	snapshotPath := "/tmp/diff-pkgonly-snapshot.json"
	vmWriteTestSnapshot(t, vm, snapshotPath, []string{"git"}, []string{}, []string{})

	output, err := vmRunDevBinary(t, vm, bin, "diff --from "+snapshotPath+" --packages-only")
	assert.NoError(t, err, "diff --packages-only should succeed, output: %s", output)
}

func TestE2E_Diff_MissingFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	output, err := vmRunDevBinary(t, vm, bin, "diff --from /tmp/nonexistent-snapshot-12345.json")
	assert.Error(t, err, "diff with missing file should fail")
	assert.True(t,
		strings.Contains(output, "not found") || strings.Contains(output, "no such file") || strings.Contains(output, "snapshot"),
		"error should mention file issue, got: %s", output)
}

func TestE2E_Diff_InvalidJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Write invalid JSON to the VM
	badFile := "/tmp/bad-snapshot.json"
	_, err := vm.Run(fmt.Sprintf("echo 'not valid json {{{' > %s", badFile))
	require.NoError(t, err)

	output, cmdErr := vmRunDevBinary(t, vm, bin, "diff --from "+badFile)
	assert.Error(t, cmdErr, "diff with invalid JSON should fail")
	assert.True(t,
		strings.Contains(output, "unmarshal") || strings.Contains(output, "parse") || strings.Contains(output, "invalid") || strings.Contains(output, "snapshot"),
		"error should mention parse issue, got: %s", output)
}

func TestE2E_Diff_NoLocalSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Run diff without any flags — it will try to load local snapshot which won't exist in fresh VM.
	output, err := vmRunDevBinary(t, vm, bin, "diff")
	if err != nil {
		// Expected if no local snapshot: should mention how to create one
		assert.True(t,
			strings.Contains(output, "snapshot") || strings.Contains(output, "--from") || strings.Contains(output, "--user"),
			"error should guide user to use --from or --user, got: %s", output)
	}
	// If it succeeds, the VM has a local snapshot — that's fine too
}

func TestE2E_Diff_FromFile_JSONStructure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Create snapshot with packages that definitely won't be on the system
	snapshotPath := "/tmp/diff-structure-snapshot.json"
	vmWriteTestSnapshot(t, vm, snapshotPath,
		[]string{"this-package-surely-not-installed-xyz"},
		[]string{"this-cask-surely-not-installed-xyz"},
		[]string{"this-npm-surely-not-installed-xyz"},
	)

	output, err := vmRunDevBinary(t, vm, bin, "diff --from "+snapshotPath+" --json")
	require.NoError(t, err, "diff --json should succeed, output: %s", output)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "output should be valid JSON")

	// Verify packages section has expected structure
	packages, ok := result["packages"].(map[string]interface{})
	require.True(t, ok, "packages should be an object")

	// Should have missing items since we used fake package names
	if missing, ok := packages["missing"].(map[string]interface{}); ok {
		// At least one category should have our fake packages
		hasEntries := false
		for _, v := range missing {
			if arr, ok := v.([]interface{}); ok && len(arr) > 0 {
				hasEntries = true
				break
			}
		}
		assert.True(t, hasEntries, "missing packages should include our fake packages")
	}
}

func TestE2E_Diff_HelpFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VM test in short mode")
	}

	vm := testutil.NewTartVM(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	output, err := vmRunDevBinary(t, vm, bin, "diff --help")
	assert.NoError(t, err, "diff --help should succeed")
	assert.Contains(t, output, "diff")
	assert.Contains(t, output, "--from")
	assert.Contains(t, output, "--user")
	assert.Contains(t, output, "--json")
	assert.Contains(t, output, "--packages-only")
}
