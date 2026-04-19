package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/auth"
)

func setupTestAuth(t *testing.T, authenticated bool) string {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if authenticated {
		authDir := filepath.Join(tmpDir, ".openboot")
		authFile := filepath.Join(authDir, "auth.json")
		require.NoError(t, os.MkdirAll(authDir, 0700))

		expiresAt := time.Now().Add(24 * time.Hour)
		storedAuth := &auth.StoredAuth{
			Token:     "obt_test_token_123",
			Username:  "testuser",
			ExpiresAt: expiresAt,
			CreatedAt: time.Now(),
		}

		data, err := json.MarshalIndent(storedAuth, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(authFile, data, 0600))
	}

	return tmpDir
}

func TestLoginCmd_AlreadyAuthenticated(t *testing.T) {
	tmpDir := setupTestAuth(t, true)

	cmd := &cobra.Command{}
	err := loginCmd.RunE(cmd, []string{})

	assert.NoError(t, err)

	authFile := filepath.Join(tmpDir, ".openboot", "auth.json")
	_, err = os.Stat(authFile)
	assert.NoError(t, err, "auth file should still exist")
}

func TestLoginCmd_NotAuthenticated_FailsWithoutMockServer(t *testing.T) {
	setupTestAuth(t, false)

	t.Setenv("OPENBOOT_API_URL", "http://localhost:9999")

	cmd := &cobra.Command{}
	err := loginCmd.RunE(cmd, []string{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "login failed")
}

func TestLogoutCmd_WhenAuthenticated(t *testing.T) {
	tmpDir := setupTestAuth(t, true)
	authFile := filepath.Join(tmpDir, ".openboot", "auth.json")

	_, err := os.Stat(authFile)
	require.NoError(t, err, "auth file should exist before logout")

	cmd := &cobra.Command{}
	err = logoutCmd.RunE(cmd, []string{})

	assert.NoError(t, err)

	_, err = os.Stat(authFile)
	assert.True(t, os.IsNotExist(err), "auth file should be deleted after logout")
}

func TestLogoutCmd_WhenNotAuthenticated(t *testing.T) {
	setupTestAuth(t, false)

	cmd := &cobra.Command{}
	err := logoutCmd.RunE(cmd, []string{})

	assert.NoError(t, err)
}

func TestLogoutCmd_DeleteError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses filesystem permission checks")
	}
	tmpDir := setupTestAuth(t, true)
	authDir := filepath.Join(tmpDir, ".openboot")

	require.NoError(t, os.Chmod(authDir, 0500))
	defer os.Chmod(authDir, 0700) //nolint:errcheck // cleanup restore; failure is non-critical in tests

	cmd := &cobra.Command{}
	err := logoutCmd.RunE(cmd, []string{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "logout failed")
}

func TestLoginCmd_CommandStructure(t *testing.T) {
	tests := []struct {
		name      string
		cmd       *cobra.Command
		checkUse  string
		checkDesc bool
	}{
		{
			name:      "login command",
			cmd:       loginCmd,
			checkUse:  "login",
			checkDesc: true,
		},
		{
			name:      "logout command",
			cmd:       logoutCmd,
			checkUse:  "logout",
			checkDesc: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.checkUse, tt.cmd.Use)
			if tt.checkDesc {
				assert.NotEmpty(t, tt.cmd.Short)
				assert.NotEmpty(t, tt.cmd.Long)
			}
			assert.NotNil(t, tt.cmd.RunE)
		})
	}
}

func TestLoginLogout_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := setupTestAuth(t, false)
	authFile := filepath.Join(tmpDir, ".openboot", "auth.json")

	_, err := os.Stat(authFile)
	assert.True(t, os.IsNotExist(err), "auth file should not exist initially")

	authDir := filepath.Join(tmpDir, ".openboot")
	require.NoError(t, os.MkdirAll(authDir, 0700))

	expiresAt := time.Now().Add(24 * time.Hour)
	storedAuth := &auth.StoredAuth{
		Token:     "obt_integration_token",
		Username:  "integrationuser",
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}

	data, err := json.MarshalIndent(storedAuth, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(authFile, data, 0600))

	assert.True(t, auth.IsAuthenticated())

	cmd := &cobra.Command{}
	err = logoutCmd.RunE(cmd, []string{})
	assert.NoError(t, err)

	_, err = os.Stat(authFile)
	assert.True(t, os.IsNotExist(err), "auth file should be deleted after logout")

	assert.False(t, auth.IsAuthenticated())
}
