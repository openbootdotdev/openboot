package progress

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSinkEmitNilSafe(t *testing.T) {
	var s Sink // nil
	assert.NotPanics(t, func() { s.Emit(Event{Name: "x"}) })
}

func TestSinkEmitForwards(t *testing.T) {
	var got []Event
	s := Sink(func(e Event) { got = append(got, e) })
	s.Emit(Event{Phase: PhaseHomebrew, Name: "git", Status: StepOK, Detail: "1s"})
	assert.Equal(t, []Event{{Phase: PhaseHomebrew, Name: "git", Status: StepOK, Detail: "1s"}}, got)
}
