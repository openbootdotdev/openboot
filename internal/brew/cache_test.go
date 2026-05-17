package brew

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheTrackerReportsFileSize(t *testing.T) {
	dir := t.TempDir()
	caskFile := filepath.Join(dir, "abc123--Docker.dmg")
	require.NoError(t, os.WriteFile(caskFile, make([]byte, 1024), 0o644))

	tracker := &CacheTracker{
		cacheDir: dir,
		match:    "Docker",
		interval: 10 * time.Millisecond,
	}

	var observed atomic.Int64
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tracker.Run(ctx, func(bytes int64) { observed.Store(bytes) })
	}()
	wg.Wait()
	assert.EqualValues(t, 1024, observed.Load())
}

func TestCacheTrackerPicksLargestMatchingFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "old--Docker.dmg"), make([]byte, 100), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "new--Docker.dmg.incomplete"), make([]byte, 5000), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "unrelated--Rectangle.dmg"), make([]byte, 9999), 0o644))

	tracker := &CacheTracker{
		cacheDir: dir,
		match:    "Docker",
		interval: 10 * time.Millisecond,
	}

	var observed atomic.Int64
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	tracker.Run(ctx, func(bytes int64) { observed.Store(bytes) })
	// Picks the largest matching file (the .incomplete download).
	assert.EqualValues(t, 5000, observed.Load())
}

func TestCacheTrackerNoMatchReportsZero(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "unrelated--Other.dmg"), make([]byte, 100), 0o644))

	tracker := &CacheTracker{
		cacheDir: dir,
		match:    "Docker",
		interval: 10 * time.Millisecond,
	}

	var observed atomic.Int64
	observed.Store(-1) // sentinel
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	tracker.Run(ctx, func(bytes int64) { observed.Store(bytes) })
	// No matching file → callback receives 0.
	assert.EqualValues(t, 0, observed.Load())
}
