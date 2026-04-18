package npm

import (
	"os/exec"
	"sync"
)

// Runner is the swappable executor for npm subcommands. Default uses the real
// npm binary; tests replace it with a fake to avoid fork/exec overhead.
//
// Node invocations (GetNodeVersion) go through exec.Command directly — they
// happen once per run and don't justify their own abstraction.
type Runner interface {
	// Output runs `npm args...` and returns stdout only.
	Output(args ...string) ([]byte, error)
	// CombinedOutput runs `npm args...` and returns stdout+stderr.
	CombinedOutput(args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Output(args ...string) ([]byte, error) {
	return exec.Command("npm", args...).Output()
}

func (execRunner) CombinedOutput(args ...string) ([]byte, error) {
	return exec.Command("npm", args...).CombinedOutput()
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
