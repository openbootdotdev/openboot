package main

import (
	"os"

	"github.com/openbootdotdev/openboot/internal/cli"
	"golang.org/x/term"
)

func main() {
	// Save terminal state before any TUI (huh/bubbletea) runs.
	// Ensures the terminal is restored even if a TUI component crashes
	// or exits without proper cleanup (e.g., when invoked via curl|bash).
	if fd := int(os.Stdin.Fd()); term.IsTerminal(fd) {
		if oldState, err := term.GetState(fd); err == nil {
			defer term.Restore(fd, oldState)
		}
	}

	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
