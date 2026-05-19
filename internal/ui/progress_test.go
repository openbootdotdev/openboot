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
	// Seed some byte state from a "previous" package and install-wide totals.
	sp.currentBytes = 999
	sp.totalBytes = 9999
	sp.speed = 1234
	sp.lastBytes = 999
	sp.lastTime = time.Unix(42, 0)
	sp.installTotalBytes = 5_000_000
	sp.installCompletedBytes = 2_000_000

	sp.SetPhase(PhaseCask)
	assert.Equal(t, PhaseCask, sp.phase)

	// SetPhase clears per-package byte state for a clean start.
	assert.EqualValues(t, 0, sp.currentBytes)
	assert.EqualValues(t, 0, sp.totalBytes)
	assert.InDelta(t, 0, sp.speed, 0.01)
	assert.EqualValues(t, 0, sp.lastBytes)
	assert.True(t, sp.lastTime.IsZero())
	// Install-wide aggregates are NOT touched — they span both phases.
	assert.EqualValues(t, 5_000_000, sp.installTotalBytes)
	assert.EqualValues(t, 2_000_000, sp.installCompletedBytes)
}

func TestStickyProgressPctForBarByteBasedAcrossPhases(t *testing.T) {
	sp := NewStickyProgress(5)
	// Total install: 5 formulae + 2 casks = some bytes mix.
	sp.SetTotalBytes(100_000_000) // 100 MB across the whole install

	// Nothing started → 0%.
	assert.InDelta(t, 0, sp.pctForBar(), 0.001)

	// Formula phase: each formula complete adds its size lump-sum (no tracker).
	sp.SetPhase(PhaseFormula)
	sp.AddCompletedBytes(5_000_000) // first formula done: 5MB
	assert.InDelta(t, 0.05, sp.pctForBar(), 0.001)

	sp.AddCompletedBytes(3_000_000) // second formula done: 3MB. Total 8MB.
	assert.InDelta(t, 0.08, sp.pctForBar(), 0.001)

	// Cask phase. Mid-download of first cask: 30M of 60M.
	sp.SetPhase(PhaseCask)
	sp.SetCurrentBytes(30_000_000, 60_000_000)
	// 8M done + 30M in flight = 38M / 100M = 0.38.
	assert.InDelta(t, 0.38, sp.pctForBar(), 0.001)

	// First cask done (60MB).
	sp.AddCompletedBytes(60_000_000)
	// 8 + 60 = 68 / 100 = 0.68. currentBytes was cleared.
	assert.InDelta(t, 0.68, sp.pctForBar(), 0.001)
}

func TestStickyProgressPctForBarFallsBackToCount(t *testing.T) {
	sp := NewStickyProgress(4)
	// installTotalBytes left at 0 (e.g. all HEAD pre-fetches failed) → count-based.
	sp.completed = 2
	assert.InDelta(t, 0.5, sp.pctForBar(), 0.001)
}

func TestStickyProgressAddCompletedBytesResetsCurrent(t *testing.T) {
	sp := NewStickyProgress(3)
	sp.SetTotalBytes(100_000_000)
	sp.SetPhase(PhaseCask)
	sp.SetCurrentBytes(40_000_000, 40_000_000)
	sp.speed = 5_000_000 // pretend EMA is established

	sp.AddCompletedBytes(40_000_000)

	// Per-package state cleared, but speed kept (it's a network-level estimate
	// that should carry across packages).
	assert.EqualValues(t, 40_000_000, sp.installCompletedBytes)
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
	eta := sp.estimateCurrentETA()
	// 400MB / 10 MB/s = 40s.
	assert.Equal(t, "~40s", eta)
}

func TestStickyProgressEstimatingPlaceholder(t *testing.T) {
	sp := NewStickyProgress(10)
	sp.SetPhase(PhaseCask)
	sp.currentBytes = 0
	sp.totalBytes = 100_000_000
	sp.speed = 0
	eta := sp.estimateCurrentETA()
	assert.Equal(t, "estimating...", eta)
}

func TestStickyProgressFormatHeadShowsBytesSpeedETA(t *testing.T) {
	// The head must be identical in shape for formula and cask phases —
	// the prior split (formula = count only, cask = bytes/speed/ETA) was
	// the visible inconsistency users noticed during longer formula
	// downloads like vhs.
	for _, phase := range []Phase{PhaseFormula, PhaseCask} {
		sp := NewStickyProgress(5)
		sp.SetPhase(phase)
		sp.currentPkg = "vhs"
		sp.completed = 1
		sp.currentBytes = 12 * 1024 * 1024
		sp.totalBytes = 28 * 1024 * 1024
		sp.speed = 5 * 1024 * 1024

		head := sp.formatHead()
		assert.Contains(t, head, "[1/5]")
		assert.Contains(t, head, "vhs")
		assert.Contains(t, head, "12M/28M")
		assert.Contains(t, head, "5M/s")
		// 16M remaining / 5M/s ≈ 3s.
		assert.Contains(t, head, "~3s")
	}
}

func TestStickyProgressFormatHeadFallsBackToDashes(t *testing.T) {
	// HEAD pre-fetch failed (size unknown) and tracker hasn't seeded speed
	// yet → all three data columns show "—" instead of bogus zeros.
	sp := NewStickyProgress(3)
	sp.currentPkg = "wget"
	sp.completed = 0

	head := sp.formatHead()
	assert.Contains(t, head, "wget")
	assert.Contains(t, head, "— · — · —")
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
