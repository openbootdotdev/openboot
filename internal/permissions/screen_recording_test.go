//go:build !darwin || !cgo

// These tests cover the stub implementations only. The darwin+cgo path calls
// CoreGraphics' CGPreflightScreenCaptureAccess (which registers the test
// binary with macOS TCC) and `open x-apple.systempreferences:...` (which
// pops up System Settings) — both have real side effects and violate the
// L1 "no real network, no real fork" contract. Coverage for the cgo path
// belongs in L5 / manual verification.

package permissions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasScreenRecordingPermission_StubReturnsFalse(t *testing.T) {
	assert.False(t, HasScreenRecordingPermission())
}

func TestOpenScreenRecordingSettings_StubIsNoOp(t *testing.T) {
	assert.NoError(t, OpenScreenRecordingSettings())
}
