package brew

import (
	"os"
	"os/exec"
	"sync"

	"github.com/openbootdotdev/openboot/internal/system"
)

// Runner is the swappable executor for brew subcommands. The package uses a
// default implementation that invokes the real `brew` binary; tests replace it
// with a fake runner to avoid fork/exec overhead.
//
// Coverage notes — the following call sites remain outside the Runner because
// they need features Runner does not express cleanly:
//   - progress-stream install path (brew_install.go: brewInstallCmd / Install /
//     InstallCask / installCaskWithProgress / brewCombinedOutputWithTTY /
//     installFormulaWithError / installSmartCaskWithError) — these rely on
//     the HOMEBREW_NO_AUTO_UPDATE env var plus custom stdout pipe wiring for
//     StickyProgress and TTY stdin for sudo prompts.
type Runner interface {
	// Output runs `brew args...` and returns stdout only.
	Output(args ...string) ([]byte, error)

	// CombinedOutput runs `brew args...` and returns stdout+stderr merged.
	CombinedOutput(args ...string) ([]byte, error)

	// Run runs `brew args...` with stdout/stderr attached to the process,
	// so the user sees progress output. Stdin is NOT attached. Returns the
	// exit error.
	Run(args ...string) error

	// RunInteractive runs `brew args...` attached to the current TTY
	// (stdin+stdout+stderr) so subcommands like `brew upgrade` that may
	// prompt for a sudo password work correctly. If /dev/tty cannot be
	// opened, falls back to os.Stdin. Returns the exit error.
	RunInteractive(args ...string) error
}

type execRunner struct{}

func (execRunner) Output(args ...string) ([]byte, error) {
	return exec.Command("brew", args...).Output() //nolint:gosec // "brew" is hardcoded; args are package names
}

func (execRunner) CombinedOutput(args ...string) ([]byte, error) {
	return exec.Command("brew", args...).CombinedOutput() //nolint:gosec // "brew" is hardcoded; args are package names
}

func (execRunner) Run(args ...string) error {
	cmd := exec.Command("brew", args...) //nolint:gosec // "brew" is hardcoded; args are package names
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (execRunner) RunInteractive(args ...string) error {
	cmd := exec.Command("brew", args...) //nolint:gosec // "brew" is hardcoded; args are package names
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	tty, opened := system.OpenTTY()
	if opened {
		defer tty.Close() //nolint:errcheck // best-effort TTY cleanup
	}
	cmd.Stdin = tty
	return cmd.Run()
}

var (
	runnerMu sync.RWMutex
	runner   Runner = execRunner{}
)

func currentRunner() Runner {
	runnerMu.RLock()
	defer runnerMu.RUnlock()
	return runner
}

// SetRunner replaces the runner. Returns a restore function intended for
// t.Cleanup. Test-only.
func SetRunner(r Runner) (restore func()) {
	runnerMu.Lock()
	prev := runner
	runner = r
	runnerMu.Unlock()
	return func() {
		runnerMu.Lock()
		runner = prev
		runnerMu.Unlock()
	}
}
