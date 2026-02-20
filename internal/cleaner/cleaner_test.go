package cleaner

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/openbootdotdev/openboot/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupFakeBrew(t *testing.T, script string) {
	t.Helper()
	tmpDir := t.TempDir()
	brewPath := filepath.Join(tmpDir, "brew")
	require.NoError(t, os.WriteFile(brewPath, []byte(script), 0755))
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+originalPath)
}

func TestToSet(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected map[string]bool
	}{
		{
			name:     "empty slice",
			input:    []string{},
			expected: map[string]bool{},
		},
		{
			name:     "nil slice",
			input:    nil,
			expected: map[string]bool{},
		},
		{
			name:     "single item",
			input:    []string{"curl"},
			expected: map[string]bool{"curl": true},
		},
		{
			name:     "multiple items",
			input:    []string{"curl", "wget", "jq"},
			expected: map[string]bool{"curl": true, "wget": true, "jq": true},
		},
		{
			name:     "duplicates",
			input:    []string{"curl", "curl", "wget"},
			expected: map[string]bool{"curl": true, "wget": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toSet(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCleanResult_TotalExtra(t *testing.T) {
	tests := []struct {
		name     string
		result   CleanResult
		expected int
	}{
		{
			name:     "all empty",
			result:   CleanResult{},
			expected: 0,
		},
		{
			name: "only formulae",
			result: CleanResult{
				ExtraFormulae: []string{"curl", "wget"},
			},
			expected: 2,
		},
		{
			name: "only casks",
			result: CleanResult{
				ExtraCasks: []string{"firefox"},
			},
			expected: 1,
		},
		{
			name: "only npm",
			result: CleanResult{
				ExtraNpm: []string{"typescript", "eslint"},
			},
			expected: 2,
		},
		{
			name: "mixed",
			result: CleanResult{
				ExtraFormulae: []string{"curl"},
				ExtraCasks:    []string{"firefox", "slack"},
				ExtraNpm:      []string{"typescript"},
			},
			expected: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.result.TotalExtra())
		})
	}
}

func TestCleanResult_TotalRemoved(t *testing.T) {
	r := &CleanResult{
		RemovedFormulae: []string{"a"},
		RemovedCasks:    []string{"b", "c"},
		RemovedNpm:      []string{},
	}
	assert.Equal(t, 3, r.TotalRemoved())
}

func TestCleanResult_TotalFailed(t *testing.T) {
	r := &CleanResult{
		FailedFormulae: []string{"x"},
		FailedNpm:      []string{"y"},
	}
	assert.Equal(t, 2, r.TotalFailed())
}

func runOp(op uninstallOp, dryRun bool) error {
	var errs []error
	for _, pkg := range op.pkgs {
		if err := op.uninstallOne(pkg, dryRun); err != nil {
			*op.failed = append(*op.failed, pkg)
			errs = append(errs, err)
		} else {
			*op.removed = append(*op.removed, pkg)
		}
	}
	return errors.Join(errs...)
}

func TestExecute_AllSucceed(t *testing.T) {
	result := &CleanResult{
		ExtraFormulae: []string{"wget", "curl"},
	}

	callLog := []string{}
	op := uninstallOp{
		label: "Removing extra formulae",
		pkgs:  result.ExtraFormulae,
		uninstallOne: func(pkg string, _ bool) error {
			callLog = append(callLog, pkg)
			return nil
		},
		removed: &result.RemovedFormulae,
		failed:  &result.FailedFormulae,
	}

	require.NoError(t, runOp(op, false))
	assert.Equal(t, []string{"wget", "curl"}, result.RemovedFormulae)
	assert.Empty(t, result.FailedFormulae)
	assert.Equal(t, []string{"wget", "curl"}, callLog)
}

func TestExecute_PartialFailure(t *testing.T) {
	result := &CleanResult{
		ExtraFormulae: []string{"good-pkg", "bad-pkg", "another-good"},
	}

	op := uninstallOp{
		label: "Removing extra formulae",
		pkgs:  result.ExtraFormulae,
		uninstallOne: func(pkg string, _ bool) error {
			if pkg == "bad-pkg" {
				return errors.New("dependency conflict")
			}
			return nil
		},
		removed: &result.RemovedFormulae,
		failed:  &result.FailedFormulae,
	}

	require.Error(t, runOp(op, false))
	assert.Equal(t, []string{"good-pkg", "another-good"}, result.RemovedFormulae)
	assert.Equal(t, []string{"bad-pkg"}, result.FailedFormulae)
	assert.Equal(t, 2, result.TotalRemoved())
	assert.Equal(t, 1, result.TotalFailed())
}

func TestExecute_DryRun_PassedThrough(t *testing.T) {
	result := &CleanResult{
		ExtraNpm: []string{"typescript", "eslint"},
	}

	sawDryRun := false
	op := uninstallOp{
		label: "Removing extra npm packages",
		pkgs:  result.ExtraNpm,
		uninstallOne: func(_ string, dryRun bool) error {
			sawDryRun = dryRun
			return nil
		},
		removed: &result.RemovedNpm,
		failed:  &result.FailedNpm,
	}

	require.NoError(t, runOp(op, true))
	assert.True(t, sawDryRun)
	assert.Equal(t, []string{"typescript", "eslint"}, result.RemovedNpm)
}

func TestExecute_EmptyResult(t *testing.T) {
	result := &CleanResult{}
	err := Execute(result, false)
	assert.NoError(t, err)
	assert.Equal(t, 0, result.TotalRemoved())
	assert.Equal(t, 0, result.TotalFailed())
}

func TestExecute_DryRun_Formulae(t *testing.T) {
	result := &CleanResult{
		ExtraFormulae: []string{"wget", "curl"},
	}
	err := Execute(result, true)
	assert.NoError(t, err)
}

func TestExecute_WithFakeBrew_Success(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\nexit 0\n")
	result := &CleanResult{
		ExtraFormulae: []string{"wget"},
		ExtraCasks:    []string{"firefox"},
	}
	err := Execute(result, false)
	assert.NoError(t, err)
	assert.Contains(t, result.RemovedFormulae, "wget")
	assert.Contains(t, result.RemovedCasks, "firefox")
	assert.Empty(t, result.FailedFormulae)
	assert.Empty(t, result.FailedCasks)
}

func TestExecute_WithFakeBrew_Failure(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\necho 'Error: No such keg'\nexit 1\n")
	result := &CleanResult{
		ExtraFormulae: []string{"bad-pkg"},
	}
	err := Execute(result, false)
	assert.Error(t, err)
	assert.Contains(t, result.FailedFormulae, "bad-pkg")
	assert.Empty(t, result.RemovedFormulae)
}

func TestDiffFromLists_ExtraPackages(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\n"+
		"if [ \"$1\" = \"list\" ] && [ \"$2\" = \"--formula\" ]; then\n"+
		"  echo git\n"+
		"  echo wget\n"+
		"  exit 0\n"+
		"fi\n"+
		"if [ \"$1\" = \"list\" ] && [ \"$2\" = \"--cask\" ]; then\n"+
		"  echo firefox\n"+
		"  echo slack\n"+
		"  exit 0\n"+
		"fi\n"+
		"exit 0\n")

	result, err := DiffFromLists([]string{"git"}, []string{"firefox"}, nil, nil)
	require.NoError(t, err)
	assert.Contains(t, result.ExtraFormulae, "wget")
	assert.NotContains(t, result.ExtraFormulae, "git")
	assert.Contains(t, result.ExtraCasks, "slack")
	assert.NotContains(t, result.ExtraCasks, "firefox")
}

func TestDiffFromLists_NoExtras(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\n"+
		"if [ \"$1\" = \"list\" ] && [ \"$2\" = \"--formula\" ]; then\n"+
		"  echo git\n"+
		"  exit 0\n"+
		"fi\n"+
		"if [ \"$1\" = \"list\" ] && [ \"$2\" = \"--cask\" ]; then\n"+
		"  exit 0\n"+
		"fi\n"+
		"exit 0\n")

	result, err := DiffFromLists([]string{"git"}, nil, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, result.ExtraFormulae)
	assert.Empty(t, result.ExtraCasks)
}

func TestDiffFromLists_WithExtraTaps(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\n"+
		"if [ \"$1\" = \"list\" ] && [ \"$2\" = \"--formula\" ]; then\n"+
		"  exit 0\n"+
		"fi\n"+
		"if [ \"$1\" = \"list\" ] && [ \"$2\" = \"--cask\" ]; then\n"+
		"  exit 0\n"+
		"fi\n"+
		"if [ \"$1\" = \"tap\" ]; then\n"+
		"  echo homebrew/cask-fonts\n"+
		"  echo hashicorp/tap\n"+
		"  exit 0\n"+
		"fi\n"+
		"exit 0\n")

	result, err := DiffFromLists(nil, nil, nil, []string{"homebrew/cask-fonts"})
	require.NoError(t, err)
	assert.Contains(t, result.ExtraTaps, "hashicorp/tap", "tap not in desired list should be extra")
	assert.NotContains(t, result.ExtraTaps, "homebrew/cask-fonts", "desired tap should not be extra")
}

func TestDiffFromLists_TapsPathSkippedWhenEmpty(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\n"+
		"if [ \"$1\" = \"list\" ] && [ \"$2\" = \"--formula\" ]; then\n"+
		"  exit 0\n"+
		"fi\n"+
		"if [ \"$1\" = \"list\" ] && [ \"$2\" = \"--cask\" ]; then\n"+
		"  exit 0\n"+
		"fi\n"+
		"exit 0\n")

	result, err := DiffFromLists(nil, nil, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, result.ExtraTaps)
}

func TestCleanResult_TotalExtra_WithTaps(t *testing.T) {
	r := &CleanResult{
		ExtraFormulae: []string{"a"},
		ExtraCasks:    []string{"b"},
		ExtraNpm:      []string{"c"},
		ExtraTaps:     []string{"d", "e"},
	}
	assert.Equal(t, 5, r.TotalExtra())
}

func TestDiffFromSnapshot_ExtraPackages(t *testing.T) {
	setupFakeBrew(t, "#!/bin/sh\n"+
		"if [ \"$1\" = \"list\" ] && [ \"$2\" = \"--formula\" ]; then\n"+
		"  echo git\n"+
		"  echo ripgrep\n"+
		"  exit 0\n"+
		"fi\n"+
		"if [ \"$1\" = \"list\" ] && [ \"$2\" = \"--cask\" ]; then\n"+
		"  exit 0\n"+
		"fi\n"+
		"exit 0\n")

	snap := &snapshot.Snapshot{
		Packages: snapshot.PackageSnapshot{
			Formulae: []string{"git"},
		},
	}
	result, err := DiffFromSnapshot(snap)
	require.NoError(t, err)
	assert.Contains(t, result.ExtraFormulae, "ripgrep")
	assert.NotContains(t, result.ExtraFormulae, "git")
}
