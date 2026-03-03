package sync

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SyncSource records which remote config was installed, so `openboot sync`
// can fetch the latest version and compute a diff.
type SyncSource struct {
	UserSlug    string    `json:"user_slug"`
	Username    string    `json:"username"`
	Slug        string    `json:"slug"`
	SyncedAt    time.Time `json:"synced_at"`
	InstalledAt time.Time `json:"installed_at"`
}

// SourcePath returns the path to the sync source file (~/.openboot/sync_source.json).
func SourcePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".openboot", "sync_source.json"), nil
}

// LoadSource reads the sync source from disk.
// Returns nil, nil if the file does not exist.
func LoadSource() (*SyncSource, error) {
	path, err := SourcePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sync source: %w", err)
	}

	var source SyncSource
	if err := json.Unmarshal(data, &source); err != nil {
		return nil, fmt.Errorf("parse sync source: %w", err)
	}

	return &source, nil
}

// SaveSource persists the sync source to disk using atomic write.
func SaveSource(source *SyncSource) error {
	path, err := SourcePath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	data, err := json.MarshalIndent(source, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sync source: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("write sync source: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename sync source: %w", err)
	}

	return nil
}

// DeleteSource removes the sync source file.
func DeleteSource() error {
	path, err := SourcePath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete sync source: %w", err)
	}
	return nil
}
