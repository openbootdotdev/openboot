package brew

import (
	"github.com/openbootdotdev/openboot/internal/progress"
	"github.com/openbootdotdev/openboot/internal/ui"
)

// progressSink, when non-nil, makes InstallWithProgress stream structured
// progress.Events instead of drawing a ui.StickyProgress bar. The install TUI
// registers a sink so it can own the terminal; the default console flow leaves
// it nil. Mirrors the SetRunner swap-and-restore pattern.
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

// stepStart reports the beginning of a package install: emits a StepStart event
// when streaming, otherwise advances the sticky progress bar.
func stepStart(bar *ui.StickyProgress, phase, name, command string) {
	if streaming() {
		progressSink.Emit(progress.Event{Phase: phase, Name: name, Status: progress.StepStart, Command: command})
		return
	}
	bar.SetCurrent(name)
}

// stepDone reports the result of a package install, preserving the exact
// console output when not streaming.
func stepDone(bar *ui.StickyProgress, phase, name string, ok bool, errMsg, duration string) {
	if streaming() {
		if ok {
			progressSink.Emit(progress.Event{Phase: phase, Name: name, Status: progress.StepOK, Detail: duration})
		} else {
			progressSink.Emit(progress.Event{Phase: phase, Name: name, Status: progress.StepFail, Detail: errMsg})
		}
		return
	}
	bar.IncrementWithStatus(ok)
	if ok {
		bar.PrintLine("  %s %s", ui.Green("✔ "+name), ui.Cyan("("+duration+")"))
	} else {
		bar.PrintLine("  %s %s", ui.Red("✗ "+name+" ("+errMsg+")"), ui.Cyan("("+duration+")"))
	}
}
