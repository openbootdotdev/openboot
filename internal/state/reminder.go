package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ReminderState struct {
	Dismissed bool `json:"dismissed"`
	Skipped   bool `json:"skipped"`
}

func DefaultStatePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".openboot", "state.json")
}

func LoadState(path string) (*ReminderState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ReminderState{Dismissed: false, Skipped: false}, nil
		}
		return nil, fmt.Errorf("read state: %w", err)
	}

	var state ReminderState
	if err := json.Unmarshal(data, &state); err != nil {
		// Log warning to stderr for corrupted JSON, but return default state gracefully
		fmt.Fprintf(os.Stderr, "warning: parse state, using defaults: %v\n", err)
		return &ReminderState{Dismissed: false, Skipped: false}, nil
	}

	return &state, nil
}

func SaveState(path string, s *ReminderState) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	// Atomic write: write to temp file, then rename
	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("write temp state: %w", err)
	}

	if err := os.Rename(tmpFile, path); err != nil {
		// Clean up temp file if rename fails
		_ = os.Remove(tmpFile)
		return fmt.Errorf("rename state: %w", err)
	}

	return nil
}

func ShouldShowReminder(s *ReminderState) bool {
	return !s.Dismissed && !s.Skipped
}

func MarkDismissed(s *ReminderState) {
	s.Dismissed = true
}

func MarkSkipped(s *ReminderState) {
	s.Skipped = true
}
