package cli

import (
	"fmt"
	"path/filepath"

	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/diff"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Compare your system against a config or snapshot",
	Long: `Show differences between your current system and a reference configuration.

Sources (checked in order):
  1. --from <file>         Compare against a snapshot file
  2. --user <username>     Compare against a specific openboot.dev config
  3. Logged in             Compare against your own openboot.dev config
  4. Not logged in         Error with guidance

This is a read-only operation — nothing is installed or removed.

Examples:
  openboot diff                              Diff against your openboot.dev config (requires login)
  openboot diff --user alice/my-config       Diff against someone else's config
  openboot diff --from my-setup.json         Diff against a snapshot file
  openboot diff --json                       Output as JSON
  openboot diff --packages-only              Only compare packages`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDiff(cmd)
	},
}

func init() {
	diffCmd.Flags().String("from", "", "snapshot file to compare against")
	diffCmd.Flags().String("user", "", "alias or openboot.dev username/slug to compare against")
	diffCmd.Flags().Bool("json", false, "output diff as JSON")
	diffCmd.Flags().Bool("packages-only", false, "only compare packages")
}

func runDiff(cmd *cobra.Command) error {
	fromFile, _ := cmd.Flags().GetString("from")
	user, _ := cmd.Flags().GetString("user")
	jsonOutput, _ := cmd.Flags().GetBool("json")
	packagesOnly, _ := cmd.Flags().GetBool("packages-only")

	// Capture current system state
	system, err := captureWithUI()
	if err != nil {
		return err
	}

	var result *diff.DiffResult

	switch {
	case fromFile != "":
		result, err = diffFromFile(system, fromFile)
	case user != "":
		result, err = diffFromRemote(system, user)
	default:
		// If logged in, diff against the user's own remote config
		stored, authErr := auth.LoadToken()
		if authErr != nil || stored == nil {
			return fmt.Errorf("no comparison target specified\n\n  Log in to diff against your openboot.dev config:\n    openboot login\n\n  Or specify a target explicitly:\n    openboot diff --user <username>     Compare against a remote config\n    openboot diff --from <file>         Compare against a snapshot file")
		}
		result, err = diffFromRemote(system, stored.Username)
	}

	if err != nil {
		return err
	}

	if jsonOutput {
		data, err := diff.FormatJSON(result)
		if err != nil {
			return fmt.Errorf("format JSON: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	diff.FormatTerminal(result, packagesOnly)

	if result.HasChanges() {
		fmt.Println()
		ui.Muted("  Apply remote → local: openboot pull   •   Upload local → remote: openboot push")
	}

	return nil
}

func diffFromFile(system *snapshot.Snapshot, path string) (*diff.DiffResult, error) {
	if path == "" {
		return nil, fmt.Errorf("snapshot file path cannot be empty")
	}
	cleanPath := filepath.Clean(path)

	reference, err := snapshot.LoadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}

	source := diff.Source{Kind: "file", Path: cleanPath}
	return diff.CompareSnapshots(system, reference, source), nil
}

func diffFromRemote(system *snapshot.Snapshot, userSlug string) (*diff.DiffResult, error) {
	var token string
	if stored, err := auth.LoadToken(); err == nil && stored != nil {
		token = stored.Token
	}

	rc, err := config.FetchRemoteConfig(userSlug, token)
	if err != nil {
		return nil, fmt.Errorf("fetch remote config: %w", err)
	}

	source := diff.Source{Kind: "remote", Path: userSlug}
	return diff.CompareSnapshotToRemote(system, rc, source), nil
}

