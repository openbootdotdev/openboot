//go:build e2e && !vm

// E2E tests for the full-screen install wizard (TUI Redesign v5), driven on a
// real pseudo-terminal via script(1) (present on every macOS host).
//
// Gap filled: the wizard's model logic is unit-tested in
// internal/ui/tui/wizard, but nothing exercised the compiled binary on a real
// pty — alt-screen entry/exit, the global stdout redirect in wizard.Run, and
// clean teardown only manifest with a TTY attached. Following the lazygit
// integration-test discipline, every step waits for an on-screen marker before
// sending keys — no fixed sleeps.
//
// These tests never confirm an install: the smoke test quits from the loadout
// screen, and the choreography test walks boot → select → filter → toggle →
// git identity and quits right before the final confirm. Boot probes are
// read-only. The destructive counterpart that lets the install run for real
// lives in install_wizard_vm_test.go (L4, CI only).
package e2e

import (
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/testutil"
)

// syncBuffer is a goroutine-safe writer the pty output streams into while the
// test polls it for render markers.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// wizardSession is a live wizard process on a pty.
type wizardSession struct {
	stdin io.WriteCloser
	out   *syncBuffer
	done  chan error
	cmd   *exec.Cmd
}

// startWizardPty launches `<binary> install` under script(1) with a sized pty.
func startWizardPty(t *testing.T, binary string, env []string) *wizardSession {
	t.Helper()

	// script(1) allocates a pty; size it first so the wizard has a viewport
	// (bubbletea renders nothing at 0x0).
	cmd := exec.Command("script", "-q", "/dev/null",
		"sh", "-c", "stty rows 30 cols 100; exec "+binary+" install")
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	out := &syncBuffer{}
	cmd.Stdout = out
	cmd.Stderr = out

	require.NoError(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	return &wizardSession{stdin: stdin, out: out, done: done, cmd: cmd}
}

// waitFor polls the pty output until marker appears. Markers must lie within a
// single styled span so ANSI codes don't split them.
func (s *wizardSession) waitFor(marker string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(s.out.String(), marker) {
			return true
		}
		select {
		case <-s.done:
			return strings.Contains(s.out.String(), marker)
		case <-time.After(200 * time.Millisecond):
		}
	}
	return false
}

func (s *wizardSession) send(t *testing.T, keys string) {
	t.Helper()
	_, err := io.WriteString(s.stdin, keys)
	require.NoError(t, err)
}

// sendPaced writes each segment as its own keystroke burst with a small gap in
// between, so mode-switching keys ("/", tab) land as their own key events
// instead of being coalesced with the following text. The gaps pace input;
// state waiting still goes through waitFor markers.
func (s *wizardSession) sendPaced(t *testing.T, segments ...string) {
	t.Helper()
	for _, seg := range segments {
		s.send(t, seg)
		time.Sleep(50 * time.Millisecond)
	}
}

// expectExit waits for the process to end, killing it on timeout.
func (s *wizardSession) expectExit(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case waitErr := <-s.done:
		assert.NoError(t, waitErr, "binary should exit cleanly")
	case <-time.After(timeout):
		_ = s.cmd.Process.Kill()
		t.Fatalf("wizard did not exit within %s; output:\n%s", timeout, s.out.String())
	}
}

// wizardEnv builds the isolated environment for a wizard run: fresh HOME (no
// sync source → wizard branch), git config redirected to a throwaway file so
// the git-identity screen deterministically appears, dead localhost API so the
// catalog falls back to the embedded copy, and a fixed TERM.
func wizardEnv(home string) []string {
	env := isolatedEnv(home, "http://localhost:1")
	return append(env,
		"TERM=xterm-256color",
		"GIT_CONFIG_GLOBAL="+home+"/gitconfig-test",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
}

// TestE2E_InstallWizard_LaunchRenderQuit: boot screen renders with real probe
// results, q quits from the loadout list, terminal is restored.
func TestE2E_InstallWizard_LaunchRenderQuit(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	s := startWizardPty(t, binary, wizardEnv(t.TempDir()))

	// Generous deadline: the installed-tools scan runs real `brew list`,
	// which can be slow on a cold CI runner.
	require.True(t, s.waitFor("Choose a starting point", 90*time.Second),
		"boot screen should reach the loadout list; output:\n%s", s.out.String())

	s.send(t, "q")
	s.expectExit(t, 15*time.Second)

	got := s.out.String()
	assert.Contains(t, got, "openboot install", "status bar rendered")
	assert.Contains(t, got, "Minimal", "loadout list rendered")
	assert.Contains(t, got, "Developer", "loadout list rendered")
	// Entered and left the alternate screen — terminal restored.
	assert.Contains(t, got, "\x1b[?1049h", "entered alt-screen")
	assert.Contains(t, got, "\x1b[?1049l", "left alt-screen (terminal restored)")
	// Fresh HOME must not route to the sync flow.
	assert.NotContains(t, got, "Syncing with", "fresh HOME must take the wizard branch")
}

// TestE2E_InstallWizard_FullChoreography drives every keystroke of the real
// install path — hand-pick, filter, toggle, confirm, git identity — and quits
// with ctrl+c right before the final confirm, so nothing is installed. This
// pins the exact key sequence the L4 real-install test replays destructively.
func TestE2E_InstallWizard_FullChoreography(t *testing.T) {
	formula := wizardTestFormula(t)
	t.Logf("driving selection with formula %q", formula)

	binary := testutil.BuildTestBinary(t)
	s := startWizardPty(t, binary, wizardEnv(t.TempDir()))

	require.True(t, s.waitFor("Choose a starting point", 90*time.Second),
		"boot: loadout list; output:\n%s", s.out.String())
	s.send(t, "c") // hand-pick: empty selection

	require.True(t, s.waitFor("type to filter", 10*time.Second),
		"select: filter placeholder; output:\n%s", s.out.String())
	s.sendPaced(t, "/", formula, "\r") // filter, enter toggles the single hit

	require.True(t, s.waitFor("1 pkgs", 10*time.Second),
		"status bar should show 1 package selected; output:\n%s", s.out.String())
	s.send(t, "\r") // proceed → git screen (no identity in isolated config)

	require.True(t, s.waitFor("Set your git identity", 10*time.Second),
		"git capture screen; output:\n%s", s.out.String())
	s.sendPaced(t, "CI Bot", "\t", "ci@example.com")

	require.True(t, s.waitFor("ci@example.com", 10*time.Second),
		"email field rendered; output:\n%s", s.out.String())
	s.send(t, "\r") // → review screen

	require.True(t, s.waitFor("Ready to install", 10*time.Second),
		"confirm screen; output:\n%s", s.out.String())

	// Stop here — the next enter would start a real install (L4's job).
	s.send(t, "\x03")
	s.expectExit(t, 15*time.Second)

	got := s.out.String()
	assert.Contains(t, got, "GIT", "git screen status badge rendered")
	assert.Contains(t, got, "REVIEW", "confirm screen status badge rendered")
	assert.Contains(t, got, "\x1b[?1049l", "terminal restored")
}
