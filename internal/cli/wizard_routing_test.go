package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openbootdotdev/openboot/internal/config"
)

// wizardSource is the source-kind half of shouldLaunchWizard, split out so the
// routing is testable without a TTY: bare installs and valid presets get the
// full wizard; unknown presets and remote sources use their dedicated paths.
func TestWizardSource(t *testing.T) {
	oldPreset := installCfg.Preset
	defer func() { installCfg.Preset = oldPreset }()

	assert.True(t, wizardSource(&installSource{kind: sourceNone}), "bare install")

	installCfg.Preset = "developer"
	assert.True(t, wizardSource(&installSource{kind: sourcePreset}), "valid preset")

	installCfg.Preset = "not-a-preset"
	assert.False(t, wizardSource(&installSource{kind: sourcePreset}), "unknown preset is rejected before wizard launch")

	assert.False(t, wizardSource(&installSource{kind: sourceCloud}), "cloud config")
	assert.False(t, wizardSource(&installSource{kind: sourceSyncSource}), "sync source")
	assert.False(t, wizardSource(&installSource{kind: sourceFile}), "local file")
}

func TestWizardMode(t *testing.T) {
	tests := []struct {
		name   string
		cfg    config.Config
		hasTTY bool
		want   bool
	}{
		{name: "interactive install", hasTTY: true, want: true},
		{name: "dry run uses the same mode", cfg: config.Config{InstallOptions: config.InstallOptions{DryRun: true}}, hasTTY: true, want: true},
		{name: "silent", cfg: config.Config{InstallOptions: config.InstallOptions{Silent: true}}, hasTTY: true},
		{name: "update", cfg: config.Config{InstallOptions: config.InstallOptions{Update: true}}, hasTTY: true},
		{name: "non tty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, wizardMode(&tt.cfg, tt.hasTTY))
		})
	}
}

func TestApplyInstallSourceRejectsUnknownPreset(t *testing.T) {
	oldPreset := installCfg.Preset
	defer func() { installCfg.Preset = oldPreset }()

	installCfg.Preset = "not-a-preset"
	err := applyInstallSource(&installSource{kind: sourcePreset})

	assert.EqualError(t, err, `unknown preset "not-a-preset" (available: minimal, developer, full)`)
}
