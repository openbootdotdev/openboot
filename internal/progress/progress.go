// Package progress defines the streaming progress event contract between the
// install engine (brew, npm) and a live renderer such as the install TUI.
//
// It is a leaf package with no internal dependencies so brew, npm, installer,
// and the TUI can all import it without creating an import cycle.
//
// When no Sink is registered the install engine renders its own progress
// (ui.StickyProgress). When a Sink is registered the engine emits Events
// instead and draws nothing to stdout, letting the caller own the display.
package progress

// Status describes where a step is in its lifecycle.
type Status int

const (
	// StepStart is emitted when a step begins (before the command runs).
	StepStart Status = iota
	// StepOK is emitted when a step finishes successfully.
	StepOK
	// StepFail is emitted when a step fails.
	StepFail
)

// Event is a single progress signal emitted during installation.
type Event struct {
	Phase   string // pipeline phase, e.g. "Homebrew", "Applications", "npm globals"
	Name    string // step name, e.g. the package being installed
	Status  Status
	Command string // shell command being run, for the live log (StepStart only)
	Detail  string // result detail: version/duration on success, error message on failure
}

// Canonical phase names emitted by the install engine. The TUI matches on
// these so brew, npm, and the renderer stay in agreement.
const (
	PhaseHomebrew     = "Homebrew"
	PhaseApplications = "Applications"
	PhaseNpm          = "npm globals"
)

// Sink receives install progress events. A nil Sink means "no streaming
// renderer registered" — the engine falls back to its own progress output.
type Sink func(Event)

// Emit sends an event to s, tolerating a nil sink.
func (s Sink) Emit(ev Event) {
	if s != nil {
		s(ev)
	}
}
