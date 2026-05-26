package macos

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openbootdotdev/openboot/internal/system"
)

type Preference struct {
	Domain string
	Key    string
	Type   string
	Value  string
	Desc   string
	// Host selects the defaults scope. "" = main domain (default),
	// "currentHost" = -currentHost / ByHost. Required for prefs that macOS
	// stores per-host (e.g. com.apple.controlcenter menu bar dropdown mode
	// on Sequoia); writing to the main domain for those is a silent no-op.
	Host string
}

// DefaultPreferences is derived from DefaultCategories and is the single source
// of truth for all macOS preferences. Add or change preferences in categories.go.
var DefaultPreferences = func() []Preference {
	var prefs []Preference
	for _, cat := range DefaultCategories {
		prefs = append(prefs, cat.Prefs...)
	}
	return prefs
}()

func normalizeBool(value string) string {
	switch strings.ToLower(value) {
	case "1", "yes":
		return "true"
	case "0", "no":
		return "false"
	default:
		return value
	}
}

func InferPreferenceType(value string) string {
	switch strings.ToLower(value) {
	case "true", "false", "1", "0", "yes", "no":
		return "bool"
	}
	allDigits := true
	for _, c := range value {
		if c < '0' || c > '9' {
			allDigits = false
			break
		}
	}
	if allDigits && value != "" {
		return "int"
	}
	if strings.Contains(value, ".") {
		allNumeric := true
		for _, c := range strings.ReplaceAll(value, ".", "") {
			if c < '0' || c > '9' {
				allNumeric = false
				break
			}
		}
		if allNumeric {
			return "float"
		}
	}
	return "string"
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func Configure(prefs []Preference, dryRun bool) error {
	var errs []error
	for _, pref := range prefs {
		value := expandHome(pref.Value)

		if dryRun {
			fmt.Printf("[DRY-RUN] Would set %s%s %s = %s (%s)\n", hostScopeLabel(pref.Host), pref.Domain, pref.Key, value, pref.Desc)
			continue
		}

		args := []string{}
		if pref.Host == "currentHost" {
			args = append(args, "-currentHost")
		}
		args = append(args, "write", pref.Domain, pref.Key)
		switch pref.Type {
		case "bool":
			args = append(args, "-bool", normalizeBool(value))
		case "int":
			args = append(args, "-int", value)
		case "float":
			args = append(args, "-float", value)
		case "string":
			args = append(args, "-string", value)
		default:
			args = append(args, value)
		}

		if _, err := system.RunCommandSilent("defaults", args...); err != nil {
			errs = append(errs, fmt.Errorf("set %s%s %s: %w", hostScopeLabel(pref.Host), pref.Domain, pref.Key, err))
		}
	}

	return errors.Join(errs...)
}

// hostScopeLabel returns a short prefix for log/error messages that identifies
// the defaults scope. Empty for the main domain (the common case).
func hostScopeLabel(host string) string {
	if host == "currentHost" {
		return "(ByHost) "
	}
	return ""
}

func CreateScreenshotsDir(dryRun bool) error {
	home, err := system.HomeDir()
	if err != nil {
		return fmt.Errorf("create screenshots dir: %w", err)
	}
	dir := filepath.Join(home, "Screenshots")

	if dryRun {
		fmt.Printf("[DRY-RUN] Would create %s directory\n", dir)
		return nil
	}

	return os.MkdirAll(dir, 0750)
}

func RestartAffectedApps(dryRun bool) error {
	apps := []string{"Finder", "Dock", "SystemUIServer", "ControlCenter"}

	for _, app := range apps {
		if dryRun {
			fmt.Printf("[DRY-RUN] Would restart %s\n", app)
			continue
		}

		// killall returns an error if the app isn't running, which is expected and safe to ignore
		system.RunCommandSilent("killall", app) //nolint:errcheck,gosec // non-fatal: app may not be running
	}

	return nil
}
