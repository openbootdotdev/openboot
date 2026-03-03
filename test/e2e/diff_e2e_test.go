//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openbootdotdev/openboot/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTestSnapshot creates a minimal snapshot JSON file in tmpDir and returns its path.
func writeTestSnapshot(t *testing.T, tmpDir string, formulae, casks, npm []string) string {
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

	path := filepath.Join(tmpDir, "test-snapshot.json")
	err = os.WriteFile(path, data, 0644)
	require.NoError(t, err)

	return path
}

func TestE2E_Diff_FromFile(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	tmpDir := t.TempDir()

	// Create a snapshot with some known packages
	snapshotPath := writeTestSnapshot(t, tmpDir, []string{"git", "curl"}, []string{}, []string{})

	cmd := exec.Command(binary, "diff", "--from", snapshotPath)
	output, err := cmd.CombinedOutput()
	outStr := string(output)

	// diff should succeed (exit 0) — it's a read-only comparison
	assert.NoError(t, err, "diff --from should succeed, output: %s", outStr)
}

func TestE2E_Diff_FromFile_JSON(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	tmpDir := t.TempDir()

	snapshotPath := writeTestSnapshot(t, tmpDir, []string{"git"}, []string{}, []string{})

	cmd := exec.Command(binary, "diff", "--from", snapshotPath, "--json")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	require.NoError(t, err, "diff --from --json should succeed, stderr: %s", stderr.String())

	// stdout should be valid JSON
	jsonOutput := stdout.String()
	var result map[string]interface{}
	err = json.Unmarshal([]byte(jsonOutput), &result)
	assert.NoError(t, err, "diff --json output should be valid JSON, got: %s", jsonOutput)

	// Should have expected top-level keys
	assert.Contains(t, result, "source", "JSON should contain 'source' field")
	assert.Contains(t, result, "packages", "JSON should contain 'packages' field")
}

func TestE2E_Diff_FromFile_PackagesOnly(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	tmpDir := t.TempDir()

	snapshotPath := writeTestSnapshot(t, tmpDir, []string{"git"}, []string{}, []string{})

	cmd := exec.Command(binary, "diff", "--from", snapshotPath, "--packages-only")
	output, err := cmd.CombinedOutput()
	outStr := string(output)

	assert.NoError(t, err, "diff --packages-only should succeed, output: %s", outStr)
}

func TestE2E_Diff_MissingFile(t *testing.T) {
	binary := testutil.BuildTestBinary(t)

	cmd := exec.Command(binary, "diff", "--from", "/tmp/nonexistent-snapshot-12345.json")
	output, err := cmd.CombinedOutput()
	outStr := string(output)

	assert.Error(t, err, "diff with missing file should fail")
	assert.True(t,
		strings.Contains(outStr, "not found") || strings.Contains(outStr, "no such file") || strings.Contains(outStr, "snapshot"),
		"error should mention file issue, got: %s", outStr)
}

func TestE2E_Diff_InvalidJSON(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	tmpDir := t.TempDir()

	// Write invalid JSON
	badFile := filepath.Join(tmpDir, "bad-snapshot.json")
	err := os.WriteFile(badFile, []byte("not valid json {{{"), 0644)
	require.NoError(t, err)

	cmd := exec.Command(binary, "diff", "--from", badFile)
	output, cmdErr := cmd.CombinedOutput()
	outStr := string(output)

	assert.Error(t, cmdErr, "diff with invalid JSON should fail")
	assert.True(t,
		strings.Contains(outStr, "unmarshal") || strings.Contains(outStr, "parse") || strings.Contains(outStr, "invalid") || strings.Contains(outStr, "snapshot"),
		"error should mention parse issue, got: %s", outStr)
}

func TestE2E_Diff_NoLocalSnapshot(t *testing.T) {
	binary := testutil.BuildTestBinary(t)

	// Run diff without any flags — it will try to load local snapshot
	// which may or may not exist on the test machine.
	// If it doesn't exist, it should fail with a helpful message.
	cmd := exec.Command(binary, "diff")
	output, err := cmd.CombinedOutput()
	outStr := string(output)

	if err != nil {
		// Expected if no local snapshot: should mention how to create one
		assert.True(t,
			strings.Contains(outStr, "snapshot") || strings.Contains(outStr, "--from") || strings.Contains(outStr, "--user"),
			"error should guide user to use --from or --user, got: %s", outStr)
	}
	// If it succeeds, the user has a local snapshot — that's fine too
}

func TestE2E_Diff_FromFile_JSONStructure(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	tmpDir := t.TempDir()

	// Create snapshot with packages that definitely won't all be on the system
	snapshotPath := writeTestSnapshot(t, tmpDir,
		[]string{"this-package-surely-not-installed-xyz"},
		[]string{"this-cask-surely-not-installed-xyz"},
		[]string{"this-npm-surely-not-installed-xyz"},
	)

	cmd := exec.Command(binary, "diff", "--from", snapshotPath, "--json")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	require.NoError(t, err, "diff --json should succeed, stderr: %s", stderr.String())

	var result map[string]interface{}
	err = json.Unmarshal([]byte(stdout.String()), &result)
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
	binary := testutil.BuildTestBinary(t)

	cmd := exec.Command(binary, "diff", "--help")
	output, err := cmd.CombinedOutput()
	outStr := string(output)

	assert.NoError(t, err, "diff --help should succeed")
	assert.Contains(t, outStr, "diff")
	assert.Contains(t, outStr, "--from")
	assert.Contains(t, outStr, "--user")
	assert.Contains(t, outStr, "--json")
	assert.Contains(t, outStr, "--packages-only")
}
