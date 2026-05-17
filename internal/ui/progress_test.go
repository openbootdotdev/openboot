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

func TestStickyProgressSetPhase(t *testing.T) {
	sp := NewStickyProgress(10)
	// Seed some byte state from a "previous" cask.
	sp.currentBytes = 999
	sp.totalBytes = 9999
	sp.speed = 1234
	sp.lastBytes = 999
	sp.lastTime = time.Unix(42, 0)
	sp.phaseTotalBytes = 5_000_000
	sp.phaseCompletedBytes = 2_000_000

	sp.SetPhase(PhaseCask)
	assert.Equal(t, PhaseCask, sp.phase)

	// SetPhase must reset all per-cask byte state so the next cask
	// starts clean (no stale bytes bleeding into the bar). Phase-wide
	// aggregates reset too — re-entering the cask phase starts fresh.
	assert.EqualValues(t, 0, sp.currentBytes)
	assert.EqualValues(t, 0, sp.totalBytes)
	assert.InDelta(t, 0, sp.speed, 0.01)
	assert.EqualValues(t, 0, sp.lastBytes)
	assert.True(t, sp.lastTime.IsZero())
	assert.EqualValues(t, 0, sp.phaseTotalBytes)
	assert.EqualValues(t, 0, sp.phaseCompletedBytes)
}

func TestStickyProgressPctForBarByteBased(t *testing.T) {
	sp := NewStickyProgress(3)
	sp.SetPhase(PhaseCask)
	sp.SetPhaseBytesTotal(100_000_000) // 100 MB across the cask phase

	// Nothing started yet → 0%.
	assert.InDelta(t, 0, sp.pctForBar(), 0.001)

	// Mid-download of first cask: 30M of 100M.
	sp.SetCurrentBytes(30_000_000, 100_000_000)
	assert.InDelta(t, 0.3, sp.pctForBar(), 0.001)

	// First cask done — 40M cask actually, advance the aggregate.
	sp.AddCompletedCaskBytes(40_000_000)
	assert.InDelta(t, 0.4, sp.pctForBar(), 0.001)

	// Second cask mid-download: 30M of 60M, on top of the 40M already done.
	sp.SetCurrentBytes(30_000_000, 60_000_000)
	// (40 + 30) / 100 = 0.7
	assert.InDelta(t, 0.7, sp.pctForBar(), 0.001)
}

func TestStickyProgressPctForBarFallsBackToCount(t *testing.T) {
	sp := NewStickyProgress(4)
	sp.SetPhase(PhaseCask)
	// phaseTotalBytes left at 0 (e.g. all HEAD pre-fetches failed) → count-based.
	sp.completed = 2
	assert.InDelta(t, 0.5, sp.pctForBar(), 0.001)
}

func TestStickyProgressPctForBarFormulaPhaseUsesCount(t *testing.T) {
	sp := NewStickyProgress(10)
	// PhaseFormula is the default. Byte aggregates should be ignored even if set.
	sp.phaseTotalBytes = 999_999_999
	sp.phaseCompletedBytes = 500_000_000
	sp.completed = 3
	assert.InDelta(t, 0.3, sp.pctForBar(), 0.001)
}

func TestStickyProgressAddCompletedCaskBytesResetsCurrent(t *testing.T) {
	sp := NewStickyProgress(3)
	sp.SetPhase(PhaseCask)
	sp.SetPhaseBytesTotal(100_000_000)
	sp.SetCurrentBytes(40_000_000, 40_000_000)
	sp.speed = 5_000_000 // pretend EMA is established

	sp.AddCompletedCaskBytes(40_000_000)

	// Per-cask state cleared, but speed kept (it's a network-level estimate
	// that should carry across casks).
	assert.EqualValues(t, 40_000_000, sp.phaseCompletedBytes)
	assert.EqualValues(t, 0, sp.currentBytes)
	assert.EqualValues(t, 0, sp.totalBytes)
	assert.EqualValues(t, 0, sp.lastBytes)
	assert.True(t, sp.lastTime.IsZero())
	assert.InDelta(t, 5_000_000, sp.speed, 0.01)
}

func TestStickyProgressSetCurrentBytes(t *testing.T) {
	sp := NewStickyProgress(10)
	sp.SetPhase(PhaseCask)
	sp.SetCurrentBytes(50_000_000, 200_000_000)
	assert.EqualValues(t, 50_000_000, sp.currentBytes)
	assert.EqualValues(t, 200_000_000, sp.totalBytes)
}

func TestStickyProgressSpeedEMA(t *testing.T) {
	sp := NewStickyProgress(10)
	sp.SetPhase(PhaseCask)
	// First sample: no speed yet (need two points).
	sp.observeBytesAt(1_000_000, time.Unix(0, 0))
	assert.InDelta(t, 0, sp.speed, 0.01)
	// Second sample one second later: 1 MB more = 1 MB/s, sets speed directly.
	sp.observeBytesAt(2_000_000, time.Unix(1, 0))
	assert.InDelta(t, 1_000_000, sp.speed, 1.0)
	// Third sample: 500 KB more in 1s = 500 K/s instantaneous.
	// EMA blends: 0.3*500_000 + 0.7*1_000_000 = 850_000.
	sp.observeBytesAt(2_500_000, time.Unix(2, 0))
	assert.InDelta(t, 850_000, sp.speed, 1.0)
}

func TestStickyProgressBytesETAUsesSpeed(t *testing.T) {
	sp := NewStickyProgress(10)
	sp.SetPhase(PhaseCask)
	sp.currentBytes = 100_000_000
	sp.totalBytes = 500_000_000
	sp.speed = 10_000_000 // 10 MB/s
	eta := sp.estimateCurrentCaskETA()
	// 400MB / 10 MB/s = 40s.
	assert.Equal(t, "~40s", eta)
}

func TestStickyProgressEstimatingPlaceholder(t *testing.T) {
	sp := NewStickyProgress(10)
	sp.SetPhase(PhaseCask)
	sp.currentBytes = 0
	sp.totalBytes = 100_000_000
	sp.speed = 0
	eta := sp.estimateCurrentCaskETA()
	assert.Equal(t, "estimating...", eta)
}

func TestStickyProgressFallsBackWhenScrollRegionUnsupported(t *testing.T) {
	t.Setenv("TERM", "dumb")
	sp := NewStickyProgress(3)
	sp.Start()
	defer sp.Finish()
	// Region should NOT have been attached.
	sp.mu.Lock()
	hasRegion := sp.region != nil
	sp.mu.Unlock()
	assert.False(t, hasRegion, "scroll region should be disabled on dumb TERM")
}
