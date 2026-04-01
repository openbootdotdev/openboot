package brew

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBrewError(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name:     "package not found",
			output:   "Error: No available formula with the name \"nonexistent\"",
			expected: "package not found",
		},
		{
			name:     "already installed",
			output:   "Warning: curl 7.85.0 is already installed and up-to-date",
			expected: "",
		},
		{
			name:     "no internet connection",
			output:   "Error: No internet connection available",
			expected: "no internet connection",
		},
		{
			name:     "connection refused",
			output:   "Error: Connection refused when trying to reach github.com",
			expected: "connection refused",
		},
		{
			name:     "connection timed out",
			output:   "Error: The request timed out",
			expected: "connection timed out",
		},
		{
			name:     "permission denied",
			output:   "Error: Permission denied when writing to /usr/local/bin",
			expected: "permission denied",
		},
		{
			name:     "disk full",
			output:   "Error: Disk full - no space left on device",
			expected: "disk full",
		},
		{
			name:     "disk full alternative",
			output:   "Error: No space left on device",
			expected: "disk full",
		},
		{
			name:     "sha256 mismatch",
			output:   "Error: SHA256 mismatch for downloaded file",
			expected: "download corrupted",
		},
		{
			name:     "dependency error",
			output:   "Error: Package depends on missing dependency",
			expected: "dependency error",
		},
		{
			name:     "unknown error with error line",
			output:   "Some output\nError: Something went wrong\nMore output",
			expected: "Error: Something went wrong",
		},
		{
			name:     "unknown error no error line",
			output:   "Some random output\nNo problem found",
			expected: "unknown error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseBrewError(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseBrewError_LongErrorLine(t *testing.T) {
	longLine := "Error: " + strings.Repeat("x", 100)
	result := parseBrewError(longLine)
	assert.LessOrEqual(t, len(result), 63)
	assert.True(t, strings.HasSuffix(result, "..."))
}

func TestParseBrewError_EmptyOutput(t *testing.T) {
	result := parseBrewError("")
	assert.Equal(t, "unknown error", result)
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected bool
	}{
		{"connection_timed_out", "connection timed out", true},
		{"connection_refused", "connection refused", true},
		{"no_internet", "no internet connection", true},
		{"download_corrupted", "download corrupted", true},
		{"already_running", "already running", true},
		{"cannot_download", "Cannot download non-corrupt file", true},
		{"signature_mismatch", "signature mismatch detected", true},
		{"package_not_found", "package not found", false},
		{"permission_denied", "permission denied", false},
		{"disk_full", "disk full", false},
		{"dependency_error", "dependency error", false},
		{"unknown_error", "unknown error", false},
		{"empty_string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableError(tt.errMsg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInstall_EmptyPackages(t *testing.T) {
	err := Install([]string{}, false)
	assert.NoError(t, err)
}

func TestInstall_DryRun(t *testing.T) {
	err := Install([]string{"git", "curl", "jq"}, true)
	assert.NoError(t, err)
}

func TestInstallCask_EmptyPackages(t *testing.T) {
	err := InstallCask([]string{}, false)
	assert.NoError(t, err)
}

func TestInstallCask_DryRun(t *testing.T) {
	err := InstallCask([]string{"firefox", "visual-studio-code"}, true)
	assert.NoError(t, err)
}

func TestInstallTaps_EmptyTaps(t *testing.T) {
	err := InstallTaps([]string{}, false)
	assert.NoError(t, err)
}

func TestInstallTaps_DryRun(t *testing.T) {
	err := InstallTaps([]string{"homebrew/cask-fonts", "hashicorp/tap"}, true)
	assert.NoError(t, err)
}

func TestInstallWithProgress_EmptyPackages(t *testing.T) {
	formulae, casks, err := InstallWithProgress([]string{}, []string{}, false)
	assert.NoError(t, err)
	assert.Empty(t, formulae)
	assert.Empty(t, casks)
}

func TestInstallWithProgress_DryRun(t *testing.T) {
	formulae, casks, err := InstallWithProgress([]string{"git", "curl"}, []string{"firefox"}, true)
	assert.NoError(t, err)
	assert.Empty(t, formulae)
	assert.Empty(t, casks)
}

func TestInstallWithProgress_DryRunReturnsNoInstalledPackages(t *testing.T) {
	formulae, casks, err := InstallWithProgress([]string{"ripgrep", "fd"}, []string{"visual-studio-code"}, true)
	assert.NoError(t, err)
	assert.Empty(t, formulae, "dry-run should not report packages as installed")
	assert.Empty(t, casks, "dry-run should not report casks as installed")
}

func TestUpdate_DryRun(t *testing.T) {
	err := Update(true)
	assert.NoError(t, err)
}

func TestCheckDiskSpace(t *testing.T) {
	gb, err := CheckDiskSpace()
	assert.NoError(t, err)
	assert.Greater(t, gb, 0.0)
}

func TestHandleFailedJobs_WithFailures(t *testing.T) {
	failed := []failedJob{
		{installJob: installJob{name: "pkg1", isCask: false}, errMsg: "not found"},
		{installJob: installJob{name: "pkg2", isCask: true}, errMsg: ""},
	}
	handleFailedJobs(failed)
}

// TestInstallWithProgress_BatchMode verifies that InstallWithProgress uses batch
// commands (brew install pkg1 pkg2...) instead of individual commands.
// This leverages Homebrew's native parallel download capability.
func TestInstallWithProgress_BatchMode(t *testing.T) {
	// Capture stdout to verify batch command format
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	formulae, casks, runErr := InstallWithProgress(
		[]string{"git", "curl", "wget"},
		[]string{"firefox", "chrome"},
		true,
	)

	w.Close()
	os.Stdout = oldStdout

	var buf strings.Builder
	_, copyErr := buf.ReadFrom(r)
	require.NoError(t, copyErr)
	output := buf.String()

	assert.NoError(t, runErr)
	assert.Empty(t, formulae, "dry-run should not report installed formulae")
	assert.Empty(t, casks, "dry-run should not report installed casks")

	// Verify batch command format: all CLI packages in a single brew install
	assert.Contains(t, output, "brew install git curl wget",
		"dry-run should show a single batch brew install command for all CLI packages")
	// Verify batch cask command format
	assert.Contains(t, output, "brew install --cask firefox chrome",
		"dry-run should show a single batch brew install --cask command for all cask packages")
}

func TestExtractPackageError(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		pkg      string
		expected string
	}{
		{
			name:     "package-specific error line",
			output:   "==> Installing foo\nError: foo: no bottle available!\n==> Installing bar",
			pkg:      "foo",
			expected: "Error: foo: no bottle available!",
		},
		{
			name:     "no package-specific line falls back to parser",
			output:   "Error: No internet connection available",
			pkg:      "baz",
			expected: "no internet connection",
		},
		{
			name:     "no useful output gives batch message",
			output:   "some random output\nnothing useful here",
			pkg:      "qux",
			expected: "not installed after batch attempt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPackageError(tt.output, tt.pkg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestResolveFormulaName tests that formula aliases are resolved correctly.
// For example, "postgresql" resolves to "postgresql@18", "kubectl" to "kubernetes-cli".
// If resolution fails, returns the original name.
func TestResolveFormulaName(t *testing.T) {
	// Test with a formula that likely exists (git is very common)
	resolved := ResolveFormulaName("git")
	assert.NotEmpty(t, resolved)
	// Should return either "git" or a versioned variant
	assert.True(t, resolved == "git" || strings.Contains(resolved, "git"),
		"Should resolve git to itself or a variant, got: %s", resolved)

	// Test with a non-existent formula - should return original
	resolved = ResolveFormulaName("nonexistent-formula-xyz")
	assert.Equal(t, "nonexistent-formula-xyz", resolved,
		"Should return original name when resolution fails")
}
