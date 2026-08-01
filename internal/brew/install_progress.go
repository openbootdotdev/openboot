package brew

import "github.com/openbootdotdev/openboot/internal/ui"

func stepStart(bar *ui.StickyProgress, name string) {
	bar.SetCurrent(name)
}

func stepDone(bar *ui.StickyProgress, name string, ok bool, errMsg, duration string) {
	bar.IncrementWithStatus(ok)
	if ok {
		bar.PrintLine("  %s %s", ui.Green("✔ "+name), ui.Cyan("("+duration+")"))
	} else {
		bar.PrintLine("  %s %s", ui.Red("✗ "+name+" ("+errMsg+")"), ui.Cyan("("+duration+")"))
	}
}
