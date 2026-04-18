package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/openbootdotdev/openboot/internal/auth"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/push"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
	"github.com/openbootdotdev/openboot/internal/ui"
	"github.com/spf13/cobra"
)

var pushCmd = &cobra.Command{
	Use:   "push [file]",
	Short: "Push your system state to openboot.dev",
	Long: `Upload your current system state (or a local config file) to openboot.dev.

Like 'git push', running without arguments captures a snapshot of your current
Mac environment and uploads it. If a sync source is configured (from a previous
'openboot install'), it updates that config automatically.

You can also push a local JSON file directly (config or snapshot format, auto-detected).
Use --slug to target a specific existing config.`,
	Example: `  # Push current system state (auto-capture snapshot)
  openboot push

  # Push a local config or snapshot file
  openboot push config.json

  # Push and update a specific existing config
  openboot push --slug my-config
  openboot push config.json --slug my-config`,
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slug, _ := cmd.Flags().GetString("slug")
		message, _ := cmd.Flags().GetString("message")
		if len(args) == 0 {
			return runPushAuto(slug, message)
		}
		return runPush(args[0], slug, message)
	},
}

func init() {
	pushCmd.Flags().String("slug", "", "update an existing config by slug")
	pushCmd.Flags().StringP("message", "m", "", "revision message (saved in history when updating)")
	rootCmd.AddCommand(pushCmd)
}

// runPushAuto captures the current system snapshot and uploads it to openboot.dev.
// If a sync source is configured, it updates that config silently; otherwise, it
// presents an interactive picker so the user can choose an existing config or create a new one.
func runPushAuto(slugOverride, message string) error {
	apiBase := auth.GetAPIBase()

	stored, err := ensurePushAuth(apiBase)
	if err != nil {
		return err
	}

	// Capture current system state
	fmt.Fprintln(os.Stderr)
	ui.Header("Capturing system snapshot...")
	snap, err := captureEnvironment()
	if err != nil {
		return err
	}

	// Determine slug: --slug flag → sync source → interactive picker
	slug := slugOverride
	if slug == "" {
		if source, loadErr := syncpkg.LoadSource(); loadErr == nil && source != nil && source.Slug != "" {
			slug = source.Slug
		}
	}
	if slug == "" {
		slug, err = pickOrCreateConfig(stored.Token, apiBase)
		if err != nil {
			return err
		}
	}

	return pushSnapshot(snap, slug, message, stored.Token, stored.Username, apiBase)
}

func runPush(filePath, slug, message string) error {
	apiBase := auth.GetAPIBase()

	stored, err := ensurePushAuth(apiBase)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Detect format: snapshots have a "captured_at" timestamp; configs do not.
	var probe struct {
		CapturedAt string `json:"captured_at"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("parse file: %w", err)
	}

	if probe.CapturedAt != "" {
		var snap snapshot.Snapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			return fmt.Errorf("parse snapshot: %w", err)
		}
		return pushSnapshot(&snap, slug, message, stored.Token, stored.Username, apiBase)
	}
	return pushConfig(data, slug, stored.Token, stored.Username, apiBase)
}

// ensurePushAuth makes sure the user is logged in and returns the stored credentials.
// It triggers an interactive login if necessary.
func ensurePushAuth(apiBase string) (*auth.StoredAuth, error) {
	if !auth.IsAuthenticated() {
		fmt.Fprintln(os.Stderr)
		ui.Info("You need to log in to upload configs.")
		fmt.Fprintln(os.Stderr)
		if _, err := auth.LoginInteractive(apiBase); err != nil {
			return nil, fmt.Errorf("authentication failed: %w", err)
		}
	}

	stored, err := auth.LoadToken()
	if err != nil {
		return nil, fmt.Errorf("load auth token: %w", err)
	}
	if stored == nil {
		return nil, fmt.Errorf("no valid auth token found — please log in again")
	}
	return stored, nil
}

func pushSnapshot(snap *snapshot.Snapshot, slug, message, token, username, apiBase string) error {
	// Updating an existing config: skip all prompts.
	// Creating a new config: ask for name, description, visibility.
	var name, desc, visibility string
	if slug == "" {
		var err error
		name, desc, visibility, err = promptPushDetails("")
		if err != nil {
			return err
		}
	}

	result, err := push.UploadSnapshot(context.Background(), push.SnapshotOptions{
		Snapshot:   snap,
		Slug:       slug,
		Message:    message,
		Name:       name,
		Desc:       desc,
		Visibility: visibility,
		Token:      token,
		APIBase:    apiBase,
	})
	if err != nil {
		return err
	}
	renderPushSuccess(result.Slug, username)
	return nil
}

func pushConfig(data []byte, slug, token, username, apiBase string) error {
	rc, err := config.UnmarshalRemoteConfigFlexible(data)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	if err := rc.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	name, desc, visibility, err := promptPushDetails("")
	if err != nil {
		return err
	}

	result, err := push.UploadConfig(context.Background(), push.ConfigOptions{
		RemoteConfig: rc,
		Slug:         slug,
		Name:         name,
		Desc:         desc,
		Visibility:   visibility,
		Token:        token,
		APIBase:      apiBase,
	})
	if err != nil {
		return err
	}
	renderPushSuccess(result.Slug, username)
	return nil
}

// renderPushSuccess prints the uploaded-config summary to stderr. Kept in the
// CLI layer so the push package does not know about ui styling.
func renderPushSuccess(resultSlug, username string) {
	fmt.Fprintln(os.Stderr)
	ui.Success("Config uploaded successfully!")
	fmt.Fprintln(os.Stderr)
	if resultSlug != "" {
		fmt.Fprintf(os.Stderr, "  View:    https://openboot.dev/%s/%s\n", username, resultSlug)
		fmt.Fprintf(os.Stderr, "  Install: openboot -u %s/%s\n", username, resultSlug)
	}
	fmt.Fprintln(os.Stderr)
}

// apiPackage is a thin alias kept for the cli package's own tests.
// The wire type now lives in internal/push as push.APIPackage.
type apiPackage = push.APIPackage

// remoteConfigToAPIPackages exists as a thin wrapper to preserve the CLI
// package's existing unit tests. New code should call push.RemoteConfigToAPIPackages.
func remoteConfigToAPIPackages(rc *config.RemoteConfig) []apiPackage {
	return push.RemoteConfigToAPIPackages(rc)
}

// remoteConfigSummary is re-exported for tests that were written against the
// pre-refactor symbol name.
type remoteConfigSummary = push.RemoteConfigSummary

// fetchUserConfigs is a thin wrapper around push.FetchUserConfigs that
// preserves the pre-refactor signature used by tests.
func fetchUserConfigs(token, apiBase string) ([]remoteConfigSummary, error) {
	return push.FetchUserConfigs(context.Background(), token, apiBase)
}

const createNewOption = "+ Create a new config"

// pickOrCreateConfig shows an interactive list of the user's existing configs plus a
// "Create new" option. Returns the chosen slug (non-empty = update existing), or ""
// (= create new, caller must ask for name/desc/visibility).
func pickOrCreateConfig(token, apiBase string) (string, error) {
	configs, _ := fetchUserConfigs(token, apiBase) // ignore fetch errors — just show create-new

	if len(configs) == 0 {
		return "", nil // no existing configs — skip picker, go straight to create-new
	}

	options := make([]string, 0, len(configs)+1)
	for _, c := range configs {
		label := c.Slug
		if c.Name != "" && c.Name != c.Slug {
			label = fmt.Sprintf("%s — %s", c.Slug, c.Name)
		}
		options = append(options, label)
	}
	options = append(options, createNewOption)

	fmt.Fprintln(os.Stderr)
	choice, err := ui.SelectOption("Push to which config?", options)
	if err != nil {
		return "", fmt.Errorf("select config: %w", err)
	}

	if choice == createNewOption {
		return "", nil // caller will prompt for name/desc/visibility
	}

	// Extract slug from "slug — Name" label
	slug := strings.SplitN(choice, " — ", 2)[0]
	return slug, nil
}

func promptPushDetails(defaultName string) (string, string, string, error) {
	fmt.Fprintln(os.Stderr)
	var name string
	var err error
	if defaultName != "" {
		name, err = ui.InputWithDefault("Config name", "My Mac Setup", defaultName)
	} else {
		name, err = ui.Input("Config name", "My Mac Setup")
	}
	if err != nil {
		return "", "", "", fmt.Errorf("get config name: %w", err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "My Mac Setup"
	}

	fmt.Fprintln(os.Stderr)
	desc, err := ui.Input("Description (optional)", "")
	if err != nil {
		return "", "", "", fmt.Errorf("get description: %w", err)
	}
	desc = strings.TrimSpace(desc)

	fmt.Fprintln(os.Stderr)
	options := []string{
		"Public - Anyone can discover and use this config",
		"Unlisted - Only people with the link can access",
		"Private - Only you can see this config",
	}
	choice, err := ui.SelectOption("Who can see this config?", options)
	if err != nil {
		return "", "", "", fmt.Errorf("select visibility: %w", err)
	}

	visibility := "unlisted"
	switch {
	case strings.HasPrefix(choice, "Public"):
		visibility = "public"
	case strings.HasPrefix(choice, "Private"):
		visibility = "private"
	}

	return name, desc, visibility, nil
}
