package installer

import "github.com/openbootdotdev/openboot/internal/ui"

// Reporter is the output interface for installation progress.
// Separates UI output from business logic so installer functions
// can be tested without a real terminal.
type Reporter interface {
	Header(msg string)
	Info(msg string)
	Success(msg string)
	Warn(msg string)
	Error(msg string)
	Muted(msg string)
}

// ConsoleReporter writes to the terminal via the ui package.
type ConsoleReporter struct{}

var _ Reporter = ConsoleReporter{}

func (ConsoleReporter) Header(msg string)  { ui.Header(msg) }
func (ConsoleReporter) Info(msg string)    { ui.Info(msg) }
func (ConsoleReporter) Success(msg string) { ui.Success(msg) }
func (ConsoleReporter) Warn(msg string)    { ui.Warn(msg) }
func (ConsoleReporter) Error(msg string)   { ui.Error(msg) }
func (ConsoleReporter) Muted(msg string)   { ui.Muted(msg) }

// NopReporter discards all output. Used in tests.
type NopReporter struct{}

var _ Reporter = NopReporter{}

func (NopReporter) Header(msg string)  {}
func (NopReporter) Info(msg string)    {}
func (NopReporter) Success(msg string) {}
func (NopReporter) Warn(msg string)    {}
func (NopReporter) Error(msg string)   {}
func (NopReporter) Muted(msg string)   {}
