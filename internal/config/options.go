package config

// ToInstallOptions returns a copy of the embedded InstallOptions.
// Plan() may write to opts fields (e.g. opts.Preset) during planning;
// returning a copy preserves the original Config values unchanged.
func (c *Config) ToInstallOptions() *InstallOptions {
	opts := c.InstallOptions
	return &opts
}

// ToInstallState returns a pointer to the embedded InstallState within Config.
// Zero-copy: callers receive a direct reference to the runtime state fields.
func (c *Config) ToInstallState() *InstallState {
	return &c.InstallState
}

// ApplyState writes runtime state back into the Config (for callers that still
// use *Config as the shared context, e.g. CLI sync/diff commands).
func (c *Config) ApplyState(s *InstallState) {
	c.InstallState = *s
}
