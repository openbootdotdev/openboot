package ui

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsScrollRegionSupported(t *testing.T) {
	tests := []struct {
		name     string
		term     string
		rows     int
		expected bool
	}{
		{"xterm-ghostty rows OK", "xterm-ghostty", 24, true},
		{"xterm-256color rows OK", "xterm-256color", 24, true},
		{"terminal exactly at threshold", "xterm-256color", 6, true},
		{"dumb terminal", "dumb", 24, false},
		{"empty TERM", "", 24, false},
		{"terminal too short", "xterm-256color", 4, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isScrollRegionSupportedFor(tt.term, tt.rows, true)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestIsScrollRegionSupportedNonTTY(t *testing.T) {
	// Even with good TERM and rows, non-TTY disables.
	got := isScrollRegionSupportedFor("xterm-256color", 24, false)
	assert.False(t, got)
}

func TestScrollRegionReserveEmitsSequence(t *testing.T) {
	var buf bytes.Buffer
	r := &ScrollRegion{w: &buf, rows: 24, cols: 80, reserved: 2}
	r.reserve()
	out := buf.String()
	// Expected: hide cursor, set scroll region rows 1..(24-2)=22, move cursor
	// to the last row of the scrollable area (row 22).
	assert.Contains(t, out, "\x1b[?25l")
	assert.Contains(t, out, "\x1b[1;22r")
	assert.Contains(t, out, "\x1b[22;1H")
}

func TestScrollRegionResetClearsReservedRowsAndReleasesRegion(t *testing.T) {
	var buf bytes.Buffer
	r := &ScrollRegion{w: &buf, rows: 24, cols: 80, reserved: 2}
	r.reset()
	out := buf.String()
	// Reset must:
	//  1. clear each reserved row (rows 23 and 24 with EL2 = \x1b[2K)
	//  2. release the scroll region (\x1b[r)
	//  3. show the cursor (\x1b[?25h)
	//  4. preserve the caller's cursor position via DEC save/restore
	assert.Contains(t, out, "\x1b7")
	assert.Contains(t, out, "\x1b[23;1H\x1b[2K")
	assert.Contains(t, out, "\x1b[24;1H\x1b[2K")
	assert.Contains(t, out, "\x1b8")
	assert.Contains(t, out, "\x1b[r")
	assert.Contains(t, out, "\x1b[?25h")
}

func TestScrollRegionDrawBottomUsesSaveRestoreCursor(t *testing.T) {
	var buf bytes.Buffer
	r := &ScrollRegion{w: &buf, rows: 24, cols: 80, reserved: 2}
	r.drawBottom([]string{"line one", "line two"})
	out := buf.String()
	// DEC save (\x1b7) before, DEC restore (\x1b8) after.
	// With rows=24, reserved=2: lines write to rows 23 and 24.
	assert.Contains(t, out, "\x1b7")
	assert.Contains(t, out, "\x1b8")
	assert.Contains(t, out, "\x1b[23;1H")
	assert.Contains(t, out, "\x1b[24;1H")
	assert.Contains(t, out, "line one")
	assert.Contains(t, out, "line two")
}

func TestScrollRegionDrawBottomTooManyLinesPanics(t *testing.T) {
	var buf bytes.Buffer
	r := &ScrollRegion{w: &buf, rows: 24, cols: 80, reserved: 2}
	assert.Panics(t, func() {
		r.drawBottom([]string{"a", "b", "c"}) // reserved=2, given 3
	})
}
