package brew

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/progress"
)

func TestSetProgressSinkSwapAndRestore(t *testing.T) {
	require.Nil(t, progressSink)
	require.False(t, streaming())

	var got []progress.Event
	restore := SetProgressSink(func(ev progress.Event) { got = append(got, ev) })
	require.True(t, streaming())

	progressSink.Emit(progress.Event{Name: "x"})
	restore()
	assert.Nil(t, progressSink)
	assert.False(t, streaming())
	assert.Len(t, got, 1)
}

// When streaming, the step helpers must emit events and never touch the (nil) bar.
func TestStepHelpersEmitWhenStreaming(t *testing.T) {
	var got []progress.Event
	restore := SetProgressSink(func(ev progress.Event) { got = append(got, ev) })
	defer restore()

	assert.NotPanics(t, func() {
		stepStart(nil, progress.PhaseHomebrew, "git", "brew install git")
		stepDone(nil, progress.PhaseHomebrew, "git", true, "", "1.2s")
		stepDone(nil, progress.PhaseApplications, "figma", false, "download failed", "0.5s")
	})

	require.Len(t, got, 3)
	assert.Equal(t, progress.Event{Phase: progress.PhaseHomebrew, Name: "git", Status: progress.StepStart, Command: "brew install git"}, got[0])
	assert.Equal(t, progress.Event{Phase: progress.PhaseHomebrew, Name: "git", Status: progress.StepOK, Detail: "1.2s"}, got[1])
	assert.Equal(t, progress.Event{Phase: progress.PhaseApplications, Name: "figma", Status: progress.StepFail, Detail: "download failed"}, got[2])
}
