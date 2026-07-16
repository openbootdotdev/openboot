package installer

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/macos"
)

// recordReporter captures Header calls so a test can assert which install
// phases ApplyContext actually entered.
type recordReporter struct{ headers []string }

func (r *recordReporter) Header(msg string) { r.headers = append(r.headers, msg) }
func (r *recordReporter) Info(string)       {}
func (r *recordReporter) Success(string)    {}
func (r *recordReporter) Warn(string)       {}
func (r *recordReporter) Error(string)      {}
func (r *recordReporter) Muted(string)      {}

// A cancelled context must stop ApplyContext before the config steps run, so a
// ctrl+c abort doesn't keep symlinking dotfiles / rewriting macOS defaults after
// the user asked to stop. (M3: the config steps take no ctx, so without the
// ctx.Err() gates they would run to completion regardless of cancellation.)
func TestApplyContextAbortsBeforeConfigStepsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled — simulates ctrl+c landing during the package phase

	plan := InstallPlan{
		SkipGit:        true, // no git, no packages → reach the first ctx gate immediately
		InstallOhMyZsh: true, // would emit "Shell Configuration" if the step ran
		DotfilesURL:    "https://github.com/example/dotfiles",
		MacOSPrefs: []macos.Preference{
			{Domain: "com.apple.dock", Key: "tilesize", Type: "int", Value: "48"},
		},
	}

	rr := &recordReporter{}
	err := ApplyContext(ctx, plan, rr)

	require.ErrorIs(t, err, context.Canceled, "an aborted apply reports the cancellation")
	for _, h := range rr.headers {
		assert.NotContains(t, h, "Shell Config", "aborted install must not run the shell step")
		assert.NotContains(t, h, "Dotfiles", "aborted install must not run dotfiles")
		assert.NotContains(t, h, "macOS Prefer", "aborted install must not run macOS prefs")
	}
	// It's fine to have entered the package header before the gate fired; the
	// point is that no *config* step started.
	assert.NotContains(t, strings.Join(rr.headers, "|"), "Post-Install")
}
