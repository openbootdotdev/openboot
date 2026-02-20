package ui

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"within limit", "hello", 10, "hello"},
		{"exact limit", "hello", 5, "hello"},
		{"needs truncation with ellipsis", "hello world", 8, "hello..."},
		{"maxLen exactly 3", "hello", 3, "hel"},
		{"maxLen 4 adds ellipsis", "hello world", 4, "h..."},
		{"maxLen 2 no ellipsis", "hello", 2, "he"},
		{"empty string", "", 5, ""},
		{"empty string zero len", "", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Duration
		expected string
	}{
		{"sub-second", 500 * time.Millisecond, "0.5s"},
		{"10 seconds", 10 * time.Second, "10.0s"},
		{"30.5 seconds", 30*time.Second + 500*time.Millisecond, "30.5s"},
		{"exactly 1 minute", 60 * time.Second, "1m0s"},
		{"1 minute 30 seconds", 90 * time.Second, "1m30s"},
		{"2 minutes", 120 * time.Second, "2m0s"},
		{"2 minutes 15 seconds", 135 * time.Second, "2m15s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatDuration(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStickyProgressDefaults(t *testing.T) {
	sp := NewStickyProgress(10)

	assert.Equal(t, 10, sp.total)
	assert.Equal(t, 0, sp.completed)
	assert.Equal(t, 0, sp.succeeded)
	assert.Equal(t, 0, sp.failed)
	assert.Equal(t, 0, sp.skipped)
	assert.False(t, sp.active)
}

func TestStickyProgressIncrementWithStatus(t *testing.T) {
	sp := NewStickyProgress(5)

	sp.IncrementWithStatus(true)
	sp.IncrementWithStatus(true)
	sp.IncrementWithStatus(false)

	assert.Equal(t, 3, sp.completed)
	assert.Equal(t, 2, sp.succeeded)
	assert.Equal(t, 1, sp.failed)
}

func TestStickyProgressIncrement(t *testing.T) {
	sp := NewStickyProgress(5)

	sp.Increment()
	sp.Increment()

	assert.Equal(t, 2, sp.completed)
	assert.Equal(t, 2, sp.succeeded)
	assert.Equal(t, 0, sp.failed)
}

func TestStickyProgressSetSkipped(t *testing.T) {
	sp := NewStickyProgress(5)
	sp.SetSkipped(3)

	assert.Equal(t, 3, sp.skipped)
}

func TestStickyProgressCombined(t *testing.T) {
	sp := NewStickyProgress(10)

	sp.Increment()
	sp.IncrementWithStatus(true)
	sp.IncrementWithStatus(false)
	sp.SetSkipped(4)

	assert.Equal(t, 3, sp.completed)
	assert.Equal(t, 2, sp.succeeded)
	assert.Equal(t, 1, sp.failed)
	assert.Equal(t, 4, sp.skipped)
}

func TestStickyProgressConcurrentSafety(t *testing.T) {
	const goroutines = 50
	sp := NewStickyProgress(goroutines * 2)

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			sp.IncrementWithStatus(true)
		}()
		go func() {
			defer wg.Done()
			sp.IncrementWithStatus(false)
		}()
	}
	wg.Wait()

	assert.Equal(t, goroutines*2, sp.completed)
	assert.Equal(t, goroutines, sp.succeeded)
	assert.Equal(t, goroutines, sp.failed)
}

func TestStickyProgressSetCurrentDoesNotPanic(t *testing.T) {
	sp := NewStickyProgress(5)
	assert.NotPanics(t, func() {
		sp.SetCurrent("some-package")
	})
}

func TestStickyProgressPauseResume(t *testing.T) {
	sp := NewStickyProgress(5)
	sp.mu.Lock()
	sp.active = true
	sp.mu.Unlock()

	sp.PauseForInteractive()
	assert.False(t, sp.active)
}
