# OpenBoot Threat Model

**Version:** as of repository HEAD  
**Scope:** `openboot` CLI, macOS only  
**Audience:** Security-minded developers evaluating OpenBoot for personal or team use

---

## 1. Scope

### What OpenBoot Does

OpenBoot is a macOS CLI that automates developer environment setup. In a single invocation it can:

- Install Homebrew formulae and casks (by name, via `brew install`)
- Install npm global packages (by name, via `npm install -g`)
- Add Homebrew taps (by owner/repo name, via `brew tap`)
- Install Oh-My-Zsh and patch `~/.zshrc` with a theme and plugin list
- Apply macOS `defaults write` preferences
- Clone a dotfiles git repository and run `stow` to link files
- Run an arbitrary post-install shell script sourced from a remote config (opt-in only)
- Capture and restore snapshots of the above state

### What OpenBoot Does Not Do

- It does not escalate privileges with `sudo` directly. Any privilege escalation that occurs happens inside Homebrew or Xcode CLT installers, which request it themselves.
- It does not store credentials other than a single bearer token in `~/.openboot/auth.json`.
- It does not phone home with telemetry, package lists, or usage data.
- It does not execute remote shell code by default. The `post_install` field in a remote config is skipped unless the operator explicitly passes `--allow-post-install` (in non-interactive mode) or confirms a prompt (in interactive mode).
- It does not modify files outside the user's home directory, except through Homebrew or Xcode which manage their own prefix paths.

---

## 2. Actors and Trust Levels

| Actor | Trust Level | Rationale |
|---|---|---|
| **Local user running `openboot`** | Full trust | The CLI runs as the user. All actions are bounded by the user's own file permissions. |
| **openboot.dev API** | High, with validation | All responses are validated against a schema (`RemoteConfig.Validate()`). Response body size is capped at 1 MiB (`io.LimitReader`). The API URL is pinned to HTTPS (configurable only to HTTPS or localhost). |
| **Remote config author** (`openboot install <user/slug>`) | Medium — verified identity, untrusted content | The config author is an authenticated openboot.dev user. Their package names are regex-validated. Their `post_install` commands are not executed without explicit opt-in. |
| **Oh-My-Zsh GitHub CDN** (`raw.githubusercontent.com`) | Accepted risk, no hash verification | The OMZ install script is fetched and executed via `curl | sh`. No checksum is verified. This is the official OMZ install method. |
| **Homebrew** | High | Homebrew is treated as a trusted package manager. Package integrity is managed by Homebrew's own SHA-256 verification. OpenBoot only passes package names to `brew install`. |
| **npm registry** | Medium | npm packages are installed by name with no additional pinning beyond whatever `npm` itself enforces (no lockfile context at install time). |
| **Dotfiles repository** | Untrusted until user confirms | Cloned from a URL supplied by the config or the user. The URL is validated to use HTTPS. Content of the cloned repo is not scanned. |
| **GitHub Releases** (auto-update) | High, no hash verification | Binary downloads come from `github.com/openbootdotdev/openboot/releases`. No checksum is verified after download. The binary is written over the current executable. |

---

## 3. Threat Scenarios

### T1 — Malicious Remote Config: Post-Install Code Execution

**Description:** A remote config hosted on openboot.dev includes a `post_install` array containing arbitrary shell commands. If a user runs `openboot install attacker/slug`, those commands could be executed.

**Likelihood:** Medium. Any authenticated openboot.dev user can publish a config with `post_install`.

**Impact:** Critical. `post_install` is passed verbatim to `/bin/zsh -c` and runs as the user, in their home directory, with full user privileges.

**Mitigations:**

- In interactive mode (`--silent` not set, TTY present), `stepPostInstall` shows a preview of every command in the script and requires explicit `y` confirmation before executing (`ui.Confirm`). A user who reads the preview can reject it.
- In non-interactive / silent mode, `post_install` is **skipped by default**. Execution only occurs if the caller also passes `--allow-post-install`. This flag is not set by default in any automated invocation.
- The preview is always shown before prompting, so the user sees what will run.

**Residual risk:** The gate is a text preview and a confirmation prompt. A user who does not read the preview, or who runs `--allow-post-install` without reviewing the config, will execute the commands. There is no sandbox, no allowlist, and no signature verification on `post_install` content. The field is inherently a remote code execution primitive behind a user-approval gate.

**Recommendation for teams:** If you are deploying openboot in a CI or fleet context, never pass `--allow-post-install` unless you control the config author's account and have reviewed the commands.

---

### T2 — Malicious Remote Config: Package Name Injection

**Description:** A remote config specifies package names that are passed to `brew install`, `brew install --cask`, `npm install -g`, or `brew tap`. An attacker-controlled config could attempt to install malicious packages or exploit shell metacharacters in names to inject arguments.

**Likelihood:** Low. The validation layer is a meaningful barrier.

**Impact:** High if bypassed. A malicious cask or formula could install persistent malware.

**Mitigations:**

- `RemoteConfig.Validate()` applies regex allowlists before any package name is used:
  - Formula and cask names: `^[a-zA-Z0-9@/_.-]+$` (max 200 chars)
  - Tap names: `^[a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+$` (exactly `owner/repo`)
  - npm package names: same regex as formulae
- Package names are passed as discrete arguments to `exec.Command("brew", "install", ...)`, not interpolated into a shell string. Shell metacharacters that survive the regex (none should) would not be interpreted by the shell.
- Homebrew itself checks package existence and integrity (SHA-256 of bottles).

**Residual risk:** Package squatting. A legitimate-looking package name that passes regex validation could still be a malicious Homebrew formula or npm package. OpenBoot does not verify that package names correspond to known-safe packages. Defense against typosquatting is the user's responsibility.

---

### T3 — Malicious Dotfiles Repository

**Description:** A remote config specifies a `dotfiles_repo` URL pointing to an attacker-controlled git repository. When cloned and stowed, the dotfiles content runs during the next shell session.

**Likelihood:** Medium. Any remote config can specify a dotfiles URL.

**Impact:** High. Dotfiles applied to `~` can inject shell hooks, PATH entries, aliases, or git credential helpers that execute code at login or on git operations.

**Mitigations:**

- `ValidateDotfilesURL` enforces:
  - HTTPS scheme only (no `git@`, no `http://`, no `file://`)
  - Hostname must be present
  - Path must not contain `..` or `//`
  - Path structure: `/<owner>/<repo>` (at most two segments with alphanumeric/dot/dash/underscore)
  - Maximum 500 characters
- The validation prevents file-scheme local path reads and path traversal in the URL itself.
- In interactive mode, the user is shown the package list (including `dotfiles_repo` via config display) before confirming.

**Residual risk:** Validation only constrains the URL form, not the repository content. Any HTTPS git URL that satisfies the regex is accepted. The dotfiles repo content is fully trusted once cloned. A user who installs a config from an untrusted author is trusting that author's dotfiles repo.

---

### T4 — Oh-My-Zsh Install via curl | sh

**Description:** `InstallOhMyZsh` fetches `https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh` and pipes it directly to `bash`. No checksum is verified.

**Likelihood:** Low (requires GitHub or CDN compromise).

**Impact:** Critical if the script is tampered with. The script runs as the user and performs arbitrary operations.

**Mitigations:**

- The URL is hardcoded to `raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/...`. The domain is not user-configurable.
- HTTPS is used. TLS certificate validation is performed by the system's `curl`.
- This is the officially documented install method for Oh-My-Zsh. The project does not provide a signed release artifact.

**Accepted risk:** There is no hash pinning. A compromise of the GitHub repository, CDN, or a TLS MitM would not be detected. This is the same risk accepted by every other tool that uses the official OMZ installer. It is listed here for transparency, not because there is a viable alternative that the OMZ project provides.

---

### T5 — Auto-Update Binary Replacement Without Hash Verification

**Description:** When not installed via Homebrew, `DownloadAndReplace` fetches the latest release binary from `github.com/openbootdotdev/openboot/releases/latest/download/openboot-darwin-<arch>` and atomically replaces the running executable. No checksum is verified after download.

**Likelihood:** Low (requires GitHub infrastructure compromise or TLS MitM).

**Impact:** Critical. A tampered binary would run with full user privileges on every subsequent invocation.

**Mitigations:**

- The download URL is hardcoded to `github.com`. It is not user-configurable.
- HTTPS is used with system TLS.
- The replacement is atomic (`os.Rename` from a `.tmp` file), so a failed download does not corrupt the existing binary.
- Homebrew installs (the primary distribution channel) use `brew upgrade`, which verifies the formula's SHA-256 bottle checksum.
- Auto-update can be disabled with `OPENBOOT_DISABLE_AUTOUPDATE=1` or by setting `~/.openboot/config.json` `autoupdate` to `"false"` or `"notify"`.

**Residual risk:** Direct binary installs (non-Homebrew) have no post-download integrity check. The `OPENBOOT_UPGRADING=1` guard prevents infinite re-exec loops but does not authenticate the downloaded binary.

**Recommendation:** Prefer Homebrew installation, which provides formula-level SHA-256 verification. If using direct binary install in a managed fleet, set `OPENBOOT_DISABLE_AUTOUPDATE=1` and control updates out-of-band.

---

### T6 — Auth Token Exposure

**Description:** The bearer token issued by openboot.dev is stored in `~/.openboot/auth.json`. If this file is read by another process or included in a backup that leaks, the token grants API access as the authenticated user.

**Likelihood:** Low on a well-administered machine.

**Impact:** Medium. An attacker with the token can push configs or snapshots as the victim user. They cannot run code on the victim's machine with the token alone.

**Mitigations:**

- `SaveToken` creates `~/.openboot/auth.json` with permissions `0600` (owner read/write only).
- The `~/.openboot/` directory is created with `0700`.
- Tokens have an `expires_at` timestamp. `LoadToken` returns `nil` for expired tokens, and the server enforces expiry on the API side.
- `openboot logout` calls `DeleteToken`, which removes the file.

**Residual risk:** Root processes on the machine can read any file regardless of permissions. Processes running as the same user can also read the token. This is the standard threat model for any bearer-token CLI credential store (similar to `~/.npmrc`, `~/.netrc`, or AWS CLI credentials).

---

### T7 — macOS Preferences Injection

**Description:** A remote config can include `macos_prefs` — an array of `defaults write` commands applied to arbitrary domains. A malicious config could write values that affect security-relevant system preferences.

**Likelihood:** Medium. Any config author can include `macos_prefs`.

**Impact:** Medium. `defaults write` operates on user-space preferences. It cannot modify system-level settings that require SIP bypass or root. However, some user preferences affect security behavior (e.g., Gatekeeper user overrides, quarantine flags, screensaver lock settings).

**Mitigations:**

- `RemoteConfig.Validate()` checks that `macos_prefs[*].type` is one of `string`, `int`, `bool`, `float`, or empty. This prevents type confusion but does not restrict domain/key pairs.
- `Configure` passes each preference as discrete arguments to `exec.Command("defaults", "write", domain, key, ...)`. Values are not shell-interpolated.
- In interactive mode, the user confirms the full package list before installation begins.

**Residual risk:** There is no allowlist of permitted `defaults write` domains or keys. A config author can set any user-writable preference. Users should not apply configs from authors they do not trust.

---

### T8 — OPENBOOT_API_URL Redirection

**Description:** The `OPENBOOT_API_URL` environment variable overrides the API base URL. If an attacker can set environment variables before running `openboot` (e.g., via a compromised `.zshrc` that a prior dotfiles install wrote), they could redirect API calls to an attacker-controlled server.

**Likelihood:** Very low in normal use.

**Impact:** High if exploited. A malicious API server could return crafted configs, capture the auth token sent in `Authorization` headers, or respond with packages that exploit the validation boundary.

**Mitigations:**

- `IsAllowedAPIURL` in `internal/system/apiurl.go` rejects any URL that is not `https://` or `http://localhost` / `http://127.0.0.1` / `http://[::1]`. Plaintext HTTP to non-localhost addresses is rejected.
- Both `config.getAPIBase()` and `auth.GetAPIBase()` apply this check and log a warning before falling back to `https://openboot.dev`.

**Residual risk:** An attacker who already controls the user's shell environment has many more effective attack vectors. This mitigation is adequate for the threat.

---

### T9 — Snapshot Import from Untrusted URL

**Description:** `openboot snapshot --import <url>` accepts an HTTPS URL and fetches a snapshot file. The content is parsed as a `RemoteConfig` or snapshot and can trigger package installation, dotfiles cloning, and macOS pref writes.

**Likelihood:** Low (requires user to be socially engineered into running the command with an attacker URL).

**Impact:** High. Same impact surface as T1–T3.

**Mitigations:**

- The same `ValidateDotfilesURL` and `RemoteConfig.Validate()` paths run on imported content.
- Response body is capped at 1 MiB.
- Snapshot files imported via `--import` do not contain `post_install` (the `loadSnapshotAsRemoteConfig` function does not populate that field; a code comment explicitly notes this).

**Residual risk:** Package names and dotfiles URLs in an imported snapshot are trusted after regex validation. The same caveats as T2 and T3 apply.

---

### T10 — Shell Theme and Plugin Injection via Snapshot Restore

**Description:** The `RestoreFromSnapshot` path in `internal/shell/shell.go` writes `ZSH_THEME` and `plugins=(...)` into `~/.zshrc`. Values come from a snapshot or remote config.

**Likelihood:** Low.

**Impact:** Low to Medium. If `ZSH_THEME` or plugin names contain shell metacharacters, they could escape the quoted context in `.zshrc` and inject shell code that runs at login.

**Mitigations:**

- `validateShellIdentifier` enforces `^[a-zA-Z0-9_.-]+$` on theme names and each plugin name before they are written to `.zshrc`. This eliminates all shell metacharacters.
- `buildRestoreBlock` wraps the theme in double quotes and the plugins list in parentheses, matching standard `.zshrc` syntax. The validated values cannot break out of these constructs.

**Residual risk:** Negligible given the strict character allowlist.

---

## 4. Intentionally Unsupported Capabilities

The following were considered and deliberately excluded:

**Unsigned / HTTP package sources.** `brew install` of taps from arbitrary HTTP sources is not supported. Tap names must match `owner/repo` and are resolved through Homebrew's tap infrastructure, which requires HTTPS.

**`git@` SSH dotfiles URLs.** `ValidateDotfilesURL` rejects any URL that does not begin with `https://`. SSH URLs are excluded to prevent unknown host key acceptance from introducing a trust escalation vector.

**Arbitrary URL in `OPENBOOT_API_URL`.** Plaintext HTTP to non-localhost is rejected. This prevents a local network attacker from trivially intercepting API traffic on misconfigured networks.

**`sudo` calls in core code.** No `sudo` is invoked by OpenBoot directly. Steps that require elevated privileges (Homebrew prefix creation, Xcode CLT install) invoke their own privilege escalation flows.

**`post_install` in snapshot imports.** When a local snapshot file is imported, the `post_install` field is intentionally not populated from the file. Post-install scripts only run when sourced from a live remote config with explicit user opt-in.

**Unbounded goroutines.** Brew parallel installs are capped at 4 workers. This limits resource exhaustion from large package lists.

---

## 5. Reporting Vulnerabilities

To report a security vulnerability, email the maintainers at the address in the repository's `go.mod` module path domain (`openbootdotdev`). Do not open a public GitHub issue for vulnerabilities that could be exploited before a patch is released.

Include:
- A description of the vulnerability and affected code path
- Steps to reproduce or a proof-of-concept
- Assessed impact

If the report involves an exposed credential (e.g., a hardcoded secret found in the repository), rotate the credential immediately and include the rotation confirmation in your report.
