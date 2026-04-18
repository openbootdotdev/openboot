package config

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const (
	maxPackageNameLen    = 200
	maxPostInstallCmdLen = 4096
)

var (
	pkgNameRe = regexp.MustCompile(`^[a-zA-Z0-9@/_.-]+$`)
	tapNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+$`)

	// dotfilesPathRe validates the path component: one or more segments of
	// alphanumeric, dash, underscore, or dot characters separated by slashes.
	dotfilesPathRe = regexp.MustCompile(`^/[a-zA-Z0-9._-]+(/[a-zA-Z0-9._-]+)*$`)
)

// ValidateDotfilesURL checks that a dotfiles repo URL uses HTTPS, has a
// valid path, max 500 chars, and no path traversal. Any HTTPS host is
// accepted (including self-hosted GitLab, Gitea, etc.).
func ValidateDotfilesURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}

	if len(rawURL) > 500 {
		return fmt.Errorf("dotfiles URL too long (%d chars, max 500)", len(rawURL))
	}

	if !strings.HasPrefix(rawURL, "https://") {
		return fmt.Errorf("dotfiles URL must use https:// (got %q); git@ URLs are not allowed", rawURL)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("dotfiles URL is not a valid URL: %w", err)
	}

	if parsed.Hostname() == "" {
		return fmt.Errorf("dotfiles URL is missing a hostname")
	}

	path := parsed.Path
	if strings.Contains(path, "..") {
		return fmt.Errorf("dotfiles URL path must not contain '..'")
	}
	if strings.Contains(path, "//") {
		return fmt.Errorf("dotfiles URL path must not contain '//'")
	}
	if !dotfilesPathRe.MatchString(path) {
		return fmt.Errorf("dotfiles URL has an invalid path %q; expected format: https://<host>/<owner>/<repo>", path)
	}

	return nil
}

func (rc *RemoteConfig) Validate() error {
	for _, p := range rc.Packages {
		if len(p.Name) > maxPackageNameLen {
			return fmt.Errorf("package name too long (%d chars, max %d): %q", len(p.Name), maxPackageNameLen, p.Name)
		}
		if !pkgNameRe.MatchString(p.Name) {
			return fmt.Errorf("invalid package name: %q", p.Name)
		}
	}
	for _, c := range rc.Casks {
		if len(c.Name) > maxPackageNameLen {
			return fmt.Errorf("cask name too long (%d chars, max %d): %q", len(c.Name), maxPackageNameLen, c.Name)
		}
		if !pkgNameRe.MatchString(c.Name) {
			return fmt.Errorf("invalid cask name: %q", c.Name)
		}
	}
	for _, n := range rc.Npm {
		if len(n.Name) > maxPackageNameLen {
			return fmt.Errorf("npm package name too long (%d chars, max %d): %q", len(n.Name), maxPackageNameLen, n.Name)
		}
		if !pkgNameRe.MatchString(n.Name) {
			return fmt.Errorf("invalid npm package name: %q", n.Name)
		}
	}
	for _, t := range rc.Taps {
		if len(t) > maxPackageNameLen {
			return fmt.Errorf("tap name too long (%d chars, max %d): %q", len(t), maxPackageNameLen, t)
		}
		if !tapNameRe.MatchString(t) {
			return fmt.Errorf("invalid tap name: %q (expected format: owner/repo)", t)
		}
	}
	if err := ValidateDotfilesURL(rc.DotfilesRepo); err != nil {
		return fmt.Errorf("invalid dotfiles_repo: %w", err)
	}
	validPrefTypes := map[string]bool{"": true, "string": true, "int": true, "bool": true, "float": true}
	for _, mp := range rc.MacOSPrefs {
		if !validPrefTypes[mp.Type] {
			return fmt.Errorf("invalid macos_prefs type: %q for %s %s (allowed: string, int, bool, float)", mp.Type, mp.Domain, mp.Key)
		}
		if strings.HasPrefix(mp.Domain, "-") {
			return fmt.Errorf("invalid macos_prefs domain: %q must not start with '-'", mp.Domain)
		}
		if strings.HasPrefix(mp.Key, "-") {
			return fmt.Errorf("invalid macos_prefs key: %q must not start with '-'", mp.Key)
		}
	}
	for i, cmd := range rc.PostInstall {
		if strings.TrimSpace(cmd) == "" {
			return fmt.Errorf("post_install[%d]: command must not be empty or whitespace only", i)
		}
		if strings.ContainsRune(cmd, 0) {
			return fmt.Errorf("post_install[%d]: command must not contain NUL bytes", i)
		}
		if len(cmd) > maxPostInstallCmdLen {
			return fmt.Errorf("post_install[%d]: command too long (%d chars, max %d)", i, len(cmd), maxPostInstallCmdLen)
		}
	}
	return nil
}
