package diff

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatJSON_Structure(t *testing.T) {
	result := &DiffResult{
		Source: Source{Kind: "local", Path: "~/.openboot/snapshot.json"},
		Packages: PackageDiff{
			Formulae: ListDiff{Missing: []string{"ripgrep"}, Extra: []string{"wget"}, Common: 42},
			Casks:    ListDiff{Missing: []string{"slack"}, Common: 12},
		},
		MacOS: &MacOSDiff{
			Changed: []MacOSPrefChange{{Domain: "d", Key: "k", System: "v1", Reference: "v2"}},
		},
	}

	data, err := FormatJSON(result)
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	// Check top-level keys exist
	assert.Contains(t, parsed, "source")
	assert.Contains(t, parsed, "packages")
	assert.Contains(t, parsed, "macos")
	assert.Contains(t, parsed, "summary")

	// Check summary values
	summary := parsed["summary"].(map[string]interface{})
	assert.Equal(t, float64(2), summary["missing"]) // ripgrep + slack
	assert.Equal(t, float64(1), summary["extra"])    // wget
	assert.Equal(t, float64(1), summary["changed"])  // 1 macOS pref
}

func TestFormatJSON_NilSections(t *testing.T) {
	result := &DiffResult{
		Source: Source{Kind: "remote", Path: "alice/my-config"},
		Packages: PackageDiff{
			Formulae: ListDiff{Common: 5},
		},
	}

	data, err := FormatJSON(result)
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	// Nil sections should be omitted
	assert.NotContains(t, parsed, "macos")
	assert.NotContains(t, parsed, "dev_tools")

	summary := parsed["summary"].(map[string]interface{})
	assert.Equal(t, float64(0), summary["missing"])
	assert.Equal(t, float64(0), summary["extra"])
	assert.Equal(t, float64(0), summary["changed"])
}

func TestFormatJSON_EmptyResult(t *testing.T) {
	result := &DiffResult{
		Source: Source{Kind: "local", Path: "test.json"},
	}

	data, err := FormatJSON(result)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"source"`)
	assert.Contains(t, string(data), `"summary"`)
}

func TestFormatTerminal_NoPanic(t *testing.T) {
	// Ensure FormatTerminal doesn't panic on various inputs
	tests := []struct {
		name   string
		result *DiffResult
	}{
		{
			name:   "empty result",
			result: &DiffResult{Source: Source{Kind: "local", Path: "test.json"}},
		},
		{
			name: "nil sections",
			result: &DiffResult{
				Source: Source{Kind: "remote", Path: "user/slug"},
				Packages: PackageDiff{
					Formulae: ListDiff{Missing: []string{"git"}, Common: 1},
				},
			},
		},
		{
			name: "full result",
			result: &DiffResult{
				Source: Source{Kind: "file", Path: "/tmp/snap.json"},
				Packages: PackageDiff{
					Formulae: ListDiff{Missing: []string{"a"}, Extra: []string{"b"}, Common: 3},
				},
				MacOS: &MacOSDiff{
					Changed: []MacOSPrefChange{{Domain: "d", Key: "k", System: "v1", Reference: "v2"}},
				},
				DevTools: &DevToolDiff{
					Missing: []string{"rust"},
					Changed: []DevToolDelta{{Name: "go", System: "1.22", Reference: "1.24"}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				FormatTerminal(tt.result, false)
			})
		})
	}
}

func TestFormatTerminal_AllBranches(t *testing.T) {
	// Exercises every branch in the format functions:
	// - printMacOSSection: Missing, Extra, no-content early return
	// - printDevToolsSection: Extra, no-content early return
	// - printSource: "local" kind
	tests := []struct {
		name   string
		result *DiffResult
	}{
		{
			name: "macOS missing and extra entries",
			result: &DiffResult{
				Source: Source{Kind: "file", Path: "snap.json"},
				MacOS: &MacOSDiff{
					Missing: []MacOSPrefEntry{{Domain: "com.apple.dock", Key: "tilesize", Value: "48"}},
					Extra:   []MacOSPrefEntry{{Domain: "com.apple.finder", Key: "ShowHardDrivesOnDesktop", Value: "true"}},
				},
			},
		},
		{
			name: "macOS with no content skips section",
			result: &DiffResult{
				Source: Source{Kind: "local", Path: "test.json"},
				MacOS:  &MacOSDiff{},
			},
		},
		{
			name: "devtools extra only",
			result: &DiffResult{
				Source: Source{Kind: "local", Path: "test.json"},
				DevTools: &DevToolDiff{
					Extra: []string{"python"},
				},
			},
		},
		{
			name: "devtools with no content skips section",
			result: &DiffResult{
				Source:   Source{Kind: "local", Path: "test.json"},
				DevTools: &DevToolDiff{Common: 3},
			},
		},
		{
			name: "unknown source kind",
			result: &DiffResult{
				Source: Source{Kind: "unknown", Path: "somewhere"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				FormatTerminal(tt.result, false)
			})
		})
	}
}

func TestFormatTerminal_PackagesOnly(t *testing.T) {
	result := &DiffResult{
		Source: Source{Kind: "local", Path: "test.json"},
		MacOS: &MacOSDiff{
			Changed: []MacOSPrefChange{{Domain: "d", Key: "k", System: "v1", Reference: "v2"}},
		},
	}

	// Should not panic even with packagesOnly=true (skips macos/devtools)
	assert.NotPanics(t, func() {
		FormatTerminal(result, true)
	})
}
