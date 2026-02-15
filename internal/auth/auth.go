package auth

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// StoredAuth represents the persisted authentication state for the CLI.
type StoredAuth struct {
	Token     string    `json:"token"`
	Username  string    `json:"username"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

func TokenPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".openboot", "auth.json")
}

// LoadToken reads the auth token from disk. Returns nil if the file
// doesn't exist or the token is expired.
func LoadToken() (*StoredAuth, error) {
	path := TokenPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read auth file: %w", err)
	}

	var auth StoredAuth
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, fmt.Errorf("failed to parse auth file: %w", err)
	}

	if time.Now().After(auth.ExpiresAt) {
		return nil, nil
	}

	return &auth, nil
}

// SaveToken writes the auth token to disk with 0600 permissions.
func SaveToken(auth *StoredAuth) error {
	path := TokenPath()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create auth directory: %w", err)
	}

	data, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal auth data: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write auth file: %w", err)
	}

	return nil
}

func DeleteToken() error {
	path := TokenPath()
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete auth file: %w", err)
	}
	return nil
}

func IsAuthenticated() bool {
	auth, err := LoadToken()
	return err == nil && auth != nil
}

// GenerateCode returns an 8-character code using crypto/rand (ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789).
func GenerateCode() string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	code := make([]byte, 8)
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			// Extremely unlikely; fallback to a fixed value to avoid panic.
			code[i] = 'X'
			continue
		}
		code[i] = charset[n.Int64()]
	}
	return string(code)
}
