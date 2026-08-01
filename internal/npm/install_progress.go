package npm

import "github.com/openbootdotdev/openboot/internal/ui"

func npmStepStart(bar *ui.StickyProgress, name string) {
	bar.SetCurrent(name)
}

func npmStepDone(bar *ui.StickyProgress, name string, ok bool, errMsg string) {
	// Print then Increment, matching the original console ordering exactly.
	if ok {
		bar.PrintLine("  ✔ %s", name)
	} else {
		bar.PrintLine("  ✗ %s (%s)", name, errMsg)
	}
	bar.Increment()
}
