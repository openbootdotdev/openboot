package brew

import (
	"os"
	"os/exec"
	"sync"
)

// Runner is the swappable executor for brew subcommands. The package uses a
// default implementation that invokes the real `brew` binary; tests replace it
// with a fake runner to avoid fork/exec overhead.
//
// Only the common patterns are covered here. Complex cases (install progress
// streaming, TTY-wrapped sudo prompts) still use exec.Command directly.
type Runner interface {
	// Output runs `brew args...` and returns stdout only.
	Output(args ...string) ([]byte, error)

	// CombinedOutput runs `brew args...` and returns stdout+stderr merged.
	CombinedOutput(args ...string) ([]byte, error)

	// Run runs `brew args...` with stdout/stderr attached to the process,
	// so the user sees progress output. Returns the exit error.
	Run(args ...string) error
}

type execRunner struct{}

func (execRunner) Output(args ...string) ([]byte, error) {
	return exec.Command("brew", args...).Output()
}

func (execRunner) CombinedOutput(args ...string) ([]byte, error) {
	return exec.Command("brew", args...).CombinedOutput()
}

func (execRunner) Run(args ...string) error {
	cmd := exec.Command("brew", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
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
