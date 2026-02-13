//go:build !darwin || !cgo

package permissions

func HasScreenRecordingPermission() bool {
	return false
}

func OpenScreenRecordingSettings() error {
	return nil
}
