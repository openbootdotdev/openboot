package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// LocalPath returns the path to the local snapshot file (~/.openboot/snapshot.json).
func LocalPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".openboot", "snapshot.json")
}

// SaveLocal persists the snapshot to ~/.openboot/snapshot.json.
// Returns the path where the snapshot was saved.
func SaveLocal(snap *Snapshot) (string, error) {
	path := LocalPath()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write snapshot file: %w", err)
	}

	return path, nil
}

// LoadLocal reads and unmarshals the snapshot from ~/.openboot/snapshot.json.
func LoadLocal() (*Snapshot, error) {
	path := LocalPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("snapshot file not found: %s", path)
		}
		return nil, fmt.Errorf("failed to read snapshot file: %w", err)
	}

	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("failed to parse snapshot file: %w", err)
	}

	return &snap, nil
}
