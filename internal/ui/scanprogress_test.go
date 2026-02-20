package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFormatStepCount(t *testing.T) {
	tests := []struct {
		count    int
		expected string
	}{
		{0, "0 found"},
		{1, "1 found"},
		{2, "2 found"},
		{100, "100 found"},
	}

	for _, tt := range tests {
		result := formatStepCount(tt.count)
		assert.Equal(t, tt.expected, result)
	}
}

func TestScanProgressFormatStepDuration(t *testing.T) {
	sp := &ScanProgress{}

	tests := []struct {
		name     string
		input    time.Duration
		expected string
	}{
		{"zero", 0, "< 1s"},
		{"999ms", 999 * time.Millisecond, "< 1s"},
		{"exactly 1s", 1 * time.Second, "1.0s"},
		{"2.5s", 2500 * time.Millisecond, "2.5s"},
		{"10s", 10 * time.Second, "10.0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sp.formatStepDuration(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewScanProgressInitialState(t *testing.T) {
	sp := NewScanProgress(5)
	defer close(sp.spinnerStop)

	assert.Equal(t, 5, sp.totalSteps)
	assert.Equal(t, 5, len(sp.steps))
	assert.Equal(t, 0, sp.completedCount)

	for _, s := range sp.steps {
		assert.Equal(t, "pending", s.status)
	}
}
