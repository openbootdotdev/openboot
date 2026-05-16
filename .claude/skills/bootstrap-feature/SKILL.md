---
name: bootstrap-feature
description: Use when adding a new CLI subcommand or feature to openboot. Walks through the canonical pattern: cobra command registration, runner wiring, archtest awareness, where to put tests. Trigger when the user says "add a command", "new subcommand", "implement feature X for the CLI", or starts editing under internal/cli/.
---

# Bootstrap a new openboot feature

This skill gives the canonical recipe for adding a new CLI command or
feature to openboot without violating project invariants.

## Step 1 — Where the code goes

| Kind of thing | Location |
|---|---|
| New CLI subcommand | `internal/cli/<verb>.go`, register in `internal/cli/root.go` `init()` |
| Subprocess call (any binary) | `internal/system.RunCommand` / `RunCommandSilent` — do **not** call `exec.Command` directly |
| HTTP call (any URL) | `internal/httputil.Do` — handles 429 + Retry-After |
| Path under `~` | `os.UserHomeDir()` — never `os.Getenv("HOME")`, never hardcode `~` |
| User-visible output | `internal/ui.*` helpers — never raw `fmt.Println` |
| Destructive action | guarded by `cfg.DryRun` check |
| Error returned to caller | wrapped: `fmt.Errorf("context: %w", err)` |

These are enforced (or planned) by `internal/archtest`. The rule that
covers each invariant is listed in [AGENTS.md](../../../AGENTS.md).

## Step 2 — Cobra command skeleton

```go
// internal/cli/myverb.go
package cli

import "github.com/spf13/cobra"

func newMyVerbCmd() *cobra.Command {
	var flag string
	cmd := &cobra.Command{
		Use:   "myverb [arg]",
		Short: "One-line description",
		Long:  "Longer description if useful.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMyVerb(cmd.Context(), args[0], flag)
		},
	}
	cmd.Flags().StringVar(&flag, "flag", "", "Description of the flag")
	return cmd
}
```

Register it in `internal/cli/root.go`:

```go
rootCmd.AddCommand(newMyVerbCmd())
```

## Step 3 — Runner interface for testability

If your feature shells out, **use the Runner pattern** so L1 tests can fake
subprocess calls. Example from `internal/brew/runner.go`:

```go
type Runner interface {
	Output(args ...string) ([]byte, error)
	CombinedOutput(args ...string) ([]byte, error)
	// ...
}

// realRunner uses exec.Command — this file is in execAllowedPaths.
type realRunner struct{}

func New() *Brew { return &Brew{r: &realRunner{}} }
```

Then in tests:

```go
type fakeRunner struct { ... }
func (f *fakeRunner) Output(args ...string) ([]byte, error) { ... }
```

This is the pattern that lets the fake-runner half of L1 stay fast and hermetic.

## Step 4 — Test placement

| Scope | Tier | Build tag | Where |
|---|---|---|---|
| Pure logic + fakes | L1 | none | `<pkg>/<feature>_test.go` |
| Real subprocess in temp dir | L1 | none | `test/integration/<feature>_integration_test.go` |
| Compiled binary, no installs | L3 | `e2e` | `test/e2e/...` |
| Real installs on macOS | L4/L5 | `e2e,vm,destructive` | `test/e2e/...` |

Default to faked-runner L1 unless the thing you're testing only exists when a real
brew/git/npm is on the path — then add an integration test under `test/integration/`
(no build tag; it runs as part of L1).

## Step 5 — Verify before committing

```bash
go vet ./...
make test-unit                  # ~75s, includes archtest + integration
```

If archtest fails with a new violation, fix the code rather than
baselining — the baseline is for *intentional* exceptions and they
require justification in the commit message.

## Step 6 — Conventional commit

`feat: add openboot myverb command for X`

One thing per commit. If you also fixed an unrelated bug along the way,
split it into a separate commit.

## Common mistakes

1. **Calling `exec.Command` from the feature file** — refactor through
   `internal/system` or add a Runner. archtest will catch this.
2. **Skipping `cfg.DryRun` check** — destructive ops must be a no-op
   under `--dry-run`. Print "[DRY-RUN] Would X" instead of doing X.
3. **Hardcoding `~/`** — always `os.UserHomeDir()` then `filepath.Join`.
4. **Raw `fmt.Println`** — use `ui.Info`, `ui.Success`, `ui.Warn`, `ui.Error`.
5. **Forgetting to register the command in `root.go`** — cobra silently
   does nothing if `AddCommand` is missed.
