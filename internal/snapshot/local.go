package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func LocalPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".openboot", "snapshot.json")
}

func SaveLocal(snap *Snapshot) (string, error) {
	path := LocalPath()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create snapshot dir: %w", err)
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal snapshot: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("write snapshot: %w", err)
	}

	return path, nil
}

func LoadLocal() (*Snapshot, error) {
	return LoadFile(LocalPath())
}

func LoadFile(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("snapshot file not found: %s", path)
		}
		return nil, fmt.Errorf("read snapshot: %w", err)
	}

	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parse snapshot: %w", err)
	}

	return &snap, nil
}
