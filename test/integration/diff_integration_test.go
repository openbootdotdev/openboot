//go:build integration

package integration

import (
	"testing"

	"github.com/openbootdotdev/openboot/internal/diff"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_Diff_SystemVsItself(t *testing.T) {
	// Given: we capture the current system state
	snap, err := snapshot.Capture()
	require.NoError(t, err, "should capture system snapshot")

	// When: we diff the system against itself
	source := diff.Source{Kind: "local", Path: "self"}
	result := diff.CompareSnapshots(snap, snap, source)

	// Then: there should be no differences
	assert.False(t, result.HasChanges(), "system vs itself should have no changes")
	assert.Equal(t, 0, result.TotalMissing())
	assert.Equal(t, 0, result.TotalExtra())
	assert.Equal(t, 0, result.TotalChanged())
}

func TestIntegration_Diff_SystemVsEmptySnapshot(t *testing.T) {
	// Given: we capture the current system state and an empty reference
	snap, err := snapshot.Capture()
	require.NoError(t, err)

	empty := &snapshot.Snapshot{}

	// When: we diff system against empty
	result := diff.CompareSnapshots(snap, empty, diff.Source{Kind: "file", Path: "empty.json"})

	// Then: everything installed shows up as extra
	totalFormulae := len(snap.Packages.Formulae)
	totalCasks := len(snap.Packages.Casks)
	assert.Equal(t, totalFormulae, len(result.Packages.Formulae.Extra),
		"all formulae should be extra vs empty snapshot")
	assert.Equal(t, totalCasks, len(result.Packages.Casks.Extra),
		"all casks should be extra vs empty snapshot")
	assert.Empty(t, result.Packages.Formulae.Missing)
	assert.Empty(t, result.Packages.Casks.Missing)
}

func TestIntegration_Diff_SystemVsSubset(t *testing.T) {
	// Given: we capture the current system state
	snap, err := snapshot.Capture()
	require.NoError(t, err)

	if len(snap.Packages.Formulae) < 2 {
		t.Skip("need at least 2 installed formulae")
	}

	// Build a reference with just the first formula
	ref := &snapshot.Snapshot{
		Packages: snapshot.PackageSnapshot{
			Formulae: snap.Packages.Formulae[:1],
		},
	}

	// When: we diff
	result := diff.CompareSnapshots(snap, ref, diff.Source{Kind: "file", Path: "subset.json"})

	// Then: the one desired formula should be in common, the rest should be extra
	assert.Equal(t, 1, result.Packages.Formulae.Common)
	assert.Equal(t, len(snap.Packages.Formulae)-1, len(result.Packages.Formulae.Extra))
	assert.Empty(t, result.Packages.Formulae.Missing)
}

func TestIntegration_Diff_FormatJSON_FromRealCapture(t *testing.T) {
	// Given: a real system capture
	snap, err := snapshot.Capture()
	require.NoError(t, err)

	empty := &snapshot.Snapshot{}
	result := diff.CompareSnapshots(snap, empty, diff.Source{Kind: "file", Path: "empty.json"})

	// When: we format as JSON
	data, err := diff.FormatJSON(result)

	// Then: valid JSON is produced
	require.NoError(t, err)
	assert.Contains(t, string(data), `"source"`)
	assert.Contains(t, string(data), `"summary"`)
	assert.Contains(t, string(data), `"packages"`)
}
