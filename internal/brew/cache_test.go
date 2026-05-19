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
		finalPath: caskFile,
		interval:  10 * time.Millisecond,
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

func TestCacheTrackerReadsIncompleteWhileDownloading(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "abc123--Docker.dmg")
	// While downloading brew writes to <final>.incomplete; only after the
	// download completes does it rename to <final>. The tracker reads the
	// partial so the bar advances during the download.
	require.NoError(t, os.WriteFile(final+".incomplete", make([]byte, 5000), 0o644))

	tracker := &CacheTracker{
		finalPath: final,
		interval:  10 * time.Millisecond,
	}

	var observed atomic.Int64
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	tracker.Run(ctx, func(bytes int64) { observed.Store(bytes) })
	assert.EqualValues(t, 5000, observed.Load())
}

func TestCacheTrackerReportsLargerWhenBothExist(t *testing.T) {
	// Edge case: a stale final file from a prior install plus an in-flight
	// .incomplete. The tracker must report the larger so the bar reflects
	// the actual download progress, not the stale leftover.
	dir := t.TempDir()
	final := filepath.Join(dir, "abc123--Docker.dmg")
	require.NoError(t, os.WriteFile(final, make([]byte, 100), 0o644))
	require.NoError(t, os.WriteFile(final+".incomplete", make([]byte, 5000), 0o644))

	tracker := &CacheTracker{
		finalPath: final,
		interval:  10 * time.Millisecond,
	}

	var observed atomic.Int64
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	tracker.Run(ctx, func(bytes int64) { observed.Store(bytes) })
	assert.EqualValues(t, 5000, observed.Load())
}

func TestNewCacheTrackerPassesKindFlag(t *testing.T) {
	cases := []struct {
		name     string
		kind     CacheKind
		wantFlag string
	}{
		{"cask uses --cask", CacheKindCask, "--cask"},
		{"formula uses --formula", CacheKindFormula, "--formula"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured []string
			withFakeBrew(t, func(args []string) ([]byte, error) {
				captured = append([]string(nil), args...)
				return []byte("/tmp/abc--pkg\n"), nil
			})

			tracker, err := NewCacheTracker("pkg", tc.kind)
			require.NoError(t, err)
			require.NotNil(t, tracker)
			assert.Equal(t, []string{"--cache", tc.wantFlag, "pkg"}, captured)
		})
	}
}

func TestCacheTrackerNoMatchReportsZero(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "unrelated--Other.dmg"), make([]byte, 100), 0o644))

	tracker := &CacheTracker{
		finalPath: filepath.Join(dir, "abc123--Docker.dmg"),
		interval:  10 * time.Millisecond,
	}

	var observed atomic.Int64
	observed.Store(-1) // sentinel
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	tracker.Run(ctx, func(bytes int64) { observed.Store(bytes) })
	// No matching file → callback receives 0.
	assert.EqualValues(t, 0, observed.Load())
}
