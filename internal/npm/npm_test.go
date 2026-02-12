package npm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseNpmError(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name:     "404 not found error",
			output:   "npm ERR! 404 Not Found - GET https://registry.npmjs.org/nonexistent",
			expected: "package not found",
		},
		{
			name:     "EACCES permission denied",
			output:   "npm ERR! code EACCES\nnpm ERR! syscall open\nnpm ERR! path /usr/local/lib/node_modules",
			expected: "permission denied",
		},
		{
			name:     "ENETWORK network error",
			output:   "npm ERR! code ENETWORK\nnpm ERR! network request failed",
			expected: "network error",
		},
		{
			name:     "ENOTFOUND network error",
			output:   "npm ERR! code ENOTFOUND\nnpm ERR! network request failed",
			expected: "network error",
		},
		{
			name:     "ENOSPC disk full",
			output:   "npm ERR! code ENOSPC\nnpm ERR! syscall write\nnpm ERR! No space left on device",
			expected: "disk full",
		},
		{
			name:     "unknown error with short last line",
			output:   "npm ERR! some error occurred\nnpm ERR! install failed",
			expected: "npm ERR! install failed",
		},
		{
			name:     "empty output",
			output:   "",
			expected: "install failed",
		},
		{
			name:     "long output line",
			output:   "npm ERR! " + string(make([]byte, 150)),
			expected: "install failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseNpmError(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsNpmRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected bool
	}{
		{"network_error", "network error", true},
		{"connection_issue", "connection refused by host", true},
		{"timeout", "request timeout exceeded", true},
		{"package_not_found", "package not found", false},
		{"permission_denied", "permission denied", false},
		{"disk_full", "disk full", false},
		{"install_failed", "install failed", false},
		{"empty_string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNpmRetryableError(tt.errMsg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInstall_EmptyPackages(t *testing.T) {
	err := Install([]string{}, false)
	assert.NoError(t, err)
}

func TestInstall_DryRun(t *testing.T) {
	err := Install([]string{"typescript", "eslint", "prettier"}, true)
	assert.NoError(t, err)
}

func TestInstall_DryRun_EmptyPackages(t *testing.T) {
	err := Install([]string{}, true)
	assert.NoError(t, err)
}

func TestIsAvailable(t *testing.T) {
	result := IsAvailable()
	assert.IsType(t, true, result)
}

func TestGetNodeVersion(t *testing.T) {
	if !IsAvailable() {
		t.Skip("node not available")
	}
	ver, err := GetNodeVersion()
	assert.NoError(t, err)
	assert.Greater(t, ver, 0)
}

func TestParseNpmError_MultipleLines(t *testing.T) {
	output := "line1\nline2\nshort last"
	result := parseNpmError(output)
	assert.Equal(t, "short last", result)
}

func TestParseNpmError_WhitespaceOnly(t *testing.T) {
	result := parseNpmError("   ")
	assert.NotEmpty(t, result)
}

func TestInstall_DryRun_WithWranglerWarning(t *testing.T) {
	err := Install([]string{"wrangler", "typescript"}, true)
	assert.NoError(t, err)
}
