package permissions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasScreenRecordingPermission_Returns(t *testing.T) {
	result := HasScreenRecordingPermission()
	assert.IsType(t, true, result)
}

func TestOpenScreenRecordingSettings_NoError(t *testing.T) {
	err := OpenScreenRecordingSettings()
	assert.IsType(t, (*error)(nil), &err)
}
