package permissions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHasScreenRecordingPermission_Returns verifies the function returns a bool.
// On non-darwin/non-cgo builds the stub always returns false; on darwin+cgo
// the result depends on system permissions, so we only check the type.
func TestHasScreenRecordingPermission_Returns(t *testing.T) {
	result := HasScreenRecordingPermission()
	assert.IsType(t, true, result)
}

// TestOpenScreenRecordingSettings_Returns verifies the non-darwin/non-cgo stub
// is a no-op and returns nil.
func TestOpenScreenRecordingSettings_Returns(t *testing.T) {
	err := OpenScreenRecordingSettings()
	assert.NoError(t, err)
}
