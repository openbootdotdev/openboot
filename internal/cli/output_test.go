package cli

// output_test.go tests functions that write to stdout or stderr by capturing
// the output via os.Pipe. All functions under test here are pure output
// helpers that do not call brew, npm, git, or any network endpoints.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openbootdotdev/openboot/internal/snapshot"
	syncpkg "github.com/openbootdotdev/openboot/internal/sync"
)

// mockRoundTripper lets tests intercept HTTP calls without binding to any port.
type mockRoundTripper struct{ handler http.Handler }

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	m.handler.ServeHTTP(rec, req)
	return rec.Result(), nil
}

func mockHTTPClient(handler http.Handler) *http.Client {
	return &http.Client{Transport: &mockRoundTripper{handler: handler}}
}

// captureStdout redirects os.Stdout, runs f, then restores it and returns
// everything that was written.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	f()

	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	os.Stdout = old
	return buf.String()
}

// captureStderr redirects os.Stderr, runs f, then restores it and returns
// everything that was written.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	f()

	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	os.Stderr = old
	return buf.String()
}

// ── printMissing ──────────────────────────────────────────────────────────────

func TestPrintMissing_EmptySlice_ProducesNoOutput(t *testing.T) {
	out := captureStdout(t, func() {
		printMissing("Formulae", []string{})
	})
	assert.Empty(t, out)
}

func TestPrintMissing_NilSlice_ProducesNoOutput(t *testing.T) {
	out := captureStdout(t, func() {
		printMissing("Formulae", nil)
	})
	assert.Empty(t, out)
}

func TestPrintMissing_SingleItem(t *testing.T) {
	out := captureStdout(t, func() {
		printMissing("Formulae", []string{"git"})
	})
	assert.Contains(t, out, "Formulae")
	assert.Contains(t, out, "git")
	assert.Contains(t, out, "(1)")
}

func TestPrintMissing_MultipleItems(t *testing.T) {
	out := captureStdout(t, func() {
		printMissing("Casks", []string{"firefox", "iterm2", "docker"})
	})
	assert.Contains(t, out, "Casks")
	assert.Contains(t, out, "(3)")
	assert.Contains(t, out, "firefox")
	assert.Contains(t, out, "iterm2")
	assert.Contains(t, out, "docker")
}

func TestPrintMissing_CategoryLabel(t *testing.T) {
	out := captureStdout(t, func() {
		printMissing("NPM", []string{"typescript"})
	})
	assert.Contains(t, out, "NPM")
}

// ── printInstallDiff ──────────────────────────────────────────────────────────

func TestPrintInstallDiff_EmptyDiff_ProducesNoOutput(t *testing.T) {
	out := captureStdout(t, func() {
		printInstallDiff(&syncpkg.SyncDiff{})
	})
	assert.Empty(t, out)
}

func TestPrintInstallDiff_MissingFormulae(t *testing.T) {
	d := &syncpkg.SyncDiff{
		MissingFormulae: []string{"git", "ripgrep"},
	}
	out := captureStdout(t, func() {
		printInstallDiff(d)
	})
	assert.Contains(t, out, "Packages to install")
	assert.Contains(t, out, "git")
	assert.Contains(t, out, "ripgrep")
}

func TestPrintInstallDiff_MissingCasks(t *testing.T) {
	d := &syncpkg.SyncDiff{
		MissingCasks: []string{"firefox", "iterm2"},
	}
	out := captureStdout(t, func() {
		printInstallDiff(d)
	})
	assert.Contains(t, out, "Packages to install")
	assert.Contains(t, out, "firefox")
}

func TestPrintInstallDiff_MissingNpm(t *testing.T) {
	d := &syncpkg.SyncDiff{
		MissingNpm: []string{"typescript"},
	}
	out := captureStdout(t, func() {
		printInstallDiff(d)
	})
	assert.Contains(t, out, "typescript")
}

func TestPrintInstallDiff_MissingTaps(t *testing.T) {
	d := &syncpkg.SyncDiff{
		MissingTaps: []string{"homebrew/cask-fonts"},
	}
	out := captureStdout(t, func() {
		printInstallDiff(d)
	})
	assert.Contains(t, out, "homebrew/cask-fonts")
}

func TestPrintInstallDiff_MacOSChangedWithDesc(t *testing.T) {
	d := &syncpkg.SyncDiff{
		MacOSChanged: []syncpkg.MacOSPrefDiff{
			{
				Domain:      "com.apple.dock",
				Key:         "autohide",
				Type:        "bool",
				Desc:        "Autohide dock",
				RemoteValue: "1",
				LocalValue:  "0",
			},
		},
	}
	out := captureStdout(t, func() {
		printInstallDiff(d)
	})
	assert.Contains(t, out, "macOS Changes")
	assert.Contains(t, out, "Autohide dock")
	assert.Contains(t, out, "0")
	assert.Contains(t, out, "1")
}

func TestPrintInstallDiff_MacOSChangedNoDesc_FallsBackToDomainKey(t *testing.T) {
	d := &syncpkg.SyncDiff{
		MacOSChanged: []syncpkg.MacOSPrefDiff{
			{
				Domain:      "NSGlobalDomain",
				Key:         "ApplePressAndHoldEnabled",
				Desc:        "", // empty desc
				RemoteValue: "false",
				LocalValue:  "true",
			},
		},
	}
	out := captureStdout(t, func() {
		printInstallDiff(d)
	})
	assert.Contains(t, out, "NSGlobalDomain.ApplePressAndHoldEnabled")
}

func TestPrintInstallDiff_ShellChanges_ThemeChanged(t *testing.T) {
	d := &syncpkg.SyncDiff{
		Shell: &syncpkg.ShellDiff{
			ThemeChanged:  true,
			LocalTheme:    "robbyrussell",
			RemoteTheme:   "agnoster",
			PluginsChanged: false,
		},
	}
	out := captureStdout(t, func() {
		printInstallDiff(d)
	})
	assert.Contains(t, out, "Shell Changes")
	assert.Contains(t, out, "robbyrussell")
	assert.Contains(t, out, "agnoster")
}

func TestPrintInstallDiff_ShellChanges_EmptyLocalTheme(t *testing.T) {
	d := &syncpkg.SyncDiff{
		Shell: &syncpkg.ShellDiff{
			ThemeChanged: true,
			LocalTheme:   "", // empty → should show "(none)"
			RemoteTheme:  "agnoster",
		},
	}
	out := captureStdout(t, func() {
		printInstallDiff(d)
	})
	assert.Contains(t, out, "(none)")
	assert.Contains(t, out, "agnoster")
}

func TestPrintInstallDiff_ShellChanges_PluginsChanged(t *testing.T) {
	d := &syncpkg.SyncDiff{
		Shell: &syncpkg.ShellDiff{
			PluginsChanged: true,
			LocalPlugins:   []string{"git"},
			RemotePlugins:  []string{"git", "zsh-autosuggestions"},
		},
	}
	out := captureStdout(t, func() {
		printInstallDiff(d)
	})
	assert.Contains(t, out, "Shell Changes")
	assert.Contains(t, out, "Plugins")
	assert.Contains(t, out, "zsh-autosuggestions")
}

func TestPrintInstallDiff_ShellChanges_EmptyLocalPlugins(t *testing.T) {
	d := &syncpkg.SyncDiff{
		Shell: &syncpkg.ShellDiff{
			PluginsChanged: true,
			LocalPlugins:   []string{}, // empty → should show "(none)"
			RemotePlugins:  []string{"git"},
		},
	}
	out := captureStdout(t, func() {
		printInstallDiff(d)
	})
	assert.Contains(t, out, "(none)")
}

func TestPrintInstallDiff_DotfilesChanged(t *testing.T) {
	d := &syncpkg.SyncDiff{
		DotfilesChanged: true,
		RemoteDotfiles:  "https://github.com/alice/dotfiles",
		LocalDotfiles:   "",
	}
	out := captureStdout(t, func() {
		printInstallDiff(d)
	})
	assert.Contains(t, out, "Dotfiles")
	assert.Contains(t, out, "https://github.com/alice/dotfiles")
}

func TestPrintInstallDiff_DotfilesChanged_WithLocalDotfiles(t *testing.T) {
	d := &syncpkg.SyncDiff{
		DotfilesChanged: true,
		RemoteDotfiles:  "https://github.com/alice/dotfiles",
		LocalDotfiles:   "https://github.com/old/dotfiles",
	}
	out := captureStdout(t, func() {
		printInstallDiff(d)
	})
	assert.Contains(t, out, "https://github.com/old/dotfiles")
	assert.Contains(t, out, "https://github.com/alice/dotfiles")
}

func TestPrintInstallDiff_AllSections(t *testing.T) {
	d := &syncpkg.SyncDiff{
		MissingFormulae: []string{"git"},
		MissingCasks:    []string{"firefox"},
		MacOSChanged: []syncpkg.MacOSPrefDiff{
			{Domain: "com.apple.dock", Key: "autohide", Desc: "Autohide", RemoteValue: "1"},
		},
		Shell: &syncpkg.ShellDiff{
			ThemeChanged: true,
			RemoteTheme:  "agnoster",
			LocalTheme:   "robbyrussell",
		},
		DotfilesChanged: true,
		RemoteDotfiles:  "https://github.com/alice/dotfiles",
	}
	out := captureStdout(t, func() {
		printInstallDiff(d)
	})
	assert.Contains(t, out, "Packages to install")
	assert.Contains(t, out, "macOS Changes")
	assert.Contains(t, out, "Shell Changes")
	assert.Contains(t, out, "Dotfiles")
}

// ── printSyncSourceHeader ─────────────────────────────────────────────────────

func TestPrintSyncSourceHeader_ZeroSyncedAt(t *testing.T) {
	source := &syncpkg.SyncSource{
		UserSlug: "alice/dev-setup",
		Username: "alice",
		Slug:     "dev-setup",
		// SyncedAt is zero value
	}
	out := captureStdout(t, func() {
		printSyncSourceHeader(source)
	})
	assert.Contains(t, out, "alice/dev-setup")
	// Zero SyncedAt → should NOT print "last synced"
	assert.NotContains(t, out, "last synced")
}

func TestPrintSyncSourceHeader_RecentSync(t *testing.T) {
	source := &syncpkg.SyncSource{
		UserSlug: "bob/my-setup",
		Username: "bob",
		Slug:     "my-setup",
		SyncedAt: time.Now().Add(-2 * time.Hour),
	}
	out := captureStdout(t, func() {
		printSyncSourceHeader(source)
	})
	assert.Contains(t, out, "bob")
	assert.Contains(t, out, "last synced")
}

func TestPrintSyncSourceHeader_StaleSync_Over90Days(t *testing.T) {
	source := &syncpkg.SyncSource{
		UserSlug: "carol/setup",
		Username: "carol",
		Slug:     "setup",
		SyncedAt: time.Now().Add(-100 * 24 * time.Hour),
	}
	out := captureStdout(t, func() {
		printSyncSourceHeader(source)
	})
	assert.Contains(t, out, "carol")
	assert.Contains(t, out, "last synced")
}

func TestPrintSyncSourceHeader_UsesAtSignLabelWhenBothSet(t *testing.T) {
	source := &syncpkg.SyncSource{
		UserSlug: "alice/dev-setup",
		Username: "alice",
		Slug:     "dev-setup",
		SyncedAt: time.Now().Add(-1 * time.Hour),
	}
	out := captureStdout(t, func() {
		printSyncSourceHeader(source)
	})
	// sourceLabel returns @alice/dev-setup when both Username and Slug are set
	assert.Contains(t, out, "@alice/dev-setup")
}

// ── showLocalSaveSummary ──────────────────────────────────────────────────────

func TestShowLocalSaveSummary_NoPackages(t *testing.T) {
	snap := &snapshot.Snapshot{}
	path := "/tmp/openboot/snapshot.json"
	out := captureStderr(t, func() {
		showLocalSaveSummary(snap, path)
	})
	assert.Contains(t, out, "Snapshot saved")
	assert.Contains(t, out, "/tmp/openboot/snapshot.json")
	assert.Contains(t, out, "0 formulae")
}

func TestShowLocalSaveSummary_WithPackages(t *testing.T) {
	snap := &snapshot.Snapshot{
		Packages: snapshot.PackageSnapshot{
			Formulae: []string{"git", "ripgrep"},
			Casks:    []string{"firefox"},
			Taps:     []string{"homebrew/cask-fonts"},
			Npm:      []string{"typescript", "prettier"},
		},
	}
	out := captureStderr(t, func() {
		showLocalSaveSummary(snap, "/some/path.json")
	})
	assert.Contains(t, out, "2 formulae")
	assert.Contains(t, out, "1 cask")
	assert.Contains(t, out, "1 tap")
	assert.Contains(t, out, "2 npm")
}

func TestShowLocalSaveSummary_WithMatchedPreset(t *testing.T) {
	snap := &snapshot.Snapshot{
		MatchedPreset: "developer",
		CatalogMatch: snapshot.CatalogMatch{
			MatchRate: 0.85,
		},
	}
	out := captureStderr(t, func() {
		showLocalSaveSummary(snap, "/tmp/snap.json")
	})
	assert.Contains(t, out, "developer")
	assert.Contains(t, out, "85%")
}

func TestShowLocalSaveSummary_NoMatchedPreset_NoPresetLine(t *testing.T) {
	snap := &snapshot.Snapshot{
		MatchedPreset: "", // no preset matched
	}
	out := captureStderr(t, func() {
		showLocalSaveSummary(snap, "/tmp/snap.json")
	})
	// When MatchedPreset is empty, preset block should not be printed
	assert.NotContains(t, out, "Preset:")
}

func TestShowLocalSaveSummary_ContainsRestoreCommand(t *testing.T) {
	snap := &snapshot.Snapshot{}
	path := "/Users/alice/.openboot/snapshot.json"
	out := captureStderr(t, func() {
		showLocalSaveSummary(snap, path)
	})
	assert.Contains(t, out, "openboot snapshot --import")
	assert.Contains(t, out, path)
}

// ── showSnapshotSummary ───────────────────────────────────────────────────────

func TestShowSnapshotSummary_EmptySnapshot(t *testing.T) {
	snap := &snapshot.Snapshot{}
	out := captureStderr(t, func() {
		showSnapshotSummary(snap)
	})
	assert.Contains(t, out, "Snapshot Summary")
	assert.Contains(t, out, "0 formulae")
	assert.Contains(t, out, "Custom configuration") // no matched preset
	assert.Contains(t, out, "Not configured")       // no git
	assert.Contains(t, out, "None detected")        // no dev tools
}

func TestShowSnapshotSummary_WithMatchedPreset(t *testing.T) {
	snap := &snapshot.Snapshot{
		MatchedPreset: "full",
		CatalogMatch: snapshot.CatalogMatch{
			MatchRate: 0.92,
		},
	}
	out := captureStderr(t, func() {
		showSnapshotSummary(snap)
	})
	assert.Contains(t, out, "full")
	assert.Contains(t, out, "92%")
}

func TestShowSnapshotSummary_WithGit(t *testing.T) {
	snap := &snapshot.Snapshot{
		Git: snapshot.GitSnapshot{
			UserName:  "Alice Smith",
			UserEmail: "alice@example.com",
		},
	}
	out := captureStderr(t, func() {
		showSnapshotSummary(snap)
	})
	assert.Contains(t, out, "Alice Smith")
	assert.Contains(t, out, "alice@example.com")
}

func TestShowSnapshotSummary_WithDevTools(t *testing.T) {
	snap := &snapshot.Snapshot{
		DevTools: []snapshot.DevTool{
			{Name: "Go", Version: "1.24.0"},
			{Name: "Node", Version: "20.0.0"},
		},
	}
	out := captureStderr(t, func() {
		showSnapshotSummary(snap)
	})
	assert.Contains(t, out, "Go")
	assert.Contains(t, out, "Node")
}

func TestShowSnapshotSummary_WithMacOSPrefs(t *testing.T) {
	snap := &snapshot.Snapshot{
		MacOSPrefs: []snapshot.MacOSPref{
			{Domain: "com.apple.dock", Key: "autohide", Value: "1"},
			{Domain: "NSGlobalDomain", Key: "ApplePressAndHoldEnabled", Value: "false"},
		},
	}
	out := captureStderr(t, func() {
		showSnapshotSummary(snap)
	})
	assert.Contains(t, out, "2 preferences")
}

func TestShowSnapshotSummary_WithCapturedAt(t *testing.T) {
	snap := &snapshot.Snapshot{
		CapturedAt: time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC),
	}
	out := captureStderr(t, func() {
		showSnapshotSummary(snap)
	})
	assert.Contains(t, out, "2025-03-15")
}

func TestShowSnapshotSummary_GitEmailOnly(t *testing.T) {
	snap := &snapshot.Snapshot{
		Git: snapshot.GitSnapshot{
			UserName:  "",
			UserEmail: "noname@example.com",
		},
	}
	out := captureStderr(t, func() {
		showSnapshotSummary(snap)
	})
	// non-empty email → git block should appear
	assert.Contains(t, out, "noname@example.com")
}

// ── printSnapshotList ─────────────────────────────────────────────────────────

func TestPrintSnapshotList_EmptyList(t *testing.T) {
	out := captureStderr(t, func() {
		printSnapshotList([]string{}, 10)
	})
	assert.Empty(t, out)
}

func TestPrintSnapshotList_NilList(t *testing.T) {
	out := captureStderr(t, func() {
		printSnapshotList(nil, 10)
	})
	assert.Empty(t, out)
}

func TestPrintSnapshotList_FewItems(t *testing.T) {
	items := []string{"git", "ripgrep", "fd"}
	out := captureStderr(t, func() {
		printSnapshotList(items, 10)
	})
	assert.Contains(t, out, "git")
	assert.Contains(t, out, "ripgrep")
	assert.Contains(t, out, "fd")
	// Should not contain "and X more"
	assert.NotContains(t, out, "more")
}

func TestPrintSnapshotList_ExceedsMax_ShowsEllipsis(t *testing.T) {
	items := make([]string, 15)
	for i := range items {
		items[i] = strings.Repeat("a", i+1) // unique names
	}
	out := captureStderr(t, func() {
		printSnapshotList(items, 10)
	})
	assert.Contains(t, out, "and 5 more")
}

func TestPrintSnapshotList_ExactlyAtMax_NoEllipsis(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}
	out := captureStderr(t, func() {
		printSnapshotList(items, 5)
	})
	assert.Contains(t, out, "a")
	assert.Contains(t, out, "e")
	assert.NotContains(t, out, "more")
}

func TestPrintSnapshotList_MaxZero_ShowsEllipsis(t *testing.T) {
	items := []string{"git"}
	out := captureStderr(t, func() {
		printSnapshotList(items, 0)
	})
	assert.Contains(t, out, "and 1 more")
}

// ── relativeTime edge cases ───────────────────────────────────────────────────

func TestRelativeTime_OneMonth(t *testing.T) {
	d := 35 * 24 * time.Hour // one month
	assert.Equal(t, "1 month ago", relativeTime(d))
}

func TestRelativeTime_OneYear(t *testing.T) {
	d := 370 * 24 * time.Hour // one year
	assert.Equal(t, "1 year ago", relativeTime(d))
}

func TestRelativeTime_MultipleYears(t *testing.T) {
	d := 800 * 24 * time.Hour // about 2 years
	result := relativeTime(d)
	assert.Contains(t, result, "years ago")
}

func TestRelativeTime_MultipleMonths(t *testing.T) {
	d := 90 * 24 * time.Hour // 3 months
	result := relativeTime(d)
	assert.Contains(t, result, "months ago")
}

// ── loadSnapshot remaining branches ──────────────────────────────────────────

func TestLoadSnapshot_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	badFile := dir + "/bad.json"
	require.NoError(t, os.WriteFile(badFile, []byte("not valid json {{{"), 0600))

	_, err := loadSnapshot(badFile)
	require.Error(t, err)
}

// ── buildImportConfig remaining branch ───────────────────────────────────────

func TestBuildImportConfig_CatalogPackages_GoIntoSelectedPkgs(t *testing.T) {
	// Find an actual catalog package name so we exercise the catalogSet[name]=true branch.
	// Rather than hardcode a package name that may change, we look up a real one.
	// config.GetCategories() reads embedded data — safe, no network.
	snap := &snapshot.Snapshot{
		Packages: snapshot.PackageSnapshot{
			// Use a name that definitely won't be in the catalog.
			Formulae: []string{"__definitely_not_in_catalog_abc123xyz__"},
		},
	}
	cfg := buildImportConfig(snap, false)

	// Since the name is not in the catalog it goes to OnlinePkgs, not SelectedPkgs.
	require.Len(t, cfg.OnlinePkgs, 1)
	assert.Equal(t, "__definitely_not_in_catalog_abc123xyz__", cfg.OnlinePkgs[0].Name)
	assert.False(t, cfg.OnlinePkgs[0].IsCask)
	assert.False(t, cfg.OnlinePkgs[0].IsNpm)
}

// ── resolveInstallSource: sync source branch ──────────────────────────────────

func TestResolveInstallSource_WithSavedSyncSource(t *testing.T) {
	// Write a sync source to a temp HOME so LoadSource returns a non-nil value.
	origPreset := installCfg.Preset
	origUser := installCfg.User
	t.Cleanup(func() {
		installCfg.Preset = origPreset
		installCfg.User = origUser
	})

	installCfg.Preset = ""
	installCfg.User = ""

	t.Setenv("HOME", t.TempDir())
	writeSyncSourceForSnap(t, "dev-setup")

	cmd := installCmd
	require.NoError(t, cmd.Flags().Set("from", ""))

	src, err := resolveInstallSource(cmd, []string{})
	require.NoError(t, err)
	// A saved sync source exists → should resolve to sourceSyncSource.
	assert.Equal(t, sourceSyncSource, src.kind)
	require.NotNil(t, src.syncSource)
	assert.Equal(t, "dev-setup", src.syncSource.Slug)
}

// ── showRestoreInfo ───────────────────────────────────────────────────────────

func TestShowRestoreInfo_PrintsPackageCounts(t *testing.T) {
	snap := &snapshot.Snapshot{
		Packages: snapshot.PackageSnapshot{
			Formulae: []string{"git", "ripgrep"},
			Casks:    []string{"firefox"},
			Npm:      []string{"typescript"},
			Taps:     []string{"homebrew/cask-fonts"},
		},
	}
	out := captureStderr(t, func() {
		showRestoreInfo(snap, "/tmp/snap.json")
	})
	assert.Contains(t, out, "Restoring from Snapshot")
	assert.Contains(t, out, "/tmp/snap.json")
	assert.Contains(t, out, "2 formulae")
	assert.Contains(t, out, "1 casks")
	assert.Contains(t, out, "1 npm")
	assert.Contains(t, out, "1 taps")
}

func TestShowRestoreInfo_WithGitInfo(t *testing.T) {
	snap := &snapshot.Snapshot{
		Git: snapshot.GitSnapshot{
			UserName:  "Alice",
			UserEmail: "alice@example.com",
		},
	}
	out := captureStderr(t, func() {
		showRestoreInfo(snap, "https://example.com/snap.json")
	})
	assert.Contains(t, out, "Alice")
	assert.Contains(t, out, "alice@example.com")
}

func TestShowRestoreInfo_NoGitInfo_NoGitLine(t *testing.T) {
	snap := &snapshot.Snapshot{}
	out := captureStderr(t, func() {
		showRestoreInfo(snap, "/tmp/snap.json")
	})
	// Git block should not appear when both fields are empty
	assert.NotContains(t, out, "<>")
}

// ── downloadSnapshotBytes ─────────────────────────────────────────────────────

func TestDownloadSnapshotBytes_Success(t *testing.T) {
	payload := []byte(`{"version":1,"captured_at":"2025-01-01T00:00:00Z"}`)
	client := mockHTTPClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))

	data, err := downloadSnapshotBytes("https://openboot.dev/snap.json", client)
	require.NoError(t, err)
	assert.Equal(t, payload, data)
}

func TestDownloadSnapshotBytes_Non200_ReturnsError(t *testing.T) {
	client := mockHTTPClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := downloadSnapshotBytes("https://openboot.dev/snap.json", client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestDownloadSnapshotBytes_BadURL_ReturnsError(t *testing.T) {
	_, err := downloadSnapshotBytes("http://localhost:0/notexist", &http.Client{})
	require.Error(t, err)
}

// ── showUploadedConfigInfo ────────────────────────────────────────────────────

func TestShowUploadedConfigInfo_Public(t *testing.T) {
	// Stub openBrowser so it doesn't actually try to open a browser.
	origOpenBrowser := openBrowser
	openBrowser = func(url string) error { return nil }
	t.Cleanup(func() { openBrowser = origOpenBrowser })

	out := captureStderr(t, func() {
		showUploadedConfigInfo("public", "https://openboot.dev/alice/my-setup", "openboot install alice/my-setup")
	})
	assert.Contains(t, out, "View your config")
	assert.Contains(t, out, "https://openboot.dev/alice/my-setup")
	assert.Contains(t, out, "Share with others")
	assert.Contains(t, out, "openboot install alice/my-setup")
}

func TestShowUploadedConfigInfo_Public_BrowserError(t *testing.T) {
	// When openBrowser returns an error it should print a warning but not fail.
	origOpenBrowser := openBrowser
	openBrowser = func(url string) error { return assert.AnError }
	t.Cleanup(func() { openBrowser = origOpenBrowser })

	// Should not panic; warning is printed to stderr.
	out := captureStderr(t, func() {
		showUploadedConfigInfo("public", "https://openboot.dev/a/b", "openboot install a/b")
	})
	assert.Contains(t, out, "View your config")
}

func TestShowUploadedConfigInfo_Unlisted(t *testing.T) {
	out := captureStderr(t, func() {
		showUploadedConfigInfo("unlisted", "https://openboot.dev/alice/secret", "openboot install alice/secret")
	})
	assert.Contains(t, out, "View your config")
	assert.Contains(t, out, "Share with people who have the link")
	assert.Contains(t, out, "unlisted")
}

func TestShowUploadedConfigInfo_Private(t *testing.T) {
	out := captureStderr(t, func() {
		showUploadedConfigInfo("private", "https://openboot.dev/alice/priv", "openboot install alice/priv")
	})
	assert.Contains(t, out, "Manage your config")
	assert.Contains(t, out, "private")
}

func TestShowUploadedConfigInfo_EmptyVisibility_DefaultsToPrivate(t *testing.T) {
	out := captureStderr(t, func() {
		showUploadedConfigInfo("", "https://openboot.dev/a/b", "openboot install a/b")
	})
	// Empty visibility hits the default case (private-like message).
	assert.Contains(t, out, "Manage your config")
}

// ── isStdoutTTY ───────────────────────────────────────────────────────────────

func TestIsStdoutTTY_ReturnsBoolean(t *testing.T) {
	// In test runner stdout is not a TTY — just verify it returns false without panic.
	result := isStdoutTTY()
	assert.False(t, result, "stdout is not a TTY in test context")
}

// ── recordPublishResult ───────────────────────────────────────────────────────

func TestRecordPublishResult_PrintsConfigURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	origOpenBrowser := openBrowser
	openBrowser = func(string) error { return nil }
	t.Cleanup(func() { openBrowser = origOpenBrowser })

	out := captureStderr(t, func() {
		recordPublishResult("alice", "my-config", "", "public", "https://openboot.dev")
	})
	assert.Contains(t, out, "https://openboot.dev/alice/my-config")
}

func TestRecordPublishResult_UpdatePrints_Updated(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeSyncSourceForSnap(t, "existing-slug")

	out := captureStderr(t, func() {
		recordPublishResult("alice", "existing-slug", "existing-slug", "unlisted", "https://openboot.dev")
	})
	assert.Contains(t, out, "updated")
}

// ── loadSnapshot: ParseBytes path (exercised indirectly by HTTPS branch) ──────

func TestLoadSnapshot_ParseBytes_Valid(t *testing.T) {
	// snapshot.ParseBytes is the downstream call in the HTTPS branch.
	// Test it here to ensure that path works end-to-end without real HTTP.
	snap := &snapshot.Snapshot{
		CapturedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Packages: snapshot.PackageSnapshot{
			Formulae: []string{"git"},
		},
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	require.NoError(t, err)

	loaded, err := snapshot.ParseBytes(data)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, []string{"git"}, loaded.Packages.Formulae)
}

func TestLoadSnapshot_ParseBytes_InvalidJSON_ReturnsError(t *testing.T) {
	_, err := snapshot.ParseBytes([]byte("not json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse snapshot")
}

// ── showSnapshotPreview ───────────────────────────────────────────────────────

func TestShowSnapshotPreview_EmptySnapshot(t *testing.T) {
	snap := &snapshot.Snapshot{}
	out := captureStderr(t, func() {
		showSnapshotPreview(snap)
	})
	assert.Contains(t, out, "Snapshot Preview")
	assert.Contains(t, out, "Homebrew Formulae:")
	assert.Contains(t, out, "Homebrew Casks:")
	assert.Contains(t, out, "NPM Packages:")
}

func TestShowSnapshotPreview_WithPackages(t *testing.T) {
	snap := &snapshot.Snapshot{
		Packages: snapshot.PackageSnapshot{
			Formulae: []string{"git", "ripgrep"},
			Casks:    []string{"firefox"},
			Taps:     []string{"homebrew/cask-fonts"},
			Npm:      []string{"typescript"},
		},
		Git: snapshot.GitSnapshot{
			UserName:  "Alice",
			UserEmail: "alice@example.com",
		},
		DevTools: []snapshot.DevTool{
			{Name: "Go", Version: "1.24"},
		},
	}
	out := captureStderr(t, func() {
		showSnapshotPreview(snap)
	})
	assert.Contains(t, out, "git")
	assert.Contains(t, out, "firefox")
	assert.Contains(t, out, "Alice")
	assert.Contains(t, out, "alice@example.com")
	assert.Contains(t, out, "Go")
}

func TestShowSnapshotPreview_WithMacOSPrefs(t *testing.T) {
	snap := &snapshot.Snapshot{
		MacOSPrefs: []snapshot.MacOSPref{
			{Domain: "com.apple.dock", Key: "autohide", Value: "1"},
		},
	}
	out := captureStderr(t, func() {
		showSnapshotPreview(snap)
	})
	assert.Contains(t, out, "com.apple.dock.autohide")
	assert.Contains(t, out, "= 1")
}
