//go:build darwin && cgo

package permissions

import (
	"fmt"
	"os/exec"
)

/*
#cgo LDFLAGS: -framework CoreGraphics
#include <CoreGraphics/CoreGraphics.h>
*/
import "C"

func HasScreenRecordingPermission() bool {
	return bool(C.CGPreflightScreenCaptureAccess())
}

func OpenScreenRecordingSettings() error {
	cmd := exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("open screen recording settings: %w", err)
	}
	return nil
}
