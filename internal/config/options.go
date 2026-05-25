package config

// ToInstallOptions returns a pointer to the embedded InstallOptions within Config.
// Zero-copy: callers receive a direct reference to the input fields.
func (c *Config) ToInstallOptions() *InstallOptions {
	return &c.InstallOptions
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
