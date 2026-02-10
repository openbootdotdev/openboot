//go:build integration

package integration

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_ParseRealNpmList(t *testing.T) {
	// Given: npm is installed
	_, err := exec.Command("npm", "--version").Output()
	if err != nil {
		t.Skip("npm not installed, skipping npm integration tests")
	}

	// When: we list global packages
	output, err := exec.Command("npm", "list", "-g", "--depth=0", "--json").Output()
	require.NoError(t, err, "npm list should succeed")

	outStr := string(output)

	// Then: output should be valid JSON
	var npmData map[string]interface{}
	err = json.Unmarshal([]byte(outStr), &npmData)
	require.NoError(t, err, "npm list output should be valid JSON")

	// Check for expected structure
	if deps, ok := npmData["dependencies"].(map[string]interface{}); ok {
		t.Logf("Found %d global npm packages", len(deps))

		// Verify each package has expected structure
		for name, info := range deps {
			assert.NotEmpty(t, name, "package should have a name")

			if pkgInfo, ok := info.(map[string]interface{}); ok {
				// NPM packages should have version info
				if version, hasVersion := pkgInfo["version"].(string); hasVersion {
					assert.NotEmpty(t, version, "package %s should have version", name)
				}
			}
		}
	}
}

func TestIntegration_NpmListPlainFormat(t *testing.T) {
	// Given: npm is installed
	_, err := exec.Command("npm", "--version").Output()
	if err != nil {
		t.Skip("npm not installed, skipping npm integration tests")
	}

	// When: we list packages in plain format
	output, err := exec.Command("npm", "list", "-g", "--depth=0").Output()
	require.NoError(t, err, "npm list should succeed")

	outStr := string(output)

	// Then: output should contain package names
	lines := strings.Split(outStr, "\n")
	assert.Greater(t, len(lines), 0, "should have output lines")

	t.Logf("NPM list output sample:\n%s", outStr[:min(len(outStr), 500)])
}

func TestIntegration_NpmSearchFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping npm search test in short mode")
	}

	// Given: npm is installed
	_, err := exec.Command("npm", "--version").Output()
	if err != nil {
		t.Skip("npm not installed")
	}

	// When: we search for a package
	output, err := exec.Command("npm", "search", "express", "--json").Output()
	require.NoError(t, err, "npm search should succeed")

	outStr := string(output)

	// Then: output should be valid JSON array
	var searchResults []map[string]interface{}
	err = json.Unmarshal([]byte(outStr), &searchResults)
	require.NoError(t, err, "npm search --json should output valid JSON array")

	if len(searchResults) > 0 {
		firstResult := searchResults[0]
		assert.Contains(t, firstResult, "name", "search result should have name")
		assert.Contains(t, firstResult, "version", "search result should have version")
	}
}

func TestIntegration_NpmViewFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping npm view test in short mode")
	}

	// Given: npm is installed
	_, err := exec.Command("npm", "--version").Output()
	if err != nil {
		t.Skip("npm not installed")
	}

	// When: we view package info
	output, err := exec.Command("npm", "view", "typescript", "--json").Output()
	require.NoError(t, err, "npm view should succeed")

	outStr := string(output)

	// Then: output should be valid JSON
	var viewData map[string]interface{}
	err = json.Unmarshal([]byte(outStr), &viewData)
	require.NoError(t, err, "npm view --json should output valid JSON")

	assert.Contains(t, viewData, "name", "view output should have name")
	assert.Contains(t, viewData, "version", "view output should have version")

	name, _ := viewData["name"].(string)
	assert.Equal(t, "typescript", name, "package name should match")
}

func TestContract_NpmListJSONStructure(t *testing.T) {
	// Contract test: verify npm list --json structure

	// Given: npm is installed
	_, err := exec.Command("npm", "--version").Output()
	if err != nil {
		t.Skip("npm not installed")
	}

	// When: we run npm list with JSON output
	output, err := exec.Command("npm", "list", "-g", "--depth=0", "--json").Output()
	require.NoError(t, err, "npm list should succeed")

	outStr := string(output)

	// Then: structure should match contract
	var npmData map[string]interface{}
	err = json.Unmarshal([]byte(outStr), &npmData)
	require.NoError(t, err, "should be valid JSON")

	// CONTRACT: npm list --json has these top-level fields
	assert.Contains(t, npmData, "dependencies", "should have dependencies field")
	assert.Contains(t, npmData, "name", "should have name field")

	t.Logf("✓ Contract verified: npm list --json structure is stable")
}

func TestIntegration_NpmErrorHandling(t *testing.T) {
	// Given: npm is installed
	_, err := exec.Command("npm", "--version").Output()
	if err != nil {
		t.Skip("npm not installed")
	}

	// When: we try invalid operations
	testCases := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "view non-existent package",
			args:    []string{"view", "this-package-definitely-does-not-exist-xyz123"},
			wantErr: true,
		},
		{
			name:    "invalid command",
			args:    []string{"invalidcommand"},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("npm", tc.args...)
			output, err := cmd.CombinedOutput()

			if tc.wantErr {
				assert.Error(t, err, "should fail for invalid operation")
				outStr := string(output)
				t.Logf("Error output: %s", outStr)
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
