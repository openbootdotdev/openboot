package macos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// makeFakeApp creates an empty directory pretending to be an .app bundle.
// Returns the absolute path.
func makeFakeApp(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir fake app: %v", err)
	}
	return dir
}

func TestSetLoginItems_DryRunDeleteThenMake(t *testing.T) {
	maccyPath := makeFakeApp(t, "Maccy.app")
	bdPath := makeFakeApp(t, "BetterDisplay.app")
	out := captureStdout(t, func() {
		err := SetLoginItems([]LoginItem{
			{Name: "Maccy", Path: maccyPath},
			{Name: "BetterDisplay", Path: bdPath, Hidden: true},
		}, true /* dryRun */)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "delete every login item")
	assert.Contains(t, out, "make new login item")
	assert.Contains(t, out, "Maccy")
	assert.Contains(t, out, "BetterDisplay")
	assert.True(t,
		strings.Contains(out, "hidden:true") || strings.Contains(out, "hidden: true"),
		"hidden flag missing from dry-run output: %s", out)
}

func TestSetLoginItems_DryRunEmptyClears(t *testing.T) {
	out := captureStdout(t, func() {
		err := SetLoginItems([]LoginItem{}, true)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "delete every login item")
}

func TestSetLoginItems_DryRunMissingPathSkipped(t *testing.T) {
	calcPath := makeFakeApp(t, "Calculator.app")
	out := captureStdout(t, func() {
		err := SetLoginItems([]LoginItem{
			{Name: "Ghost", Path: "/Applications/Ghost123Nope.app"},
			{Name: "Calculator", Path: calcPath},
		}, true)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Ghost123Nope.app")
	assert.Contains(t, out, "Calculator.app")
	makeCount := strings.Count(out, "make new login item")
	assert.Equal(t, 1, makeCount, "only Calculator should be created")
}
