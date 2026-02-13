package cleaner

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
