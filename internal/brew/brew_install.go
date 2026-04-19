package brew

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/openbootdotdev/openboot/internal/system"
	"github.com/openbootdotdev/openboot/internal/ui"
)

// sleepFunc is a test seam for retry delays.
var sleepFunc = time.Sleep

type installJob struct {
	name   string
	isCask bool
}

type failedJob struct {
	installJob
	errMsg string
}

func Install(packages []string, dryRun bool) error {
	if len(packages) == 0 {
		return nil
	}

	if dryRun {
		ui.Info("Would install CLI packages:")
		for _, p := range packages {
			fmt.Printf("    brew install %s\n", p)
		}
		return nil
	}

	ui.Info(fmt.Sprintf("Installing %d CLI packages...", len(packages)))

	args := append([]string{"install"}, packages...)
	cmd := brewInstallCmd(args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func InstallTaps(taps []string, dryRun bool) error {
	if len(taps) == 0 {
		return nil
	}

	if dryRun {
		ui.Info("Would add taps:")
		for _, t := range taps {
			fmt.Printf("    brew tap %s\n", t)
		}
		return nil
	}

	ui.Info(fmt.Sprintf("Adding %d Homebrew taps...", len(taps)))

	for _, tap := range taps {
		if err := currentRunner().Run("tap", tap); err != nil {
			ui.Warn(fmt.Sprintf("Failed to tap %s: %v", tap, err))
		}
	}
	return nil
}

func InstallCask(packages []string, dryRun bool) error {
	if len(packages) == 0 {
		return nil
	}

	if dryRun {
		ui.Info("Would install GUI applications:")
		for _, p := range packages {
			fmt.Printf("    brew install --cask %s\n", p)
		}
		return nil
	}

	ui.Info(fmt.Sprintf("Installing %d GUI applications...", len(packages)))

	args := append([]string{"install", "--cask"}, packages...)
	cmd := brewInstallCmd(args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func InstallWithProgress(cliPkgs, caskPkgs []string, dryRun bool) (installedFormulae []string, installedCasks []string, err error) {
	total := len(cliPkgs) + len(caskPkgs)
	if total == 0 {
		return nil, nil, nil
	}

	if dryRun {
		ui.Info("Would install packages:")
		for _, p := range cliPkgs {
			fmt.Printf("    brew install %s\n", p)
		}
		for _, p := range caskPkgs {
			fmt.Printf("    brew install --cask %s\n", p)
		}
		return nil, nil, nil
	}

	alreadyFormulae, alreadyCasks, checkErr := GetInstalledPackages()
	if checkErr != nil {
		return nil, nil, fmt.Errorf("list installed packages: %w", checkErr)
	}

	// Batch-resolve formula aliases in a single brew info call.
	// Casks don't have an alias system, so we skip resolution for them.
	aliasMap := ResolveFormulaNames(cliPkgs)

	var newCli []string
	for _, p := range cliPkgs {
		resolvedName := aliasMap[p]
		if !alreadyFormulae[resolvedName] {
			newCli = append(newCli, p)
		} else {
			installedFormulae = append(installedFormulae, resolvedName)
		}
	}
	var newCask []string
	for _, p := range caskPkgs {
		if !alreadyCasks[p] {
			newCask = append(newCask, p)
		} else {
			installedCasks = append(installedCasks, p)
		}
	}

	skipped := total - len(newCli) - len(newCask)
	if skipped > 0 {
		ui.Muted(fmt.Sprintf("  %d already installed, %d to install", skipped, len(newCli)+len(newCask)))
		fmt.Println()
	}

	if len(newCli)+len(newCask) == 0 {
		ui.Success("All packages already installed!")
		return installedFormulae, installedCasks, nil
	}

	if preErr := PreInstallChecks(len(newCli) + len(newCask)); preErr != nil {
		return installedFormulae, installedCasks, preErr
	}

	progress := ui.NewStickyProgress(len(newCli) + len(newCask))
	progress.SetSkipped(skipped)
	progress.Start()

	var allFailed []failedJob

	if len(newCli) > 0 {
		failed := runSerialInstallWithProgress(newCli, progress)
		failedSet := make(map[string]bool, len(failed))
		for _, f := range failed {
			failedSet[f.name] = true
		}
		for _, p := range newCli {
			if !failedSet[p] {
				installedFormulae = append(installedFormulae, aliasMap[p])
			}
		}
		allFailed = append(allFailed, failed...)
	}

	if len(newCask) > 0 {
		for _, pkg := range newCask {
			progress.SetCurrent(pkg)
			progress.PrintLine("  Installing %s...", pkg)
			start := time.Now()
			errMsg := installCaskWithProgress(pkg, progress)
			elapsed := time.Since(start)
			progress.IncrementWithStatus(errMsg == "")
			duration := ui.FormatDuration(elapsed)
			if errMsg == "" {
				progress.PrintLine("  %s %s", ui.Green("✔ "+pkg), ui.Cyan("("+duration+")"))
				installedCasks = append(installedCasks, pkg)
			} else {
				progress.PrintLine("  %s %s", ui.Red("✗ "+pkg+" ("+errMsg+")"), ui.Cyan("("+duration+")"))
				allFailed = append(allFailed, failedJob{
					installJob: installJob{name: pkg, isCask: true},
					errMsg:     errMsg,
				})
			}
		}
	}

	progress.Finish()

	if len(allFailed) > 0 {
		fmt.Printf("\nRetrying %d failed packages...\n", len(allFailed))

		for _, f := range allFailed {
			var errMsg string
			if f.isCask {
				errMsg = installSmartCaskWithError(f.name)
			} else {
				errMsg = installFormulaWithError(f.name)
			}
			if errMsg == "" {
				fmt.Printf("  ✔ %s (retry succeeded)\n", f.name)
				if f.isCask {
					installedCasks = append(installedCasks, f.name)
				} else {
					installedFormulae = append(installedFormulae, aliasMap[f.name])
				}
			} else {
				fmt.Printf("  ✗ %s (still failed)\n", f.name)
			}
		}

		succeededFormulae := make(map[string]bool, len(installedFormulae))
		for _, p := range installedFormulae {
			succeededFormulae[p] = true
		}
		succeededCasks := make(map[string]bool, len(installedCasks))
		for _, p := range installedCasks {
			succeededCasks[p] = true
		}
		var stillFailed []failedJob
		for _, f := range allFailed {
			if f.isCask {
				if !succeededCasks[f.name] {
					stillFailed = append(stillFailed, f)
				}
			} else {
				if !succeededFormulae[aliasMap[f.name]] {
					stillFailed = append(stillFailed, f)
				}
			}
		}
		allFailed = stillFailed
	}

	handleFailedJobs(allFailed)

	return installedFormulae, installedCasks, nil
}

func handleFailedJobs(failed []failedJob) {
	if len(failed) == 0 {
		return
	}

	fmt.Println()
	ui.Error(fmt.Sprintf("%d packages failed to install:", len(failed)))
	for _, f := range failed {
		if f.errMsg != "" {
			fmt.Printf("    - %s (%s)\n", f.name, f.errMsg)
		} else {
			fmt.Printf("    - %s\n", f.name)
		}
	}
}

func runSerialInstallWithProgress(pkgs []string, progress *ui.StickyProgress) []failedJob {
	if len(pkgs) == 0 {
		return nil
	}

	failed := make([]failedJob, 0)
	for _, pkg := range pkgs {
		job := installJob{name: pkg, isCask: false}
		progress.SetCurrent(job.name)
		start := time.Now()
		errMsg := installFormulaWithError(job.name)
		elapsed := time.Since(start)
		progress.IncrementWithStatus(errMsg == "")
		duration := ui.FormatDuration(elapsed)
		if errMsg == "" {
			progress.PrintLine("  %s %s", ui.Green("✔ "+job.name), ui.Cyan("("+duration+")"))
			continue
		}

		progress.PrintLine("  %s %s", ui.Red("✗ "+job.name+" ("+errMsg+")"), ui.Cyan("("+duration+")"))
		failed = append(failed, failedJob{
			installJob: job,
			errMsg:     errMsg,
		})
	}

	return failed
}

func installCaskWithProgress(pkg string, progress *ui.StickyProgress) string {
	progress.PauseForInteractive()

	cmd := brewInstallCmd("install", "--cask", pkg)
	tty, opened := system.OpenTTY()
	if opened {
		defer tty.Close() //nolint:errcheck // best-effort TTY cleanup
	}
	cmd.Stdin = tty
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()

	progress.ResumeAfterInteractive()

	if err != nil {
		return "install failed"
	}
	return ""
}

func brewInstallCmd(args ...string) *exec.Cmd {
	cmd := exec.Command("brew", args...)
	cmd.Env = append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1")
	return cmd
}

// brewCombinedOutputWithTTY runs a brew command capturing combined output while
// providing a TTY for stdin so that sudo password prompts work.
func brewCombinedOutputWithTTY(args ...string) (string, error) {
	cmd := brewInstallCmd(args...)
	tty, opened := system.OpenTTY()
	if opened {
		cmd.Stdin = tty
		defer tty.Close() //nolint:errcheck // best-effort TTY cleanup
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func installFormulaWithError(pkg string) string {
	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		cmd := brewInstallCmd("install", pkg)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return ""
		}

		outputStr := string(output)
		if strings.Contains(strings.ToLower(outputStr), "try again using") && strings.Contains(strings.ToLower(outputStr), "--cask") {
			// Cask installs may need sudo for .pkg — use TTY stdin.
			caskOutput, err2 := brewCombinedOutputWithTTY("install", "--cask", pkg)
			if err2 == nil {
				return ""
			}
			outputStr = caskOutput
		}

		errMsg := parseBrewError(outputStr)
		if attempt < maxAttempts && isRetryableError(errMsg) {
			delay := time.Duration(attempt) * 2 * time.Second
			sleepFunc(delay)
			continue
		}

		return errMsg
	}
	return "max retries exceeded"
}

func isRetryableError(errMsg string) bool {
	retryableErrors := []string{
		"connection timed out",
		"connection refused",
		"no internet connection",
		"download corrupted",
		"already running",
		"Cannot download non-corrupt",
		"signature mismatch",
	}
	lower := strings.ToLower(errMsg)
	for _, retryable := range retryableErrors {
		if strings.Contains(lower, strings.ToLower(retryable)) {
			return true
		}
	}
	return false
}

func installSmartCaskWithError(pkg string) string {
	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Cask installs may need sudo for .pkg — use TTY stdin.
		caskOutput, err := brewCombinedOutputWithTTY("install", "--cask", pkg)
		if err == nil {
			return ""
		}

		cmd2 := brewInstallCmd("install", pkg)
		output2, err2 := cmd2.CombinedOutput()
		if err2 == nil {
			return ""
		}

		errMsg := parseBrewError(caskOutput)
		if errMsg == "unknown error" {
			errMsg = parseBrewError(string(output2))
		}

		if attempt < maxAttempts && isRetryableError(errMsg) {
			delay := time.Duration(attempt) * 2 * time.Second
			sleepFunc(delay)
			continue
		}

		return errMsg
	}
	return "max retries exceeded"
}

func parseBrewError(output string) string {
	lowerOutput := strings.ToLower(output)

	switch {
	case strings.Contains(lowerOutput, "no available formula"):
		return "package not found"
	case strings.Contains(lowerOutput, "already installed"):
		return ""
	case strings.Contains(lowerOutput, "no internet"):
		return "no internet connection"
	case strings.Contains(lowerOutput, "connection refused"):
		return "connection refused"
	case strings.Contains(lowerOutput, "timed out"):
		return "connection timed out"
	case strings.Contains(lowerOutput, "permission denied"):
		return "permission denied"
	case strings.Contains(lowerOutput, "disk full") || strings.Contains(lowerOutput, "no space"):
		return "disk full"
	case strings.Contains(lowerOutput, "sha256 mismatch"):
		return "download corrupted"
	case strings.Contains(lowerOutput, "depends on"):
		return "dependency error"
	default:
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.Contains(strings.ToLower(line), "error") {
				if len(line) > 60 {
					return line[:60] + "..."
				}
				return line
			}
		}
		return "unknown error"
	}
}

// ResolveFormulaNames resolves formula aliases to their canonical names in a
// single batched `brew info --json` call. It returns a map from each input name
// to its canonical name. On any error it falls back to an identity mapping.
func ResolveFormulaNames(names []string) map[string]string {
	if len(names) == 0 {
		return make(map[string]string)
	}

	args := append([]string{"info", "--json"}, names...)
	cmd := exec.Command("brew", args...)
	output, err := cmd.Output()
	if err != nil {
		return identityMap(names)
	}

	return parseFormulaAliases(names, output)
}

// parseFormulaAliases builds an alias map from the JSON response of
// `brew info --json`. The JSON array is positionally aligned with names.
func parseFormulaAliases(names []string, jsonData []byte) map[string]string {
	var entries []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(jsonData, &entries); err != nil {
		return identityMap(names)
	}

	resolved := make(map[string]string, len(names))
	for i, n := range names {
		if i < len(entries) && entries[i].Name != "" {
			resolved[n] = entries[i].Name
		} else {
			resolved[n] = n
		}
	}
	return resolved
}

func identityMap(names []string) map[string]string {
	m := make(map[string]string, len(names))
	for _, n := range names {
		m[n] = n
	}
	return m
}
