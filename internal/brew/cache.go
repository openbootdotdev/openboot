package brew

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CacheTracker polls the brew downloads directory for the partial file
// matching a cask name and reports its current size.
type CacheTracker struct {
	cacheDir string
	match    string
	interval time.Duration
}

// NewCacheTracker builds a tracker for the given cask. cacheDir is resolved
// via `brew --cache --cask <name>` (parent dir).
func NewCacheTracker(caskName string) (*CacheTracker, error) {
	dir, err := resolveBrewCacheDir(caskName)
	if err != nil {
		return nil, err
	}
	return &CacheTracker{
		cacheDir: dir,
		match:    caskName,
		interval: 500 * time.Millisecond,
	}, nil
}

// Run polls every interval and invokes onProgress with the current matched
// file size. Stops when ctx is done. Always emits at least one final value
// before returning.
func (t *CacheTracker) Run(ctx context.Context, onProgress func(bytes int64)) {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	emit := func() { onProgress(t.currentSize()) }
	emit()
	for {
		select {
		case <-ctx.Done():
			emit() // final reading
			return
		case <-ticker.C:
			emit()
		}
	}
}

func (t *CacheTracker) currentSize() int64 {
	entries, err := os.ReadDir(t.cacheDir)
	if err != nil {
		return 0
	}
	var largest int64
	needle := strings.ToLower(t.match)
	for _, e := range entries {
		name := strings.ToLower(e.Name())
		if !strings.Contains(name, needle) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.Size() > largest {
			largest = info.Size()
		}
	}
	return largest
}

// resolveBrewCacheDir asks brew where it stores downloads for the given cask
// and returns the containing directory. (`brew --cache --cask X` returns the
// expected full path; we use its parent so we can glob multiple candidates.)
func resolveBrewCacheDir(caskName string) (string, error) {
	out, err := currentRunner().Output("--cache", "--cask", caskName)
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", os.ErrNotExist
	}
	return filepath.Dir(path), nil
}
