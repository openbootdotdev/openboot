package installer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/macos"
)

// Section numbers used to be written into each step's own header string —
// "Step 1: Git Configuration", "Step 4: Installation", "Step 6: Dotfiles" —
// while whether a step ran was decided separately, by its own guard. A real
// run printed 1, 4, 6, 7 with two unnumbered sections wedged in. These tests
// pin the invariant that makes that impossible: numbering is derived from the
// plan, so it is always dense and always matches what actually runs.

func stepNames(plan InstallPlan) []string {
	var out []string
	for _, s := range plannedSteps(plan) {
		out = append(out, s.name)
	}
	return out
}

func TestPlannedStepsAreDenselyNumbered(t *testing.T) {
	full := InstallPlan{
		Formulae:       []string{"jq"},
		Npm:            []string{"typescript"},
		InstallOhMyZsh: true,
		DotfilesURL:    "https://github.com/x/dotfiles",
		MacOSPrefs:     make([]macos.Preference, 3),
		PostInstall:    []string{"echo hi"},
	}
	steps := plannedSteps(full)
	require.Len(t, steps, 7, "git, packages, npm, shell, dotfiles, macOS, post-install")

	var titles []string
	for i, s := range steps {
		titles = append(titles, sectionTitle(i, len(steps), s.name))
	}
	assert.Equal(t, "[1/7] Git identity", titles[0])
	assert.Equal(t, "[7/7] Post-install script", titles[6])
	for i, title := range titles {
		assert.Truef(t, strings.HasPrefix(title, sectionTitle(i, len(steps), "")[:4]),
			"title %q must carry its own index — no gaps", title)
	}
}

// A step that won't run must not consume a number: this is the exact failure
// the old hardcoded strings had.
func TestPlannedStepsSkipWhatWontRun(t *testing.T) {
	packagesOnly := InstallPlan{
		PackagesOnly:   true,
		Formulae:       []string{"jq"},
		InstallOhMyZsh: true,                            // ignored under PackagesOnly
		DotfilesURL:    "https://github.com/x/dotfiles", // ignored
		MacOSPrefs:     make([]macos.Preference, 3),     // ignored
	}
	assert.Equal(t, []string{"Packages"}, stepNames(packagesOnly),
		"--packages-only runs exactly one section, numbered 1/1")

	noGit := InstallPlan{SkipGit: true, Formulae: []string{"jq"}}
	assert.Equal(t, []string{"Packages"}, stepNames(noGit), "a skipped git step takes no number")

	npmOnly := InstallPlan{SkipGit: true, Npm: []string{"typescript"}}
	assert.Equal(t, []string{"npm globals"}, stepNames(npmOnly),
		"npm gets a real numbered section — it had no number at all before")

	assert.Empty(t, stepNames(InstallPlan{SkipGit: true}), "an empty plan runs nothing")
}

// The step list is the execution order the user reads; lock it.
func TestPlannedStepsOrder(t *testing.T) {
	assert.Equal(t,
		[]string{"Git identity", "Packages", "npm globals", "Shell", "Dotfiles", "macOS preferences", "Post-install script"},
		stepNames(InstallPlan{
			Formulae: []string{"jq"}, Npm: []string{"ts"}, InstallOhMyZsh: true,
			DotfilesURL: "u", MacOSPrefs: make([]macos.Preference, 1), PostInstall: []string{"x"},
		}))
}
