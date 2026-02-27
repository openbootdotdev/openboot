package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadToken_Success(t *testing.T) {
	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, ".openboot")
	authFile := filepath.Join(authDir, "auth.json")

	require.NoError(t, os.MkdirAll(authDir, 0700))

	expiresAt := time.Now().Add(24 * time.Hour)
	auth := &StoredAuth{
		Token:     "obt_test_token_123",
		Username:  "testuser",
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}

	data, err := json.MarshalIndent(auth, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(authFile, data, 0600))

	t.Setenv("HOME", tmpDir)

	loaded, err := LoadToken()
	require.NoError(t, err)
	assert.NotNil(t, loaded)
	assert.Equal(t, "obt_test_token_123", loaded.Token)
	assert.Equal(t, "testuser", loaded.Username)
}

func TestLoadToken_FileNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	loaded, err := LoadToken()
	assert.NoError(t, err)
	assert.Nil(t, loaded)
}

func TestLoadToken_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, ".openboot")
	authFile := filepath.Join(authDir, "auth.json")

	require.NoError(t, os.MkdirAll(authDir, 0700))
	require.NoError(t, os.WriteFile(authFile, []byte("invalid json {"), 0600))

	t.Setenv("HOME", tmpDir)

	loaded, err := LoadToken()
	assert.Error(t, err)
	assert.Nil(t, loaded)
	assert.Contains(t, err.Error(), "parse auth")
}

func TestLoadToken_ExpiredToken(t *testing.T) {
	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, ".openboot")
	authFile := filepath.Join(authDir, "auth.json")

	require.NoError(t, os.MkdirAll(authDir, 0700))

	expiresAt := time.Now().Add(-1 * time.Hour)
	auth := &StoredAuth{
		Token:     "obt_expired_token",
		Username:  "testuser",
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().Add(-25 * time.Hour),
	}

	data, err := json.MarshalIndent(auth, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(authFile, data, 0600))

	t.Setenv("HOME", tmpDir)

	loaded, err := LoadToken()
	assert.NoError(t, err)
	assert.Nil(t, loaded)
}

func TestLoadToken_MissingFields(t *testing.T) {
	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, ".openboot")
	authFile := filepath.Join(authDir, "auth.json")

	require.NoError(t, os.MkdirAll(authDir, 0700))

	expiresAt := time.Now().Add(24 * time.Hour)
	partialAuth := map[string]interface{}{
		"token":      "obt_partial_token",
		"expires_at": expiresAt.Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(partialAuth, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(authFile, data, 0600))

	t.Setenv("HOME", tmpDir)

	loaded, err := LoadToken()
	require.NoError(t, err)
	assert.NotNil(t, loaded)
	assert.Equal(t, "obt_partial_token", loaded.Token)
	assert.Equal(t, "", loaded.Username)
}

func TestSaveToken_Success(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	auth := &StoredAuth{
		Token:     "obt_save_test_token",
		Username:  "saveuser",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}

	err := SaveToken(auth)
	require.NoError(t, err)

	authFile := filepath.Join(tmpDir, ".openboot", "auth.json")
	assert.FileExists(t, authFile)

	data, err := os.ReadFile(authFile)
	require.NoError(t, err)

	var loaded StoredAuth
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)
	assert.Equal(t, "obt_save_test_token", loaded.Token)
	assert.Equal(t, "saveuser", loaded.Username)
}

func TestSaveToken_CreatesDirectoryWith0700(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	auth := &StoredAuth{
		Token:     "obt_dir_test_token",
		Username:  "diruser",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}

	err := SaveToken(auth)
	require.NoError(t, err)

	authDir := filepath.Join(tmpDir, ".openboot")
	info, err := os.Stat(authDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	mode := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0700), mode)
}

func TestSaveToken_CreatesFileWith0600(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	auth := &StoredAuth{
		Token:     "obt_file_perm_token",
		Username:  "permuser",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}

	err := SaveToken(auth)
	require.NoError(t, err)

	authFile := filepath.Join(tmpDir, ".openboot", "auth.json")
	info, err := os.Stat(authFile)
	require.NoError(t, err)

	mode := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0600), mode)
}

func TestSaveToken_OverwritesExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	auth1 := &StoredAuth{
		Token:     "obt_first_token",
		Username:  "user1",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}

	err := SaveToken(auth1)
	require.NoError(t, err)

	auth2 := &StoredAuth{
		Token:     "obt_second_token",
		Username:  "user2",
		ExpiresAt: time.Now().Add(48 * time.Hour),
		CreatedAt: time.Now(),
	}

	err = SaveToken(auth2)
	require.NoError(t, err)

	authFile := filepath.Join(tmpDir, ".openboot", "auth.json")
	data, err := os.ReadFile(authFile)
	require.NoError(t, err)

	var loaded StoredAuth
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)
	assert.Equal(t, "obt_second_token", loaded.Token)
	assert.Equal(t, "user2", loaded.Username)
}

func TestDeleteToken_Success(t *testing.T) {
	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, ".openboot")
	authFile := filepath.Join(authDir, "auth.json")

	require.NoError(t, os.MkdirAll(authDir, 0700))

	auth := &StoredAuth{
		Token:     "obt_delete_token",
		Username:  "deleteuser",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}

	data, err := json.MarshalIndent(auth, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(authFile, data, 0600))

	t.Setenv("HOME", tmpDir)

	err = DeleteToken()
	require.NoError(t, err)

	assert.NoFileExists(t, authFile)
}

func TestDeleteToken_FileNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	err := DeleteToken()
	assert.NoError(t, err)
}

func TestDeleteToken_PermissionError(t *testing.T) {
	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, ".openboot")
	authFile := filepath.Join(authDir, "auth.json")

	require.NoError(t, os.MkdirAll(authDir, 0700))
	require.NoError(t, os.WriteFile(authFile, []byte("test"), 0600))

	require.NoError(t, os.Chmod(authDir, 0500))
	t.Cleanup(func() {
		os.Chmod(authDir, 0700)
	})

	t.Setenv("HOME", tmpDir)

	err := DeleteToken()
	assert.Error(t, err)
}

func TestIsAuthenticated_ValidToken(t *testing.T) {
	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, ".openboot")
	authFile := filepath.Join(authDir, "auth.json")

	require.NoError(t, os.MkdirAll(authDir, 0700))

	expiresAt := time.Now().Add(24 * time.Hour)
	auth := &StoredAuth{
		Token:     "obt_valid_token",
		Username:  "validuser",
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}

	data, err := json.MarshalIndent(auth, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(authFile, data, 0600))

	t.Setenv("HOME", tmpDir)

	assert.True(t, IsAuthenticated())
}

func TestIsAuthenticated_ExpiredToken(t *testing.T) {
	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, ".openboot")
	authFile := filepath.Join(authDir, "auth.json")

	require.NoError(t, os.MkdirAll(authDir, 0700))

	expiresAt := time.Now().Add(-1 * time.Hour)
	auth := &StoredAuth{
		Token:     "obt_expired_token",
		Username:  "expireduser",
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().Add(-25 * time.Hour),
	}

	data, err := json.MarshalIndent(auth, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(authFile, data, 0600))

	t.Setenv("HOME", tmpDir)

	assert.False(t, IsAuthenticated())
}

func TestIsAuthenticated_NoToken(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	assert.False(t, IsAuthenticated())
}

func TestGenerateCode_Length(t *testing.T) {
	code, err := GenerateCode()
	require.NoError(t, err)
	assert.Equal(t, 8, len(code))
}

func TestGenerateCode_Alphanumeric(t *testing.T) {
	alphanumericRegex := regexp.MustCompile(`^[ABCDEFGHJKLMNPQRSTUVWXYZ23456789]{8}$`)

	for i := 0; i < 100; i++ {
		code, err := GenerateCode()
		require.NoError(t, err)
		assert.True(t, alphanumericRegex.MatchString(code), "code %s is not alphanumeric", code)
	}
}

func TestGenerateCode_Uniqueness(t *testing.T) {
	codes := make(map[string]bool)

	for i := 0; i < 1000; i++ {
		code, err := GenerateCode()
		require.NoError(t, err)
		assert.False(t, codes[code], "duplicate code generated: %s", code)
		codes[code] = true
	}

	assert.Equal(t, 1000, len(codes))
}

func TestGenerateCode_NoFixedValue(t *testing.T) {
	code1, err := GenerateCode()
	require.NoError(t, err)
	code2, err := GenerateCode()
	require.NoError(t, err)
	assert.NotEqual(t, code1, code2)
}

func TestTokenPath_Format(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path, err := TokenPath()
	assert.NoError(t, err)
	assert.Contains(t, path, ".openboot")
	assert.Contains(t, path, "auth.json")
	assert.True(t, filepath.IsAbs(path))
}

func TestTokenPath_ConsistentPath(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path1, err := TokenPath()
	assert.NoError(t, err)
	path2, err := TokenPath()
	assert.NoError(t, err)
	assert.Equal(t, path1, path2)
}

func TestStoredAuth_JSONMarshaling(t *testing.T) {
	expiresAt := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC)

	auth := &StoredAuth{
		Token:     "obt_test_token",
		Username:  "testuser",
		ExpiresAt: expiresAt,
		CreatedAt: createdAt,
	}

	data, err := json.MarshalIndent(auth, "", "  ")
	require.NoError(t, err)

	var loaded StoredAuth
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)

	assert.Equal(t, auth.Token, loaded.Token)
	assert.Equal(t, auth.Username, loaded.Username)
	assert.Equal(t, auth.ExpiresAt.Unix(), loaded.ExpiresAt.Unix())
	assert.Equal(t, auth.CreatedAt.Unix(), loaded.CreatedAt.Unix())
}

func TestLoadToken_ReadPermissionError(t *testing.T) {
	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, ".openboot")
	authFile := filepath.Join(authDir, "auth.json")

	require.NoError(t, os.MkdirAll(authDir, 0700))
	require.NoError(t, os.WriteFile(authFile, []byte("{}"), 0000))

	t.Cleanup(func() {
		os.Chmod(authFile, 0600)
	})

	t.Setenv("HOME", tmpDir)

	loaded, err := LoadToken()
	assert.Error(t, err)
	assert.Nil(t, loaded)
}

func TestSaveToken_MarshalError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	auth := &StoredAuth{
		Token:     "obt_test_token",
		Username:  "testuser",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}

	err := SaveToken(auth)
	require.NoError(t, err)

	authFile := filepath.Join(tmpDir, ".openboot", "auth.json")
	data, err := os.ReadFile(authFile)
	require.NoError(t, err)

	assert.Contains(t, string(data), "obt_test_token")
	assert.Contains(t, string(data), "testuser")
}

func TestLoadToken_BoundaryExpiration(t *testing.T) {
	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, ".openboot")
	authFile := filepath.Join(authDir, "auth.json")

	require.NoError(t, os.MkdirAll(authDir, 0700))

	expiresAt := time.Now().Add(1 * time.Second)
	auth := &StoredAuth{
		Token:     "obt_boundary_token",
		Username:  "boundaryuser",
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}

	data, err := json.MarshalIndent(auth, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(authFile, data, 0600))

	t.Setenv("HOME", tmpDir)

	loaded, err := LoadToken()
	require.NoError(t, err)
	assert.NotNil(t, loaded)

	time.Sleep(2 * time.Second)

	loaded, err = LoadToken()
	require.NoError(t, err)
	assert.Nil(t, loaded)
}
