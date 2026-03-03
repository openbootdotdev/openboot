package diff

import (
	"encoding/json"
	"fmt"

	"github.com/openbootdotdev/openboot/internal/ui"
)

// FormatTerminal prints a colored diff to stdout.
// Nil sections are automatically skipped (e.g. remote configs without shell/git data).
func FormatTerminal(result *DiffResult, packagesOnly bool) {
	fmt.Println()
	ui.Header("OpenBoot Diff")
	fmt.Println()

	printSource(result.Source)
	fmt.Println()

	printListSection("Formulae", result.Packages.Formulae)
	printListSection("Casks", result.Packages.Casks)
	printListSection("NPM", result.Packages.Npm)
	printListSection("Taps", result.Packages.Taps)

	if !packagesOnly {
		if result.Shell != nil {
			printShellSection(result.Shell)
		}
		if result.Git != nil {
			printGitSection(result.Git)
		}
		if result.MacOS != nil {
			printMacOSSection(result.MacOS)
		}
		if result.DevTools != nil {
			printDevToolsSection(result.DevTools)
		}
	}

	printSummary(result)
}

// FormatJSON returns the diff result as indented JSON.
func FormatJSON(result *DiffResult) ([]byte, error) {
	out := jsonOutput{
		Source:   result.Source,
		Packages: result.Packages,
		Shell:    result.Shell,
		Git:      result.Git,
		MacOS:    result.MacOS,
		DevTools: result.DevTools,
		Summary: jsonSummary{
			Missing: result.TotalMissing(),
			Extra:   result.TotalExtra(),
			Changed: result.TotalChanged(),
		},
	}

	return json.MarshalIndent(out, "", "  ")
}

type jsonOutput struct {
	Source   Source      `json:"source"`
	Packages PackageDiff `json:"packages"`
	Shell    *ShellDiff  `json:"shell,omitempty"`
	Git      *GitDiff    `json:"git,omitempty"`
	MacOS    *MacOSDiff  `json:"macos,omitempty"`
	DevTools *DevToolDiff `json:"dev_tools,omitempty"`
	Summary  jsonSummary `json:"summary"`
}

type jsonSummary struct {
	Missing int `json:"missing"`
	Extra   int `json:"extra"`
	Changed int `json:"changed"`
}

func printSource(source Source) {
	switch source.Kind {
	case "local":
		ui.Info(fmt.Sprintf("Comparing: system vs local snapshot (%s)", source.Path))
	case "file":
		ui.Info(fmt.Sprintf("Comparing: system vs snapshot file (%s)", source.Path))
	case "remote":
		ui.Info(fmt.Sprintf("Comparing: system vs remote config (%s)", source.Path))
	default:
		ui.Info(fmt.Sprintf("Comparing: system vs %s", source.Path))
	}
}

func printListSection(name string, ld ListDiff) {
	if len(ld.Missing) == 0 && len(ld.Extra) == 0 && ld.Common == 0 {
		return
	}

	fmt.Printf("  %s:\n", name)
	for _, item := range ld.Missing {
		fmt.Printf("    %s %-28s %s\n", ui.Green("+"), item, ui.Green("(missing)"))
	}
	for _, item := range ld.Extra {
		fmt.Printf("    %s %-28s %s\n", ui.Red("-"), item, ui.Red("(extra)"))
	}
	if ld.Common > 0 {
		fmt.Printf("    %s %d in common\n", ui.Cyan("="), ld.Common)
	}
	fmt.Println()
}

func printShellSection(sd *ShellDiff) {
	hasContent := sd.Theme != nil || sd.OhMyZsh != nil ||
		len(sd.Plugins.Missing) > 0 || len(sd.Plugins.Extra) > 0
	if !hasContent {
		return
	}

	fmt.Printf("  Shell:\n")
	if sd.OhMyZsh != nil {
		fmt.Printf("    %s oh-my-zsh: %s %s %s\n",
			ui.Yellow("~"), sd.OhMyZsh.System, ui.Yellow("\u2192"), sd.OhMyZsh.Reference)
	}
	if sd.Theme != nil {
		fmt.Printf("    %s theme: %s %s %s\n",
			ui.Yellow("~"), sd.Theme.System, ui.Yellow("\u2192"), sd.Theme.Reference)
	}
	for _, p := range sd.Plugins.Missing {
		fmt.Printf("    %s plugin: %-20s %s\n", ui.Green("+"), p, ui.Green("(missing)"))
	}
	for _, p := range sd.Plugins.Extra {
		fmt.Printf("    %s plugin: %-20s %s\n", ui.Red("-"), p, ui.Red("(extra)"))
	}
	fmt.Println()
}

func printGitSection(gd *GitDiff) {
	hasContent := gd.UserName != nil || gd.UserEmail != nil
	if !hasContent {
		return
	}

	fmt.Printf("  Git:\n")
	if gd.UserName != nil {
		fmt.Printf("    %s user.name: %s %s %s\n",
			ui.Yellow("~"), gd.UserName.System, ui.Yellow("\u2192"), gd.UserName.Reference)
	}
	if gd.UserEmail != nil {
		fmt.Printf("    %s user.email: %s %s %s\n",
			ui.Yellow("~"), gd.UserEmail.System, ui.Yellow("\u2192"), gd.UserEmail.Reference)
	}
	fmt.Println()
}

func printMacOSSection(md *MacOSDiff) {
	hasContent := len(md.Changed) > 0 || len(md.Missing) > 0 || len(md.Extra) > 0
	if !hasContent {
		return
	}

	fmt.Printf("  macOS Preferences:\n")
	for _, c := range md.Changed {
		fmt.Printf("    %s %s.%s: %s %s %s\n",
			ui.Yellow("~"), c.Domain, c.Key, c.System, ui.Yellow("\u2192"), c.Reference)
	}
	for _, m := range md.Missing {
		fmt.Printf("    %s %s.%s = %s  %s\n",
			ui.Green("+"), m.Domain, m.Key, m.Value, ui.Green("(missing)"))
	}
	for _, e := range md.Extra {
		fmt.Printf("    %s %s.%s = %s  %s\n",
			ui.Red("-"), e.Domain, e.Key, e.Value, ui.Red("(extra)"))
	}
	fmt.Println()
}

func printDevToolsSection(dd *DevToolDiff) {
	hasContent := len(dd.Missing) > 0 || len(dd.Extra) > 0 || len(dd.Changed) > 0
	if !hasContent {
		return
	}

	fmt.Printf("  Dev Tools:\n")
	for _, c := range dd.Changed {
		fmt.Printf("    %s %s: %s %s %s\n",
			ui.Yellow("~"), c.Name, c.System, ui.Yellow("\u2192"), c.Reference)
	}
	for _, name := range dd.Missing {
		fmt.Printf("    %s %-28s %s\n", ui.Green("+"), name, ui.Green("(missing)"))
	}
	for _, name := range dd.Extra {
		fmt.Printf("    %s %-28s %s\n", ui.Red("-"), name, ui.Red("(extra)"))
	}
	fmt.Println()
}

func printSummary(result *DiffResult) {
	missing := result.TotalMissing()
	extra := result.TotalExtra()
	changed := result.TotalChanged()

	if missing == 0 && extra == 0 && changed == 0 {
		ui.Success("No differences found — your system matches the reference.")
	} else {
		fmt.Printf("  Summary: %s missing (+)  %s extra (-)  %s changed (~)\n",
			ui.Green(fmt.Sprintf("%d", missing)),
			ui.Red(fmt.Sprintf("%d", extra)),
			ui.Yellow(fmt.Sprintf("%d", changed)))
	}
	fmt.Println()
}
