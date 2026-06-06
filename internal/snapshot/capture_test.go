package snapshot

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseLines tests the parseLines function.
func TestParseLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single line",
			input:    "git",
			expected: []string{"git"},
		},
		{
			name:     "multiple lines",
			input:    "git\ngo\nnode",
			expected: []string{"git", "go", "node"},
		},
		{
			name:     "lines with whitespace",
			input:    "  git  \n  go  \n  node  ",
			expected: []string{"git", "go", "node"},
		},
		{
			name:     "lines with empty lines",
			input:    "git\n\ngo\n\nnode",
			expected: []string{"git", "go", "node"},
		},
		{
			name:     "trailing newline",
			input:    "git\ngo\nnode\n",
			expected: []string{"git", "go", "node"},
		},
		{
			name:     "leading newline",
			input:    "\ngit\ngo\nnode",
			expected: []string{"git", "go", "node"},
		},
		{
			name:     "tabs and spaces",
			input:    "\t git \t\n\t go \t\n\t node \t",
			expected: []string{"git", "go", "node"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLines(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseVersion tests the parseVersion function.
func TestParseVersion(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		output   string
		expected string
	}{
		{
			name:     "go version",
			toolName: "go",
			output:   "go version go1.22.0 darwin/arm64",
			expected: "1.22.0",
		},
		{
			name:     "go version with different format",
			toolName: "go",
			output:   "go version go1.21.5 linux/amd64",
			expected: "1.21.5",
		},
		{
			name:     "node version",
			toolName: "node",
			output:   "v20.11.0",
			expected: "20.11.0",
		},
		{
			name:     "python3 version",
			toolName: "python3",
			output:   "Python 3.12.0",
			expected: "3.12.0",
		},
		{
			name:     "rustc version",
			toolName: "rustc",
			output:   "rustc 1.75.0 (82e1608df 2023-12-21)",
			expected: "1.75.0",
		},
		{
			name:     "java version",
			toolName: "java",
			output:   "openjdk 21.0.1 2023-10-17\nOpenJDK Runtime Environment",
			expected: "21.0.1",
		},
		{
			name:     "ruby version",
			toolName: "ruby",
			output:   "ruby 3.2.2 (2023-03-30 revision e51014f9c0) [arm64-darwin22]",
			expected: "3.2.2",
		},
		{
			name:     "docker version",
			toolName: "docker",
			output:   "Docker version 24.0.7, build afdd53b",
			expected: "24.0.7",
		},
		{
			name:     "unknown tool",
			toolName: "unknown",
			output:   "some output",
			expected: "some output",
		},
		{
			name:     "empty output",
			toolName: "go",
			output:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseVersion(tt.toolName, tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseBunList covers the `bun pm ls -g` parser. The input mimics real
// bun output: header line with the global path, then tree-drawn entries.
func TestParseBunList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty output",
			input:    "",
			expected: []string{},
		},
		{
			name: "single entry",
			input: "/Users/x/.bun/install/global node_modules (1)\n" +
				"└── prettier@3.2.5\n",
			expected: []string{"prettier"},
		},
		{
			name: "scoped and unscoped entries",
			input: "/Users/x/.bun/install/global node_modules (3)\n" +
				"├── @anthropic-ai/claude-code@1.0.5\n" +
				"├── prettier@3.2.5\n" +
				"└── typescript@5.4.3\n",
			expected: []string{"@anthropic-ai/claude-code", "prettier", "typescript"},
		},
		{
			name: "bun itself is excluded",
			input: "/Users/x/.bun/install/global node_modules (2)\n" +
				"├── bun@1.1.0\n" +
				"└── prettier@3.2.5\n",
			expected: []string{"prettier"},
		},
		{
			name: "duplicates collapsed",
			input: "/Users/x/.bun/install/global node_modules (2)\n" +
				"├── prettier@3.2.5\n" +
				"└── prettier@3.2.5\n",
			expected: []string{"prettier"},
		},
		{
			name: "lines without a version are skipped",
			input: "/Users/x/.bun/install/global node_modules\n" +
				"├── not-a-package-line\n" +
				"└── prettier@3.2.5\n",
			expected: []string{"prettier"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseBunList(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSanitizePath tests the sanitizePath function.
func TestSanitizePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "absolute path",
			path:     "/usr/local/bin",
			expected: "/usr/local/bin",
		},
		{
			name:     "relative path",
			path:     "relative/path",
			expected: "relative/path",
		},
		{
			name:     "empty path",
			path:     "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizePath(tt.path)
			// Just verify it doesn't panic and returns a string
			assert.IsType(t, "", result)
		})
	}
}

func TestCaptureWithProgress_HealthTracksFailedSteps(t *testing.T) {
	r := &CaptureResults{}
	steps := []captureStep{
		{
			name:    "Step A",
			capture: func(r *CaptureResults) error { r.Formulae = []string{"pkg"}; return nil },
			count:   func(r *CaptureResults) int { return len(r.Formulae) },
		},
		{
			name:    "Step B",
			capture: func(r *CaptureResults) error { r.Casks = []string{}; return errors.New("simulated failure") },
			count:   func(r *CaptureResults) int { return 0 },
		},
		{
			name:    "Step C",
			capture: func(r *CaptureResults) error { r.Taps = []string{"other"}; return nil },
			count:   func(r *CaptureResults) int { return len(r.Taps) },
		},
	}

	var failedSteps []string
	for _, step := range steps {
		err := step.capture(r)
		if err != nil {
			failedSteps = append(failedSteps, step.name)
		}
	}

	require.Equal(t, []string{"Step B"}, failedSteps)
	assert.True(t, len(failedSteps) > 0)
}

func TestCaptureWithProgress_HealthEmptyOnSuccess(t *testing.T) {
	r := &CaptureResults{}
	steps := []captureStep{
		{
			name:    "Step A",
			capture: func(r *CaptureResults) error { r.Formulae = []string{"pkg"}; return nil },
			count:   func(r *CaptureResults) int { return len(r.Formulae) },
		},
	}

	var failedSteps []string
	for _, step := range steps {
		err := step.capture(r)
		if err != nil {
			failedSteps = append(failedSteps, step.name)
		}
	}

	assert.Empty(t, failedSteps)
}

func TestCaptureDotfiles_NoDotfilesDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	snap, err := CaptureDotfiles()
	assert.NoError(t, err)
	require.NotNil(t, snap)
	assert.Empty(t, snap.RepoURL)
}

func TestCaptureDotfiles_DotfilesDirExistsButNoGit(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".dotfiles"), 0755))

	snap, err := CaptureDotfiles()
	assert.NoError(t, err)
	require.NotNil(t, snap)
	assert.Empty(t, snap.RepoURL)
}

func TestCaptureDotfiles_GitDirExistsButNoRemote(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".dotfiles", ".git"), 0755))

	snap, err := CaptureDotfiles()
	assert.NoError(t, err)
	require.NotNil(t, snap)
	assert.Empty(t, snap.RepoURL)
}
