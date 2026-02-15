package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ReminderState represents the persisted state for screen recording permission reminder.
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

// LoadState reads reminder state. Returns default state if file is missing
// or contains invalid JSON (logs warning to stderr in the latter case).
func LoadState(path string) (*ReminderState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ReminderState{Dismissed: false, Skipped: false}, nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state ReminderState
	if err := json.Unmarshal(data, &state); err != nil {
		// Log warning to stderr for corrupted JSON, but return default state gracefully
		fmt.Fprintf(os.Stderr, "warning: failed to parse state file, using defaults: %v\n", err)
		return &ReminderState{Dismissed: false, Skipped: false}, nil
	}

	return &state, nil
}

// SaveState writes reminder state atomically (temp file + rename).
func SaveState(path string, s *ReminderState) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state data: %w", err)
	}

	// Atomic write: write to temp file, then rename
	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary state file: %w", err)
	}

	if err := os.Rename(tmpFile, path); err != nil {
		// Clean up temp file if rename fails
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}

func ShouldShowReminder(s *ReminderState) bool {
	return !s.Dismissed
}

func MarkDismissed(s *ReminderState) {
	s.Dismissed = true
}

func MarkSkipped(s *ReminderState) {
	s.Skipped = true
}
