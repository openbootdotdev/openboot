// Package logging provides always-on structured logging to a rotating file
// under ~/.openboot/logs/, independent of the --verbose flag (which controls
// stderr verbosity only).
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// retentionDays is the number of days to keep log files before deleting them
// during startup cleanup.
const retentionDays = 14

// logDir resolves the directory where log files are stored. It is a package
// variable so tests can override it without touching the user's home dir.
var logDir = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".openboot", "logs"), nil
}

// now returns the current time. It is a package variable so tests can inject
// a deterministic clock.
var now = time.Now

// cleanupWG lets tests deterministically wait for the background retention
// goroutine to finish before asserting on file state.
var cleanupWG sync.WaitGroup

// fallbackReported guards the "once per process" stderr message we emit when
// the log file cannot be opened. It's a pointer so tests can reset it between
// runs.
var fallbackReported = &sync.Once{}

// multiHandler fans out slog records to a set of child handlers, letting us
// capture every record to the log file while filtering stderr separately.
type multiHandler struct {
	handlers []slog.Handler
}

func newMultiHandler(handlers ...slog.Handler) *multiHandler {
	return &multiHandler{handlers: handlers}
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	for _, h := range m.handlers {
		if !h.Enabled(ctx, r.Level) {
			continue
		}
		if err := h.Handle(ctx, r.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: next}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: next}
}

// Init configures slog.SetDefault to write to a multi-handler that always
// captures debug-level records to a daily log file under ~/.openboot/logs/,
// and mirrors records to stderr at LevelDebug (when verbose) or LevelWarn
// otherwise. It returns a closer that flushes and closes the log file.
//
// If the log file cannot be opened (permissions, read-only FS, etc.), Init
// falls back to stderr-only logging and reports the failure once.
func Init(version string, verbose bool) (func(), error) {
	stderrLevel := slog.LevelWarn
	if verbose {
		stderrLevel = slog.LevelDebug
	}
	stderrHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: stderrLevel})

	dir, dirErr := logDir()
	var (
		file    *os.File
		openErr error
	)
	if dirErr == nil {
		file, openErr = openLogFile(dir)
	} else {
		openErr = dirErr
	}

	var handler slog.Handler
	closer := func() {}
	if openErr != nil || file == nil {
		handler = stderrHandler
		reportFallback(openErr)
	} else {
		fileHandler := slog.NewJSONHandler(file, &slog.HandlerOptions{Level: slog.LevelDebug})
		handler = newMultiHandler(fileHandler, stderrHandler)
		closer = func() {
			_ = file.Sync()
			_ = file.Close()
		}
	}

	slog.SetDefault(slog.New(handler))
	slog.Info("session_start",
		"version", version,
		"pid", os.Getpid(),
		"args", os.Args,
	)

	if openErr == nil && file != nil {
		cleanupWG.Add(1)
		go func() {
			defer cleanupWG.Done()
			pruneOldLogs(dir, now())
		}()
	}

	return closer, nil
}

// WaitForCleanup blocks until any pending retention goroutines have finished.
// Exported for tests; production callers don't need to call this.
func WaitForCleanup() {
	cleanupWG.Wait()
}

// openLogFile ensures dir exists with 0700 and opens today's log file with
// 0600 in append mode.
func openLogFile(dir string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	// Tighten permissions in case dir already existed with a wider mode.
	if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // log dir needs owner-exec bit to be traversable
		return nil, fmt.Errorf("chmod log dir: %w", err)
	}

	name := fmt.Sprintf("openboot-%s.log", now().Format("2006-01-02"))
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	return f, nil
}

// pruneOldLogs removes openboot-*.log files whose mtime is older than
// retentionDays before ref. Errors are swallowed — retention is best-effort.
func pruneOldLogs(dir string, ref time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := ref.Add(-retentionDays * 24 * time.Hour)
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "openboot-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		full := filepath.Join(dir, name)
		info, err := os.Stat(full)
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(full)
		}
	}
}

func reportFallback(err error) {
	if err == nil {
		return
	}
	fallbackReported.Do(func() {
		fmt.Fprintf(os.Stderr, "openboot: file logging disabled, using stderr only: %v\n", err)
	})
}
