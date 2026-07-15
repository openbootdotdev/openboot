package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// wizardSource is the source-kind half of shouldLaunchWizard, split out so the
// routing is testable without a TTY: bare installs and valid presets get the
// full wizard; unknown presets and remote sources keep the linear path.
func TestWizardSource(t *testing.T) {
	oldPreset := installCfg.Preset
	defer func() { installCfg.Preset = oldPreset }()

	assert.True(t, wizardSource(&installSource{kind: sourceNone}), "bare install")

	installCfg.Preset = "developer"
	assert.True(t, wizardSource(&installSource{kind: sourcePreset}), "valid preset")

	installCfg.Preset = "not-a-preset"
	assert.False(t, wizardSource(&installSource{kind: sourcePreset}), "unknown preset keeps the linear error path")

	assert.False(t, wizardSource(&installSource{kind: sourceCloud}), "cloud config")
	assert.False(t, wizardSource(&installSource{kind: sourceSyncSource}), "sync source")
	assert.False(t, wizardSource(&installSource{kind: sourceFile}), "local file")
}
