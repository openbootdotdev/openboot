package brew

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingRunner captures which Runner method was invoked and with which args
// so tests can assert on routing (Run vs RunInteractive) without fork/exec.
type recordingRunner struct {
	outputCalls         [][]string
	combinedOutputCalls [][]string
	runCalls            [][]string
	runInteractiveCalls [][]string
	runErr              error
	runInteractiveErr   error
}

func (r *recordingRunner) Output(args ...string) ([]byte, error) {
	r.outputCalls = append(r.outputCalls, append([]string(nil), args...))
	return nil, nil
}

func (r *recordingRunner) CombinedOutput(args ...string) ([]byte, error) {
	r.combinedOutputCalls = append(r.combinedOutputCalls, append([]string(nil), args...))
	return nil, nil
}

func (r *recordingRunner) Run(args ...string) error {
	r.runCalls = append(r.runCalls, append([]string(nil), args...))
	return r.runErr
}

func (r *recordingRunner) RunInteractive(args ...string) error {
	r.runInteractiveCalls = append(r.runInteractiveCalls, append([]string(nil), args...))
	return r.runInteractiveErr
}

func TestUpdate_RoutesUpgradeThroughRunInteractive(t *testing.T) {
	rec := &recordingRunner{}
	t.Cleanup(SetRunner(rec))

	err := Update(false)
	require.NoError(t, err)

	// brew update is not sudo-gated, so Run is fine.
	require.Len(t, rec.runCalls, 1)
	assert.Equal(t, []string{"update"}, rec.runCalls[0])

	// brew upgrade may prompt for sudo, so it must go through RunInteractive.
	require.Len(t, rec.runInteractiveCalls, 1)
	assert.Equal(t, []string{"upgrade"}, rec.runInteractiveCalls[0])
}

// TestUpdate_ErrorPropagation is table-driven: each case swaps in a recording
// runner with a preset error on one of the two methods and verifies that
// Update returns a wrapped error mentioning the offending subcommand.
func TestUpdate_ErrorPropagation(t *testing.T) {
	tests := []struct {
		name              string
		runErr            error
		runInteractiveErr error
		wantErrContains   string
	}{
		{
			name:            "brew update fails",
			runErr:          errors.New("network down"),
			wantErrContains: "brew update",
		},
		{
			name:              "brew upgrade fails",
			runInteractiveErr: errors.New("sudo refused"),
			wantErrContains:   "brew upgrade",
		},
		{
			name: "both succeed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordingRunner{
				runErr:            tt.runErr,
				runInteractiveErr: tt.runInteractiveErr,
			}
			t.Cleanup(SetRunner(rec))

			err := Update(false)
			if tt.wantErrContains == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrContains)
		})
	}
}

func TestUpdate_DryRunDoesNotInvokeRunner(t *testing.T) {
	rec := &recordingRunner{}
	t.Cleanup(SetRunner(rec))

	err := Update(true)
	require.NoError(t, err)
	assert.Empty(t, rec.runCalls)
	assert.Empty(t, rec.runInteractiveCalls)
}

func TestPreInstallChecks_UsesRunInteractiveForUpdate(t *testing.T) {
	rec := &recordingRunner{}
	t.Cleanup(SetRunner(rec))

	orig := checkNetworkFunc
	checkNetworkFunc = func() error { return nil }
	t.Cleanup(func() { checkNetworkFunc = orig })

	err := PreInstallChecks(1)
	require.NoError(t, err)

	// The index-refresh `brew update` inside PreInstallChecks now routes
	// through RunInteractive (so any prompts reach the user's TTY).
	require.Len(t, rec.runInteractiveCalls, 1)
	assert.Equal(t, []string{"update"}, rec.runInteractiveCalls[0])
}

func TestSetRunner_RestoreReinstatesPrevious(t *testing.T) {
	before := currentRunner()

	rec := &recordingRunner{}
	restore := SetRunner(rec)
	assert.Same(t, rec, currentRunner())

	restore()
	// `before` may hold a value-typed execRunner, so compare by type/value
	// rather than pointer identity.
	assert.IsType(t, before, currentRunner())
}
