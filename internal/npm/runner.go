package npm

import "os/exec"

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

var runner Runner = execRunner{}

// SetRunner replaces the runner. Returns a restore function intended for
// t.Cleanup. Test-only.
func SetRunner(r Runner) (restore func()) {
	prev := runner
	runner = r
	return func() { runner = prev }
}
