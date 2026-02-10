//go:build integration

package integration

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_ParseRealBrewList(t *testing.T) {
	// Given: brew is installed and has packages
	output, err := exec.Command("brew", "list", "--formula", "-1").Output()
	require.NoError(t, err, "brew should be installed on test system")

	outStr := string(output)
	require.NotEmpty(t, outStr, "brew should have at least some packages")

	// When: we parse the output
	lines := strings.Split(strings.TrimSpace(outStr), "\n")

	// Then: each line should be a clean package name
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		assert.NotContains(t, line, "/", "package name should not contain path: %s", line)

		assert.Regexp(t, `^[a-zA-Z0-9][a-zA-Z0-9@._-]*$`, line,
			"package name should be valid brew identifier: %s", line)
	}

	t.Logf("Successfully parsed %d brew packages", len(lines))
}

func TestIntegration_BrewInfoFormatting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping brew info test in short mode")
	}

	// Given: we query info for a common package
	testPkg := "git"
	output, err := exec.Command("brew", "info", testPkg, "--json=v2").Output()
	require.NoError(t, err, "brew info should succeed for %s", testPkg)

	outStr := string(output)

	// Then: output should be valid JSON
	assert.Contains(t, outStr, "\"name\"", "brew info JSON should contain name field")
	assert.Contains(t, outStr, "\"versions\"", "brew info JSON should contain versions field")
	assert.True(t, strings.HasPrefix(outStr, "{") || strings.HasPrefix(outStr, "["),
		"brew info --json should output valid JSON")
}

func TestIntegration_BrewOutdatedFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping brew outdated test in short mode")
	}

	// Given: we check for outdated packages
	output, err := exec.Command("brew", "outdated", "--json=v2").Output()

	// Then: command should succeed (even if no outdated packages)
	require.NoError(t, err, "brew outdated should succeed")

	outStr := string(output)

	// JSON output should be parseable
	assert.True(t, strings.HasPrefix(outStr, "{") || strings.HasPrefix(outStr, "["),
		"brew outdated --json should output valid JSON")

	t.Logf("Brew outdated output length: %d bytes", len(outStr))
}

func TestIntegration_BrewSearchFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping brew search test in short mode")
	}

	// Given: we search for a package
	output, err := exec.Command("brew", "search", "git").Output()
	require.NoError(t, err, "brew search should succeed")

	outStr := string(output)

	// Then: output should contain package names
	assert.Contains(t, outStr, "git", "search results should contain query term")

	lines := strings.Split(strings.TrimSpace(outStr), "\n")
	assert.Greater(t, len(lines), 0, "search should return results")
}

func TestIntegration_ParseBrewCaskList(t *testing.T) {
	// Given: we list cask packages
	output, err := exec.Command("brew", "list", "--cask", "-1").Output()
	require.NoError(t, err, "brew list --cask should succeed")

	outStr := string(output)

	if strings.TrimSpace(outStr) == "" {
		t.Skip("no cask packages installed, skipping cask format test")
	}

	// When: we parse cask output
	lines := strings.Split(strings.TrimSpace(outStr), "\n")

	// Then: each line should be a clean cask name
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		assert.NotContains(t, line, "/", "cask name should not contain path: %s", line)

		assert.Regexp(t, `^[a-zA-Z0-9][a-zA-Z0-9@._-]*$`, line,
			"cask name should be valid brew identifier: %s", line)
	}

	t.Logf("Successfully parsed %d brew cask packages", len(lines))
}

func TestIntegration_BrewErrorParsing(t *testing.T) {
	// Given: we try to install a non-existent package
	cmd := exec.Command("brew", "install", "this-package-does-not-exist-xyz123")
	output, err := cmd.CombinedOutput()

	// Then: brew should return error
	assert.Error(t, err, "brew should fail for non-existent package")

	outStr := string(output)

	// Error output should contain helpful information
	assert.True(t,
		strings.Contains(outStr, "Error") ||
			strings.Contains(outStr, "error") ||
			strings.Contains(outStr, "No available formula") ||
			strings.Contains(outStr, "not found"),
		"error output should indicate package not found: %s", outStr)
}

func TestContract_BrewListOutputFormat(t *testing.T) {
	// Contract test: verify brew list output format hasn't changed

	// Given: we run brew list
	output, err := exec.Command("brew", "list", "--formula", "-1").Output()
	require.NoError(t, err, "brew should be installed")

	outStr := string(output)
	lines := strings.Split(strings.TrimSpace(outStr), "\n")

	// Then: format should match expectations
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		assert.NotContains(t, line, "\t", "CONTRACT: brew list -1 outputs one package per line")
		assert.NotContains(t, line, " ", "CONTRACT: package names do not contain spaces")
		assert.NotContains(t, line, "/", "CONTRACT: output does not contain paths")
	}

	t.Logf("✓ Contract verified: brew list -1 format is stable (%d packages)", len(lines))
}

func TestContract_BrewInfoJSONStructure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping contract test in short mode")
	}

	// Contract test: verify brew info --json=v2 structure

	// Given: we query info for git
	output, err := exec.Command("brew", "info", "git", "--json=v2").Output()
	require.NoError(t, err, "brew info should succeed")

	outStr := string(output)

	// Then: JSON structure should contain expected fields
	assert.Contains(t, outStr, "\"formulae\"", "CONTRACT: should contain formulae array")
	assert.Contains(t, outStr, "\"name\"", "CONTRACT: should contain name field")
	assert.Contains(t, outStr, "\"versions\"", "CONTRACT: should contain versions field")

	t.Logf("✓ Contract verified: brew info --json=v2 structure is stable")
}

func TestIntegration_BrewErrorMessageFormat(t *testing.T) {
	// Given: various error scenarios, test error message parsing

	testCases := []struct {
		name    string
		cmd     []string
		wantErr bool
	}{
		{
			name:    "invalid package",
			cmd:     []string{"brew", "install", "nonexistent-package-xyz"},
			wantErr: true,
		},
		{
			name:    "invalid command",
			cmd:     []string{"brew", "invalidcommand"},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(tc.cmd[0], tc.cmd[1:]...)
			output, err := cmd.CombinedOutput()

			if tc.wantErr {
				assert.Error(t, err)
				outStr := string(output)

				hasErrorInOutput := strings.Contains(strings.ToLower(outStr), "error") ||
					strings.Contains(outStr, "Error") ||
					strings.Contains(outStr, "not found") ||
					strings.Contains(outStr, "No available formula")

				assert.True(t, hasErrorInOutput || err != nil,
					"should detect brew error in: %s", outStr)
			}
		})
	}
}
