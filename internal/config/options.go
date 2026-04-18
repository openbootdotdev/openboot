package config

// ToInstallOptions extracts the read-only input fields from Config.
func (c *Config) ToInstallOptions() *InstallOptions {
	return &InstallOptions{
		Version:          c.Version,
		Preset:           c.Preset,
		User:             c.User,
		DryRun:           c.DryRun,
		Silent:           c.Silent,
		PackagesOnly:     c.PackagesOnly,
		Update:           c.Update,
		Shell:            c.Shell,
		Macos:            c.Macos,
		Dotfiles:         c.Dotfiles,
		GitName:          c.GitName,
		GitEmail:         c.GitEmail,
		PostInstall:      c.PostInstall,
		AllowPostInstall: c.AllowPostInstall,
		DotfilesURL:      c.DotfilesURL,
	}
}

// ToInstallState extracts the mutable runtime fields from Config.
func (c *Config) ToInstallState() *InstallState {
	return &InstallState{
		SelectedPkgs:     c.SelectedPkgs,
		OnlinePkgs:       c.OnlinePkgs,
		SnapshotTaps:     c.SnapshotTaps,
		RemoteConfig:     c.RemoteConfig,
		SnapshotGit:      c.SnapshotGit,
		SnapshotMacOS:    c.SnapshotMacOS,
		SnapshotDotfiles:    c.SnapshotDotfiles,
		SnapshotShellOhMyZsh: c.SnapshotShellOhMyZsh,
		SnapshotShellTheme:   c.SnapshotShellTheme,
		SnapshotShellPlugins: c.SnapshotShellPlugins,
	}
}

// ApplyState writes runtime state back into the Config (for callers that still
// use *Config as the shared context, e.g. CLI sync/diff commands).
func (c *Config) ApplyState(s *InstallState) {
	c.SelectedPkgs = s.SelectedPkgs
	c.OnlinePkgs = s.OnlinePkgs
	c.SnapshotTaps = s.SnapshotTaps
	c.RemoteConfig = s.RemoteConfig
	c.SnapshotGit = s.SnapshotGit
	c.SnapshotMacOS = s.SnapshotMacOS
	c.SnapshotDotfiles = s.SnapshotDotfiles
	c.SnapshotShellOhMyZsh = s.SnapshotShellOhMyZsh
	c.SnapshotShellTheme = s.SnapshotShellTheme
	c.SnapshotShellPlugins = s.SnapshotShellPlugins
}
