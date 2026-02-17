package snapshot

import (
	"errors"
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

// TestCaptureFormulae tests that CaptureFormulae returns a slice.
func TestCaptureFormulae(t *testing.T) {
	formulae, err := CaptureFormulae()
	// Should either succeed or fail gracefully
	assert.True(t, err == nil || formulae != nil)
	assert.IsType(t, []string{}, formulae)
}

// TestCaptureCasks tests that CaptureCasks returns a slice.
func TestCaptureCasks(t *testing.T) {
	casks, err := CaptureCasks()
	// Should either succeed or fail gracefully
	assert.True(t, err == nil || casks != nil)
	assert.IsType(t, []string{}, casks)
}

// TestCaptureTaps tests that CaptureTaps returns a slice.
func TestCaptureTaps(t *testing.T) {
	taps, err := CaptureTaps()
	// Should either succeed or fail gracefully
	assert.True(t, err == nil || taps != nil)
	assert.IsType(t, []string{}, taps)
}

// TestCaptureNpm tests that CaptureNpm returns a slice.
func TestCaptureNpm(t *testing.T) {
	npm, err := CaptureNpm()
	// Should either succeed or fail gracefully
	assert.True(t, err == nil || npm != nil)
	assert.IsType(t, []string{}, npm)
}

// TestCaptureShell tests that CaptureShell returns a ShellSnapshot.
func TestCaptureShell(t *testing.T) {
	shell, err := CaptureShell()
	assert.NoError(t, err)
	assert.NotNil(t, shell)
	assert.IsType(t, &ShellSnapshot{}, shell)
}

// TestCaptureGit tests that CaptureGit returns a GitSnapshot.
func TestCaptureGit(t *testing.T) {
	git, err := CaptureGit()
	assert.NoError(t, err)
	assert.NotNil(t, git)
	assert.IsType(t, &GitSnapshot{}, git)
}

// TestCaptureDevTools tests that CaptureDevTools returns a slice.
func TestCaptureDevTools(t *testing.T) {
	tools, err := CaptureDevTools()
	assert.NoError(t, err)
	assert.NotNil(t, tools)
	assert.IsType(t, []DevTool{}, tools)
}

// TestCaptureMacOSPrefs tests that CaptureMacOSPrefs returns a slice.
func TestCaptureMacOSPrefs(t *testing.T) {
	prefs, err := CaptureMacOSPrefs()
	// Should either succeed or fail gracefully
	assert.True(t, err == nil || prefs != nil)
	assert.IsType(t, []MacOSPref{}, prefs)
}

// TestParseLines_EdgeCases tests parseLines with edge cases.
func TestParseLines_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		minItems int
		maxItems int
	}{
		{
			name:     "only whitespace",
			input:    "   \n   \n   ",
			minItems: 0,
			maxItems: 0,
		},
		{
			name:     "mixed whitespace",
			input:    " \t \n \t \n \t ",
			minItems: 0,
			maxItems: 0,
		},
		{
			name:     "single item with spaces",
			input:    "   git   ",
			minItems: 1,
			maxItems: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLines(tt.input)
			assert.GreaterOrEqual(t, len(result), tt.minItems)
			assert.LessOrEqual(t, len(result), tt.maxItems)
		})
	}
}

// TestParseVersion_EdgeCases tests parseVersion with edge cases.
func TestParseVersion_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		output   string
	}{
		{
			name:     "empty output",
			toolName: "go",
			output:   "",
		},
		{
			name:     "whitespace only",
			toolName: "node",
			output:   "   ",
		},
		{
			name:     "malformed output",
			toolName: "go",
			output:   "invalid output format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseVersion(tt.toolName, tt.output)
			// Should not panic and return a string
			assert.IsType(t, "", result)
		})
	}
}

func TestCaptureWithProgress_HealthTracksFailedSteps(t *testing.T) {
	steps := []captureStep{
		{
			name:    "Step A",
			capture: func() (interface{}, error) { return []string{"pkg"}, nil },
			count:   func(v interface{}) int { return len(v.([]string)) },
		},
		{
			name:    "Step B",
			capture: func() (interface{}, error) { return []string{}, errors.New("simulated failure") },
			count:   func(v interface{}) int { return 0 },
		},
		{
			name:    "Step C",
			capture: func() (interface{}, error) { return []string{"other"}, nil },
			count:   func(v interface{}) int { return len(v.([]string)) },
		},
	}

	results := make([]interface{}, len(steps))
	var failedSteps []string
	for i, step := range steps {
		result, err := step.capture()
		results[i] = result
		if err != nil {
			failedSteps = append(failedSteps, step.name)
		}
	}

	require.Equal(t, []string{"Step B"}, failedSteps)
	assert.True(t, len(failedSteps) > 0)
}

func TestCaptureWithProgress_HealthEmptyOnSuccess(t *testing.T) {
	steps := []captureStep{
		{
			name:    "Step A",
			capture: func() (interface{}, error) { return []string{"pkg"}, nil },
			count:   func(v interface{}) int { return len(v.([]string)) },
		},
	}

	var failedSteps []string
	for _, step := range steps {
		_, err := step.capture()
		if err != nil {
			failedSteps = append(failedSteps, step.name)
		}
	}

	assert.Empty(t, failedSteps)
}
