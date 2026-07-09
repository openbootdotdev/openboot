package npm

import (
	"github.com/openbootdotdev/openboot/internal/progress"
	"github.com/openbootdotdev/openboot/internal/ui"
)

// progressSink, when non-nil, makes the npm installer stream structured
// progress.Events instead of drawing a ui.StickyProgress bar. Mirrors brew's
// SetProgressSink swap-and-restore pattern.
var progressSink progress.Sink

// SetProgressSink registers a streaming progress sink and returns a restore
// func that clears it.
func SetProgressSink(s progress.Sink) (restore func()) {
	prev := progressSink
	progressSink = s
	return func() { progressSink = prev }
}

// streaming reports whether a sink is registered (TUI mode).
func streaming() bool { return progressSink != nil }

// EmitSkipped emits an already-installed StepOK event for each named package,
// upholding the streaming invariant that every planned package produces
// exactly one terminal event. No-op when no sink is registered.
func EmitSkipped(names []string) {
	if !streaming() {
		return
	}
	for _, n := range names {
		progressSink.Emit(progress.Event{Phase: progress.PhaseNpm, Name: n, Status: progress.StepOK, Detail: progress.SkipDetail})
	}
}

// npmStepStart reports the start of a single npm package install.
func npmStepStart(bar *ui.StickyProgress, name string) {
	if streaming() {
		progressSink.Emit(progress.Event{Phase: progress.PhaseNpm, Name: name, Status: progress.StepStart, Command: "npm install -g " + name})
		return
	}
	bar.SetCurrent(name)
}

// npmStepDone reports the result of a single npm package install, preserving
// the exact console output when not streaming.
func npmStepDone(bar *ui.StickyProgress, name string, ok bool, errMsg string) {
	if streaming() {
		if ok {
			progressSink.Emit(progress.Event{Phase: progress.PhaseNpm, Name: name, Status: progress.StepOK})
		} else {
			progressSink.Emit(progress.Event{Phase: progress.PhaseNpm, Name: name, Status: progress.StepFail, Detail: errMsg})
		}
		return
	}
	// Print then Increment, matching the original console ordering exactly.
	if ok {
		bar.PrintLine("  ✔ %s", name)
	} else {
		bar.PrintLine("  ✗ %s (%s)", name, errMsg)
	}
	bar.Increment()
}
