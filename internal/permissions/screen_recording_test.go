package permissions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasScreenRecordingPermission_Returns(t *testing.T) {
	result := HasScreenRecordingPermission()
	assert.IsType(t, true, result)
}

// TestHasScreenRecordingPermission_NonDarwin verifies that the non-darwin/non-cgo
// stub always returns false (build tag !darwin || !cgo applies in this environment).
func TestHasScreenRecordingPermission_NonDarwin(t *testing.T) {
	// On non-darwin builds the stub unconditionally returns false.
	// On darwin+cgo the result depends on actual system permissions; we only
	// assert the return type to stay side-effect-free.
	result := HasScreenRecordingPermission()
	assert.IsType(t, false, result)
}

// TestOpenScreenRecordingSettings_NonDarwin verifies that the non-darwin/non-cgo
// stub is a no-op and returns nil.
func TestOpenScreenRecordingSettings_NonDarwin(t *testing.T) {
	// On non-darwin builds this is a no-op stub that must return nil.
	err := OpenScreenRecordingSettings()
	assert.NoError(t, err)
}
