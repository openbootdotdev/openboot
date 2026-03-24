package diff

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiffLists(t *testing.T) {
	tests := []struct {
		name      string
		system    []string
		reference []string
		want      ListDiff
	}{
		{
			name:      "both empty",
			system:    nil,
			reference: nil,
			want:      ListDiff{Common: 0},
		},
		{
			name:      "identical",
			system:    []string{"git", "curl", "wget"},
			reference: []string{"git", "curl", "wget"},
			want:      ListDiff{Common: 3},
		},
		{
			name:      "missing only",
			system:    []string{"git"},
			reference: []string{"git", "curl", "wget"},
			want: ListDiff{
				Missing: []string{"curl", "wget"},
				Common:  1,
			},
		},
		{
			name:      "extra only",
			system:    []string{"git", "curl", "wget"},
			reference: []string{"git"},
			want: ListDiff{
				Extra:  []string{"curl", "wget"},
				Common: 1,
			},
		},
		{
			name:      "mixed missing and extra",
			system:    []string{"git", "curl"},
			reference: []string{"git", "wget"},
			want: ListDiff{
				Missing: []string{"wget"},
				Extra:   []string{"curl"},
				Common:  1,
			},
		},
		{
			name:      "nil system",
			system:    nil,
			reference: []string{"git", "curl"},
			want: ListDiff{
				Missing: []string{"curl", "git"},
				Common:  0,
			},
		},
		{
			name:      "nil reference",
			system:    []string{"git", "curl"},
			reference: nil,
			want: ListDiff{
				Extra:  []string{"curl", "git"},
				Common: 0,
			},
		},
		{
			name:      "duplicates in system",
			system:    []string{"git", "git", "curl"},
			reference: []string{"git", "curl"},
			want:      ListDiff{Common: 2},
		},
		{
			name:      "duplicates in reference",
			system:    []string{"git", "curl"},
			reference: []string{"git", "git", "curl", "curl"},
			want:      ListDiff{Common: 2},
		},
		{
			name:      "completely disjoint",
			system:    []string{"a", "b"},
			reference: []string{"c", "d"},
			want: ListDiff{
				Missing: []string{"c", "d"},
				Extra:   []string{"a", "b"},
				Common:  0,
			},
		},
		{
			name:      "results are sorted",
			system:    []string{"zebra", "alpha"},
			reference: []string{"mango", "banana"},
			want: ListDiff{
				Missing: []string{"banana", "mango"},
				Extra:   []string{"alpha", "zebra"},
				Common:  0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DiffLists(tt.system, tt.reference)
			assert.Equal(t, tt.want.Common, got.Common, "Common count")
			assert.Equal(t, tt.want.Missing, got.Missing, "Missing items")
			assert.Equal(t, tt.want.Extra, got.Extra, "Extra items")
		})
	}
}

func TestDiffResult_HasChanges(t *testing.T) {
	t.Run("no changes", func(t *testing.T) {
		r := &DiffResult{
			Packages: PackageDiff{
				Formulae: ListDiff{Common: 5},
			},
		}
		assert.False(t, r.HasChanges())
	})

	t.Run("has missing packages", func(t *testing.T) {
		r := &DiffResult{
			Packages: PackageDiff{
				Formulae: ListDiff{Missing: []string{"wget"}, Common: 5},
			},
		}
		assert.True(t, r.HasChanges())
	})

	t.Run("has extra packages", func(t *testing.T) {
		r := &DiffResult{
			Packages: PackageDiff{
				Casks: ListDiff{Extra: []string{"slack"}, Common: 3},
			},
		}
		assert.True(t, r.HasChanges())
	})

	t.Run("has changed macOS prefs", func(t *testing.T) {
		r := &DiffResult{
			MacOS: &MacOSDiff{
				Changed: []MacOSPrefChange{{Domain: "d", Key: "k", System: "v1", Reference: "v2"}},
			},
		}
		assert.True(t, r.HasChanges())
	})
}

func TestDiffResult_Totals(t *testing.T) {
	r := &DiffResult{
		Packages: PackageDiff{
			Formulae: ListDiff{Missing: []string{"ripgrep"}, Extra: []string{"wget"}, Common: 42},
			Casks:    ListDiff{Missing: []string{"slack"}, Common: 12},
		},
		MacOS: &MacOSDiff{
			Changed: []MacOSPrefChange{{Domain: "d", Key: "k", System: "v1", Reference: "v2"}},
		},
	}

	// Missing: ripgrep + slack = 2
	assert.Equal(t, 2, r.TotalMissing())
	// Extra: wget = 1
	assert.Equal(t, 1, r.TotalExtra())
	// Changed: 1 macOS pref = 1
	assert.Equal(t, 1, r.TotalChanged())
	assert.True(t, r.HasChanges())
}

func TestDiffResult_Totals_WithDevTools(t *testing.T) {
	r := &DiffResult{
		DevTools: &DevToolDiff{
			Missing: []string{"rust"},
			Extra:   []string{"python"},
			Changed: []DevToolDelta{{Name: "go", System: "1.22", Reference: "1.24"}},
			Common:  1,
		},
		MacOS: &MacOSDiff{
			Missing: []MacOSPrefEntry{{Domain: "d", Key: "k", Value: "v"}},
			Extra:   []MacOSPrefEntry{{Domain: "d2", Key: "k2", Value: "v2"}},
		},
	}

	assert.Equal(t, 2, r.TotalMissing())  // rust + macOS missing
	assert.Equal(t, 2, r.TotalExtra())    // python + macOS extra
	assert.Equal(t, 1, r.TotalChanged())  // go version change
	assert.True(t, r.HasChanges())
}

func TestDiffResult_NilSections(t *testing.T) {
	r := &DiffResult{}
	assert.Equal(t, 0, r.TotalMissing())
	assert.Equal(t, 0, r.TotalExtra())
	assert.Equal(t, 0, r.TotalChanged())
	assert.False(t, r.HasChanges())
}
