package permissions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasScreenRecordingPermission_Returns(t *testing.T) {
	result := HasScreenRecordingPermission()
	assert.IsType(t, true, result)
}

// TestOpenScreenRecordingSettings is intentionally omitted: the function's only
// effect is opening System Settings UI on macOS, which cannot be meaningfully
// unit-tested without side-effecting the developer's machine.
