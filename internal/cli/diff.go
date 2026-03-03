package cli

import (
	"fmt"
	"path/filepath"

	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/diff"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Compare your system against a config or snapshot",
	Long: `Show differences between your current system and a reference configuration.

Sources (checked in order):
  1. --from <file>         Compare against a snapshot file
  2. --user <username>     Compare against your openboot.dev config
  3. Local snapshot         Compare against ~/.openboot/snapshot.json

This is a read-only operation — nothing is installed or removed.

Examples:
  openboot diff                              Diff against local snapshot
  openboot diff --user alice/my-config       Diff against cloud config
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
		result, err = diffFromLocalSnapshot(system)
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

func diffFromLocalSnapshot(system *snapshot.Snapshot) (*diff.DiffResult, error) {
	reference, err := snapshot.LoadLocal()
	if err != nil {
		return nil, fmt.Errorf("no local snapshot found — run 'openboot snapshot --local' first, or use --from or --user flags: %w", err)
	}

	source := diff.Source{Kind: "local", Path: snapshot.LocalPath()}
	return diff.CompareSnapshots(system, reference, source), nil
}
