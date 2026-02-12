package updater

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		name     string
		latest   string
		current  string
		expected bool
	}{
		{
			name:     "empty latest version",
			latest:   "",
			current:  "1.0.0",
			expected: false,
		},
		{
			name:     "same version",
			latest:   "1.0.0",
			current:  "1.0.0",
			expected: false,
		},
		{
			name:     "newer version",
			latest:   "2.0.0",
			current:  "1.0.0",
			expected: true,
		},
		{
			name:     "older version",
			latest:   "1.0.0",
			current:  "2.0.0",
			expected: false,
		},
		{
			name:     "latest with v prefix",
			latest:   "v2.0.0",
			current:  "1.0.0",
			expected: true,
		},
		{
			name:     "current with v prefix",
			latest:   "2.0.0",
			current:  "v1.0.0",
			expected: true,
		},
		{
			name:     "both with v prefix",
			latest:   "v2.0.0",
			current:  "v1.0.0",
			expected: true,
		},
		{
			name:     "same version with different prefixes",
			latest:   "v1.0.0",
			current:  "1.0.0",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNewerVersion(tt.latest, tt.current)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsHomebrewPath(t *testing.T) {
	homebrewPaths := []string{
		"/opt/homebrew/Cellar/openboot/0.21.0/bin/openboot",
		"/usr/local/Homebrew/Cellar/openboot/0.21.0/bin/openboot",
		"/opt/homebrew/bin/openboot",
		"/home/linuxbrew/.linuxbrew/Cellar/openboot/0.21.0/bin/openboot",
	}
	for _, p := range homebrewPaths {
		assert.True(t, isHomebrewPath(p), "should detect Homebrew path: %s", p)
	}

	nonHomebrewPaths := []string{
		"/usr/local/bin/openboot",
		"/Users/user/.openboot/bin/openboot",
		"/tmp/openboot",
	}
	for _, p := range nonHomebrewPaths {
		assert.False(t, isHomebrewPath(p), "should not detect as Homebrew: %s", p)
	}
}

func TestTrimVersionPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "with v prefix",
			input:    "v1.2.3",
			expected: "1.2.3",
		},
		{
			name:     "without v prefix",
			input:    "1.2.3",
			expected: "1.2.3",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "just v",
			input:    "v",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimVersionPrefix(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
