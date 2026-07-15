package npm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/progress"
)

func TestSetProgressSinkSwapAndRestore(t *testing.T) {
	require.Nil(t, progressSink)
	require.False(t, streaming())

	restore := SetProgressSink(func(progress.Event) {})
	require.True(t, streaming())
	restore()
	assert.False(t, streaming())
}

func TestNpmStepHelpersEmitWhenStreaming(t *testing.T) {
	var got []progress.Event
	restore := SetProgressSink(func(ev progress.Event) { got = append(got, ev) })
	defer restore()

	assert.NotPanics(t, func() {
		npmStepStart(nil, "typescript")
		npmStepDone(nil, "typescript", true, "", "2.1s")
		npmStepDone(nil, "eslint", false, "E404", "0.3s")
	})

	require.Len(t, got, 3)
	assert.Equal(t, progress.Event{Phase: progress.PhaseNpm, Name: "typescript", Status: progress.StepStart, Command: "npm install -g typescript"}, got[0])
	assert.Equal(t, progress.Event{Phase: progress.PhaseNpm, Name: "typescript", Status: progress.StepOK, Detail: "2.1s"}, got[1])
	assert.Equal(t, progress.Event{Phase: progress.PhaseNpm, Name: "eslint", Status: progress.StepFail, Detail: "E404"}, got[2])
}
