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
	// Expected: hide cursor, set scroll region rows 3..24, move cursor into region.
	assert.Contains(t, buf.String(), "\x1b[?25l")
	assert.Contains(t, buf.String(), "\x1b[3;24r")
	assert.Contains(t, buf.String(), "\x1b[3;1H")
}

func TestScrollRegionResetEmitsSequence(t *testing.T) {
	var buf bytes.Buffer
	r := &ScrollRegion{w: &buf, rows: 24, cols: 80, reserved: 2}
	r.reset()
	// Expected: reset scroll region, show cursor, move cursor to bottom.
	assert.Contains(t, buf.String(), "\x1b[r")
	assert.Contains(t, buf.String(), "\x1b[?25h")
	assert.Contains(t, buf.String(), "\x1b[24;1H")
}

func TestScrollRegionDrawTopUsesSaveRestoreCursor(t *testing.T) {
	var buf bytes.Buffer
	r := &ScrollRegion{w: &buf, rows: 24, cols: 80, reserved: 2}
	r.drawTop([]string{"line one", "line two"})
	out := buf.String()
	// DEC save (\x1b7) before, DEC restore (\x1b8) after.
	assert.Contains(t, out, "\x1b7")
	assert.Contains(t, out, "\x1b8")
	assert.Contains(t, out, "\x1b[1;1H")
	assert.Contains(t, out, "\x1b[2;1H")
	assert.Contains(t, out, "line one")
	assert.Contains(t, out, "line two")
}

func TestScrollRegionDrawTopTooManyLinesPanics(t *testing.T) {
	var buf bytes.Buffer
	r := &ScrollRegion{w: &buf, rows: 24, cols: 80, reserved: 2}
	assert.Panics(t, func() {
		r.drawTop([]string{"a", "b", "c"}) // reserved=2, given 3
	})
}
