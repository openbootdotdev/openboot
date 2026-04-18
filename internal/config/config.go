// Package config provides configuration types, remote config fetching,
// preset management, and package catalog access for openboot.
//
// File layout:
//   - types.go      — struct definitions (Config, InstallOptions, InstallState, RemoteConfig, etc.)
//   - options.go    — Config conversion methods (ToInstallOptions, ToInstallState, ApplyState)
//   - validate.go   — ValidateDotfilesURL and RemoteConfig.Validate
//   - presets.go    — embedded presets.yaml, GetPreset, GetPresetNames
//   - remote.go     — HTTP client, FetchRemoteConfig, UnmarshalRemoteConfigFlexible, LoadRemoteConfigFromFile, GetScreenRecordingPackages
//   - packages.go   — embedded packages.yaml, Categories, package lookup helpers
//   - packages_remote.go — remote package refresh and cache
//   - project.go    — ProjectConfig (.openboot.yml) loading and validation
package config
