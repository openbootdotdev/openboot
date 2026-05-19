package brew

import (
	"context"
	"os"
	"strings"
	"time"
)

// CacheKind disambiguates the brew --cache lookup. Formulae and casks share
// a global name space where some tokens collide (e.g. `docker` is both), so
// we must pass --formula or --cask explicitly when resolving the cache path.
type CacheKind int

const (
	CacheKindCask CacheKind = iota
	CacheKindFormula
)

// CacheTracker polls the exact brew download path for a package and reports
// the partial file's current size. During download brew writes to
// `<finalPath>.incomplete`, then renames to `<finalPath>` on success — we
// stat both and report the larger. Works for both formula bottles and cask
// downloads — they follow the same .incomplete → rename convention.
type CacheTracker struct {
	finalPath string
	interval  time.Duration
}

// NewCacheTracker builds a tracker for the given package. The exact cache
// path is resolved via `brew --cache --cask <name>` or
// `brew --cache --formula <name>` per kind.
func NewCacheTracker(name string, kind CacheKind) (*CacheTracker, error) {
	path, err := resolveBrewCachePath(name, kind)
	if err != nil {
		return nil, err
	}
	return &CacheTracker{
		finalPath: path,
		interval:  500 * time.Millisecond,
	}, nil
}

// Run polls every interval and invokes onProgress with the current file
// size. Stops when ctx is done. Always emits at least one final value
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
	var largest int64
	for _, p := range []string{t.finalPath, t.finalPath + ".incomplete"} {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if info.Size() > largest {
			largest = info.Size()
		}
	}
	return largest
}

// resolveBrewCachePath returns the exact path brew uses for the package's
// download. Matching the previous substring-based approach against the
// package name is unreliable: brew names the cached file after the URL's
// basename (e.g. `google-chrome` → `…--googlechrome.dmg`, formula bottles
// embed sha + platform), so the name often doesn't appear in it.
func resolveBrewCachePath(name string, kind CacheKind) (string, error) {
	flag := "--cask"
	if kind == CacheKindFormula {
		flag = "--formula"
	}
	out, err := currentRunner().Output("--cache", flag, name)
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", os.ErrNotExist
	}
	return path, nil
}
