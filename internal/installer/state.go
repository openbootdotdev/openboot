package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type InstallState struct {
	LastUpdated       time.Time       `json:"last_updated"`
	InstalledFormulae map[string]bool `json:"installed_formulae"`
	InstalledCasks    map[string]bool `json:"installed_casks"`
	InstalledNpm      map[string]bool `json:"installed_npm"`
}

func newInstallState() *InstallState {
	return &InstallState{
		LastUpdated:       time.Now(),
		InstalledFormulae: make(map[string]bool),
		InstalledCasks:    make(map[string]bool),
		InstalledNpm:      make(map[string]bool),
	}
}

func getStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".openboot", "install_state.json"), nil
}

func loadState() (*InstallState, error) {
	path, err := getStatePath()
	if err != nil {
		return newInstallState(), fmt.Errorf("load install state: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newInstallState(), nil
		}
		return newInstallState(), fmt.Errorf("read install state: %w", err)
	}

	var state InstallState
	if err := json.Unmarshal(data, &state); err != nil {
		return newInstallState(), fmt.Errorf("parse install state: %w", err)
	}

	if state.InstalledFormulae == nil {
		state.InstalledFormulae = make(map[string]bool)
	}
	if state.InstalledCasks == nil {
		state.InstalledCasks = make(map[string]bool)
	}
	if state.InstalledNpm == nil {
		state.InstalledNpm = make(map[string]bool)
	}

	return &state, nil
}

func (s *InstallState) save() error {
	path, err := getStatePath()
	if err != nil {
		return fmt.Errorf("save install state: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	s.LastUpdated = time.Now()

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal install state: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}

func (s *InstallState) markFormula(name string) error {
	s.InstalledFormulae[name] = true
	return s.save()
}

func (s *InstallState) markCask(name string) error {
	s.InstalledCasks[name] = true
	return s.save()
}

func (s *InstallState) markNpm(name string) error {
	s.InstalledNpm[name] = true
	return s.save()
}

func (s *InstallState) isFormulaInstalled(name string) bool {
	return s.InstalledFormulae[name]
}

func (s *InstallState) isCaskInstalled(name string) bool {
	return s.InstalledCasks[name]
}

func (s *InstallState) isNpmInstalled(name string) bool {
	return s.InstalledNpm[name]
}

// reconcileBrewWithSystem removes formulae and casks from the state that are
// no longer actually installed on the system. This handles the case where a
// user manually uninstalls a package (e.g. `brew uninstall tree`) — without
// reconciliation the state file would still claim it is installed, preventing
// openboot from reinstalling it.
func (s *InstallState) reconcileBrewWithSystem(actualFormulae, actualCasks map[string]bool) (removed int) {
	for name := range s.InstalledFormulae {
		if !actualFormulae[name] {
			delete(s.InstalledFormulae, name)
			removed++
		}
	}
	for name := range s.InstalledCasks {
		if !actualCasks[name] {
			delete(s.InstalledCasks, name)
			removed++
		}
	}
	return removed
}

// reconcileNpmWithSystem removes npm packages from the state that are no
// longer actually installed on the system.
func (s *InstallState) reconcileNpmWithSystem(actualNpm map[string]bool) (removed int) {
	for name := range s.InstalledNpm {
		if !actualNpm[name] {
			delete(s.InstalledNpm, name)
			removed++
		}
	}
	return removed
}
