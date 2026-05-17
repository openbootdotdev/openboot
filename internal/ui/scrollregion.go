package ui

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// minRowsForRegion is the smallest terminal height where reserving 2 sticky
// rows still leaves a usable scroll region. Tiny terminals fall back to the
// non-region path.
const minRowsForRegion = 6

// ScrollRegion owns the ANSI plumbing for reserving the top N rows as a
// "frozen" region while output below scrolls. Use IsScrollRegionSupported to
// check before enabling; on unsupported terminals, callers should render
// inline instead.
type ScrollRegion struct {
	w        io.Writer
	rows     int
	cols     int
	reserved int
	active   bool
}

// NewScrollRegion constructs a scroll region for stdout with the given number
// of reserved top rows. Caller must invoke Start before drawing and Stop on
// teardown (defer / signal handler).
func NewScrollRegion(reserved int) *ScrollRegion {
	w, h := terminalSize()
	return &ScrollRegion{
		w:        os.Stdout,
		rows:     h,
		cols:     w,
		reserved: reserved,
	}
}

// Start reserves the region and hides the cursor.
func (r *ScrollRegion) Start() {
	r.reserve()
	r.active = true
}

// Stop resets the scroll region and shows the cursor. Idempotent.
func (r *ScrollRegion) Stop() {
	if !r.active {
		return
	}
	r.reset()
	r.active = false
}

// DrawTop writes lines to the reserved region without disturbing the cursor
// in the scrolling area. len(lines) must be <= reserved.
func (r *ScrollRegion) DrawTop(lines []string) {
	r.drawTop(lines)
}

// Cols returns the current terminal width.
func (r *ScrollRegion) Cols() int { return r.cols }

// write is the internal sink for ANSI control sequences. Terminal writes
// are best-effort — a failed write to stdout is not recoverable and
// silently dropping the error matches every other UI helper in this file.
func (r *ScrollRegion) write(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(r.w, format, args...)
}

func (r *ScrollRegion) reserve() {
	// Hide cursor, set scroll region from row (reserved+1) to last row,
	// then move cursor into the scroll region.
	r.write("\x1b[?25l")
	r.write("\x1b[%d;%dr", r.reserved+1, r.rows)
	r.write("\x1b[%d;1H", r.reserved+1)
}

func (r *ScrollRegion) reset() {
	// Reset scroll region, show cursor, move cursor to the bottom row.
	r.write("\x1b[r")
	r.write("\x1b[?25h")
	r.write("\x1b[%d;1H", r.rows)
}

func (r *ScrollRegion) drawTop(lines []string) {
	if len(lines) > r.reserved {
		panic(fmt.Sprintf("scrollregion: %d lines exceed reserved %d", len(lines), r.reserved))
	}
	// DEC save cursor, draw each reserved row from top, restore.
	r.write("\x1b7")
	for i, line := range lines {
		r.write("\x1b[%d;1H\x1b[2K%s", i+1, line)
	}
	r.write("\x1b8")
}

// IsScrollRegionSupported reports whether the current terminal can use the
// scroll-region rendering path. Falls back to false if TERM is dumb / empty,
// stdout is not a TTY, or the terminal is too short.
func IsScrollRegionSupported() bool {
	_, h := terminalSize()
	return isScrollRegionSupportedFor(os.Getenv("TERM"), h, term.IsTerminal(int(os.Stdout.Fd()))) //nolint:gosec // os.Stdout.Fd() returns a valid file descriptor; uintptr fits in int on all supported platforms
}

func isScrollRegionSupportedFor(termEnv string, rows int, isTTY bool) bool {
	if !isTTY {
		return false
	}
	if termEnv == "" || termEnv == "dumb" {
		return false
	}
	if rows < minRowsForRegion {
		return false
	}
	return true
}

func terminalSize() (cols, rows int) {
	w, h, err := term.GetSize(int(os.Stdout.Fd())) //nolint:gosec // os.Stdout.Fd() returns a valid file descriptor; uintptr fits in int on all supported platforms
	if err != nil || w <= 0 || h <= 0 {
		return 80, 24
	}
	return w, h
}
