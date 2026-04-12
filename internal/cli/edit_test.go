package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunEdit_NotAuthenticated(t *testing.T) {
	setupTestAuth(t, false)
	t.Setenv("OPENBOOT_API_URL", "http://localhost:9999")

	err := runEdit("my-config")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not logged in")
}

func TestRunEdit_NoSlug_NoSyncSource(t *testing.T) {
	setupTestAuth(t, true)

	err := runEdit("")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no config slug")
}

func TestRunEdit_SlugFromSyncSource(t *testing.T) {
	tmpDir := setupTestAuth(t, true)
	writeSyncSource(t, tmpDir, "my-setup")

	// exec.Command("open", url) will fail in CI / test environment — that's fine,
	// we just verify the slug resolution works and the error is from "open", not auth/slug.
	err := runEdit("")

	// The only error allowed is from the "open" binary itself (not found in some envs).
	if err != nil {
		assert.Contains(t, err.Error(), "open browser")
	}
}

func TestEditCmd_CommandStructure(t *testing.T) {
	assert.Equal(t, "edit", editCmd.Use)
	assert.NotEmpty(t, editCmd.Short)
	assert.NotEmpty(t, editCmd.Long)
	assert.NotNil(t, editCmd.RunE)

	assert.NotNil(t, editCmd.Flags().Lookup("slug"))
}
