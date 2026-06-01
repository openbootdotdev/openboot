package npm

import (
	"context"
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
	return exec.Command("npm", args...).Output() //nolint:gosec // "npm" is a hardcoded binary; args are package names from validated config
}

func (execRunner) CombinedOutput(args ...string) ([]byte, error) {
	return exec.Command("npm", args...).CombinedOutput() //nolint:gosec // "npm" is a hardcoded binary; args are package names from validated config
}

func (execRunner) OutputContext(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "npm", args...).Output() //nolint:gosec // "npm" is a hardcoded binary; args are package names from validated config
}

func (execRunner) CombinedOutputContext(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "npm", args...).CombinedOutput() //nolint:gosec // "npm" is a hardcoded binary; args are package names from validated config
}

type contextRunner interface {
	OutputContext(ctx context.Context, args ...string) ([]byte, error)
	CombinedOutputContext(ctx context.Context, args ...string) ([]byte, error)
}

func runnerOutputContext(ctx context.Context, args ...string) ([]byte, error) {
	r := currentRunner()
	if cr, ok := r.(contextRunner); ok {
		return cr.OutputContext(ctx, args...)
	}
	return r.Output(args...)
}

func runnerCombinedOutputContext(ctx context.Context, args ...string) ([]byte, error) {
	r := currentRunner()
	if cr, ok := r.(contextRunner); ok {
		return cr.CombinedOutputContext(ctx, args...)
	}
	return r.CombinedOutput(args...)
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
