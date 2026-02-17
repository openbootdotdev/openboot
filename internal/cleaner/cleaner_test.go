package cleaner

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
