//go:build e2e

// Helpers shared by the wizard TUI tests in both build flavors: the
// non-destructive choreography tests (e2e && !vm) and the destructive
// real-install test (e2e && vm).
package e2e

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/openbootdotdev/openboot/internal/config"
)

// wizardFormulaCandidates are small catalog formulae whose names match exactly
// one catalog row by substring, so filtering the select screen by name yields
// a deterministic top hit for "enter toggles the top hit".
var wizardFormulaCandidates = []string{"stow", "zoxide", "tealdeer"}

// wizardTestFormula picks a candidate formula not currently installed via
// Homebrew. Skips the test when brew is unavailable or every candidate is
// already present (the choreography needs a toggleable row).
func wizardTestFormula(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("brew", "list", "--formula").Output()
	if err != nil {
		t.Skipf("brew not available: %v", err)
	}
	installed := map[string]bool{}
	for _, name := range strings.Fields(string(out)) {
		installed[name] = true
	}
	for _, name := range wizardFormulaCandidates {
		if catalogHasFormula(name) && !installed[name] {
			return name
		}
	}
	t.Skipf("all candidate formulae already installed: %v", wizardFormulaCandidates)
	return ""
}

func catalogHasFormula(name string) bool {
	for _, cat := range config.GetCategories() {
		for _, p := range cat.Packages {
			if p.Name == name && !p.IsCask && !p.IsNpm {
				return true
			}
		}
	}
	return false
}
