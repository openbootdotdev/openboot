//go:build e2e && !vm

// E2E smoke test for the full-screen install wizard (TUI Redesign v5).
//
// Gap filled: the wizard's model logic is unit-tested in
// internal/ui/tui/wizard, but nothing exercised the compiled binary on a real
// pseudo-terminal — alt-screen entry/exit, the global stdout redirect in
// wizard.Run, and clean teardown only manifest with a TTY attached. This test
// drives the binary under script(1) (present on every macOS host), waits for
// the boot probes to finish, quits with 'q', and asserts the wizard rendered
// and restored the terminal. It never confirms an install, so nothing is
// installed; boot probes are read-only.
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

func TestE2E_InstallWizard_LaunchRenderQuit(t *testing.T) {
	binary := testutil.BuildTestBinary(t)
	home := t.TempDir() // fresh HOME: no sync source, no state → wizard branch

	// Dead localhost API: catalog refresh fails fast and falls back to the
	// embedded catalog, keeping the test hermetic.
	env := isolatedEnv(home, "http://localhost:1")
	env = append(env, "TERM=xterm-256color")

	// script(1) allocates a pty; size it first so the wizard has a viewport
	// (bubbletea renders nothing at 0x0).
	cmd := exec.Command("script", "-q", "/dev/null",
		"sh", "-c", "stty rows 30 cols 100; exec "+binary+" install")
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	var out syncBuffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	require.NoError(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	// Wait for the boot probes to complete (loadout list appears). Generous
	// deadline: the installed-tools scan runs real `brew list` which can be
	// slow on a cold CI runner.
	waitFor := func(marker string, timeout time.Duration) bool {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			if strings.Contains(out.String(), marker) {
				return true
			}
			select {
			case <-done:
				return strings.Contains(out.String(), marker)
			case <-time.After(200 * time.Millisecond):
			}
		}
		return false
	}

	require.True(t, waitFor("Choose a starting point", 90*time.Second),
		"boot screen should reach the loadout list; output:\n%s", out.String())

	// Quit from the loadout screen — never confirm an install.
	_, err = io.WriteString(stdin, "q")
	require.NoError(t, err)

	select {
	case waitErr := <-done:
		assert.NoError(t, waitErr, "binary should exit 0 after q")
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("wizard did not exit within 15s of pressing q; output:\n%s", out.String())
	}

	got := out.String()
	// Rendered the boot screen with real probe results and the preset list.
	assert.Contains(t, got, "openboot install", "status bar rendered")
	assert.Contains(t, got, "Minimal", "loadout list rendered")
	assert.Contains(t, got, "Developer", "loadout list rendered")
	// Entered and left the alternate screen — terminal restored.
	assert.Contains(t, got, "\x1b[?1049h", "entered alt-screen")
	assert.Contains(t, got, "\x1b[?1049l", "left alt-screen (terminal restored)")
	// Fresh HOME must not route to the sync flow.
	assert.NotContains(t, got, "Syncing with", "fresh HOME must take the wizard branch")
}
