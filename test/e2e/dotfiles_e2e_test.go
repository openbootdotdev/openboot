//go:build e2e && vm

// Package e2e contains VM-based E2E tests for the dotfiles clone + stow
// feature.
//
// Gap filled: the VM journey included a `--dotfiles clone` flag in
// TestVM_Journey_FullSetupConfiguresEverything but only verified that
// ~/.dotfiles/.git exists (the clone step). Whether the dotfiles were actually
// linked (stowed / symlinked) into HOME was never asserted.

package e2e

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/testutil"
)

// TestVM_Journey_DotfilesClonedAndLinked runs
//
//	openboot --preset minimal --silent --dotfiles clone --shell skip --macos skip
//
// and verifies that:
//  1. ~/.dotfiles is a valid git repository (clone succeeded).
//  2. At least one file from ~/.dotfiles appears as a symlink in HOME
//     (link/stow step actually ran).
//
// This is the scenario a user experiences when they run openboot and choose
// to set up dotfiles: they expect their dotfiles to be cloned *and* linked,
// not merely downloaded.
// countDotfileSymlinksCmd counts symlinks in HOME that resolve into ~/.dotfiles.
const countDotfileSymlinksCmd = `
count=0
for f in ~/.*; do
    if [ -L "$f" ]; then
        target=$(readlink "$f" 2>/dev/null || true)
        if echo "$target" | grep -q "\.dotfiles"; then
            count=$((count + 1))
        fi
    fi
done
echo "$count"
`

func TestVM_Journey_DotfilesClonedAndLinked(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dotfiles journey test in short mode")
	}

	vm := testutil.NewMacHost(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Capture symlink count BEFORE install so we can detect new symlinks created
	// by the link step (avoids false positives from pre-existing dotfile symlinks).
	beforeOut, _ := vm.Run(countDotfileSymlinksCmd)
	symsBefore := strings.TrimSpace(beforeOut)

	output, err := vmRunDevBinaryWithGit(t, vm, bin,
		"--preset minimal --silent --dotfiles clone --shell skip --macos skip")
	t.Logf("dotfiles setup output:\n%s", output)
	require.NoError(t, err, "install with --dotfiles clone should succeed")

	// ── 1. Clone verification ────────────────────────────────────────────────

	t.Run("dotfiles_git_repo_exists", func(t *testing.T) {
		out, err := vm.Run("test -d ~/.dotfiles/.git && echo git-repo || echo not-a-repo")
		require.NoError(t, err)
		assert.Contains(t, out, "git-repo",
			"~/.dotfiles should be a git repository after clone")
	})

	t.Run("dotfiles_has_remote_origin", func(t *testing.T) {
		out, _ := vm.Run("git -C ~/.dotfiles remote get-url origin 2>/dev/null")
		assert.NotEmpty(t, strings.TrimSpace(out),
			"dotfiles repo should have a remote origin URL")
	})

	// ── 2. Link / stow verification ──────────────────────────────────────────
	//
	// After the clone the installer calls dotfiles.Link() which either runs
	// `stow` (if the repo has stow packages) or creates direct symlinks from
	// files starting with "." in the repo root.  We compare symlink counts
	// BEFORE vs AFTER to avoid a false positive from pre-existing symlinks
	// that were already present on the CI runner.

	t.Run("link_step_created_new_symlinks_in_home", func(t *testing.T) {
		// The count was captured before the install above.
		after, _ := vm.Run(countDotfileSymlinksCmd)
		afterCount := strings.TrimSpace(after)
		assert.NotEqual(t, symsBefore, afterCount,
			"link step must create new symlinks pointing into ~/.dotfiles "+
				"(before=%s after=%s)", symsBefore, afterCount)
	})

	// ── 3. Re-run is idempotent ──────────────────────────────────────────────
	//
	// Running `--dotfiles clone` a second time should not fail (the installer
	// detects the existing repo and syncs it instead of cloning fresh).

	t.Run("second_install_is_idempotent", func(t *testing.T) {
		_, err := vmRunDevBinaryWithGit(t, vm, bin,
			"--preset minimal --silent --dotfiles clone --shell skip --macos skip")
		assert.NoError(t, err,
			"running --dotfiles clone a second time should not fail")
	})
}

// TestVM_Journey_DotfilesLink_OnlyLinks runs
//
//	openboot --preset minimal --silent --dotfiles link --shell skip --macos skip
//
// when ~/.dotfiles already exists (from a previous clone), verifying that the
// link-only mode does not re-clone but still creates symlinks.
func TestVM_Journey_DotfilesLink_OnlyLinks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dotfiles link-only journey test in short mode")
	}

	vm := testutil.NewMacHost(t)
	vmInstallHomebrew(t, vm)
	bin := vmCopyDevBinary(t, vm)

	// Pre-clone so the dotfiles directory exists.
	_, err := vmRunDevBinaryWithGit(t, vm, bin,
		"--preset minimal --silent --dotfiles clone --shell skip --macos skip")
	require.NoError(t, err, "pre-clone should succeed")

	// Record the current origin commit to confirm link-only does not fetch.
	commitBefore, _ := vm.Run("git -C ~/.dotfiles rev-parse HEAD 2>/dev/null")

	_, err = vmRunDevBinaryWithGit(t, vm, bin,
		"--preset minimal --silent --dotfiles link --shell skip --macos skip")
	require.NoError(t, err, "--dotfiles link should succeed when repo exists")

	t.Run("repo_commit_unchanged", func(t *testing.T) {
		// The link-only path should not touch commits (no fetch/reset).
		commitAfter, _ := vm.Run("git -C ~/.dotfiles rev-parse HEAD 2>/dev/null")
		assert.Equal(t, strings.TrimSpace(commitBefore), strings.TrimSpace(commitAfter),
			"--dotfiles link must not change the commit")
	})

	t.Run("symlinks_still_present", func(t *testing.T) {
		out, _ := vm.Run(`
			count=0
			for f in ~/.*; do
				if [ -L "$f" ]; then
					target=$(readlink "$f" 2>/dev/null || true)
					if echo "$target" | grep -q "\.dotfiles"; then
						count=$((count + 1))
					fi
				fi
			done
			echo "$count"
		`)
		assert.NotEqual(t, "0", strings.TrimSpace(out),
			"symlinks should still exist after --dotfiles link")
	})
}
