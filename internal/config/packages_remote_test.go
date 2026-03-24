package config

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// saveAndRestoreCategories snapshots the global Categories and restores it
// after the test, so merge tests don't pollute each other.
func saveAndRestoreCategories(t *testing.T) {
	t.Helper()
	orig := make([]Category, len(Categories))
	for i, cat := range Categories {
		pkgs := make([]Package, len(cat.Packages))
		copy(pkgs, cat.Packages)
		orig[i] = Category{Name: cat.Name, Icon: cat.Icon, Packages: pkgs}
	}
	t.Cleanup(func() { Categories = orig })
}

// --- Cache tests ---

func TestWriteAndReadPackagesCache(t *testing.T) {
	dir := t.TempDir()
	origCacheDir := cacheDir
	cacheDir = func() string { return dir }
	t.Cleanup(func() { cacheDir = origCacheDir })

	pkgs := []remotePackage{
		{Name: "git", Desc: "VCS", Category: "essential", Installer: "formula"},
		{Name: "docker", Desc: "Containers", Category: "development", Installer: "cask"},
	}

	err := writePackagesCache(pkgs)
	require.NoError(t, err)

	// File exists with correct permissions.
	info, err := os.Stat(filepath.Join(dir, packagesCacheFile))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// Read back.
	got, err := readPackagesCache()
	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.Equal(t, "git", got[0].Name)
	assert.Equal(t, "docker", got[1].Name)
}

func TestReadPackagesCache_Expired(t *testing.T) {
	dir := t.TempDir()
	origCacheDir := cacheDir
	cacheDir = func() string { return dir }
	t.Cleanup(func() { cacheDir = origCacheDir })

	// Write a cache entry from 25 hours ago (beyond 24h TTL).
	entry := packagesCacheEntry{
		FetchedAt: time.Now().Add(-25 * time.Hour),
		Packages:  []remotePackage{{Name: "stale"}},
	}
	data, _ := json.Marshal(entry)
	os.WriteFile(filepath.Join(dir, packagesCacheFile), data, 0600)

	_, err := readPackagesCache()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cache expired")
}

func TestReadPackagesCache_Missing(t *testing.T) {
	dir := t.TempDir()
	origCacheDir := cacheDir
	cacheDir = func() string { return dir }
	t.Cleanup(func() { cacheDir = origCacheDir })

	_, err := readPackagesCache()
	assert.Error(t, err) // file not found
}

func TestReadPackagesCache_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	origCacheDir := cacheDir
	cacheDir = func() string { return dir }
	t.Cleanup(func() { cacheDir = origCacheDir })

	os.WriteFile(filepath.Join(dir, packagesCacheFile), []byte("not json"), 0600)

	_, err := readPackagesCache()
	assert.Error(t, err)
}

// --- Fetch tests ---

func TestFetchRemotePackages_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/packages", r.URL.Path)
		json.NewEncoder(w).Encode(remotePackagesResponse{
			Packages: []remotePackage{
				{Name: "git", Desc: "VCS", Category: "essential", Installer: "formula"},
			},
		})
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	pkgs, err := fetchRemotePackages()
	require.NoError(t, err)
	assert.Len(t, pkgs, 1)
	assert.Equal(t, "git", pkgs[0].Name)
}

func TestFetchRemotePackages_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	_, err := fetchRemotePackages()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestFetchRemotePackages_NetworkError(t *testing.T) {
	t.Setenv("OPENBOOT_API_URL", "http://localhost:1")

	_, err := fetchRemotePackages()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "fetch packages")
}

func TestFetchRemotePackages_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	t.Setenv("OPENBOOT_API_URL", server.URL)

	_, err := fetchRemotePackages()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse packages")
}

// --- Merge tests ---

func TestMergeRemotePackages_UpdatesDescriptions(t *testing.T) {
	saveAndRestoreCategories(t)

	// Set up a category with a package missing its description.
	Categories = []Category{
		{Name: "Essential", Packages: []Package{
			{Name: "git", Description: ""},
		}},
	}

	mergeRemotePackages([]remotePackage{
		{Name: "git", Desc: "Distributed VCS", Category: "essential", Installer: "formula"},
	})

	assert.Equal(t, "Distributed VCS", Categories[0].Packages[0].Description)
}

func TestMergeRemotePackages_DoesNotOverwriteExistingDescription(t *testing.T) {
	saveAndRestoreCategories(t)

	Categories = []Category{
		{Name: "Essential", Packages: []Package{
			{Name: "git", Description: "Original desc"},
		}},
	}

	mergeRemotePackages([]remotePackage{
		{Name: "git", Desc: "New desc", Category: "essential", Installer: "formula"},
	})

	assert.Equal(t, "Original desc", Categories[0].Packages[0].Description)
}

func TestMergeRemotePackages_UpdatesInstallerFlags(t *testing.T) {
	saveAndRestoreCategories(t)

	Categories = []Category{
		{Name: "Development", Packages: []Package{
			{Name: "typescript", Description: "TS", IsCask: false, IsNpm: false},
			{Name: "docker", Description: "Containers", IsCask: false, IsNpm: false},
		}},
	}

	mergeRemotePackages([]remotePackage{
		{Name: "typescript", Desc: "TS", Category: "development", Installer: "npm"},
		{Name: "docker", Desc: "Containers", Category: "development", Installer: "cask"},
	})

	assert.True(t, Categories[0].Packages[0].IsNpm, "typescript should be marked npm")
	assert.True(t, Categories[0].Packages[1].IsCask, "docker should be marked cask")
}

func TestMergeRemotePackages_AddsNewPackages(t *testing.T) {
	saveAndRestoreCategories(t)

	Categories = []Category{
		{Name: "Essential", Packages: []Package{
			{Name: "git"},
		}},
	}

	mergeRemotePackages([]remotePackage{
		{Name: "git", Desc: "VCS", Category: "essential", Installer: "formula"},
		{Name: "slack", Desc: "Chat", Category: "productivity", Installer: "cask"},
		{Name: "redis", Desc: "Cache", Category: "optional", Installer: "formula"},
	})

	// "git" was existing, so Essential should still have 1.
	assert.Len(t, Categories[0].Packages, 1)

	// "slack" and "redis" are new → new categories created.
	found := map[string]bool{}
	for _, cat := range Categories {
		for _, pkg := range cat.Packages {
			found[pkg.Name] = true
		}
	}
	assert.True(t, found["slack"], "slack should be added")
	assert.True(t, found["redis"], "redis should be added")
}

func TestMergeRemotePackages_NewCaskPackageHasFlag(t *testing.T) {
	saveAndRestoreCategories(t)

	Categories = []Category{}

	mergeRemotePackages([]remotePackage{
		{Name: "warp", Desc: "Terminal", Category: "productivity", Installer: "cask"},
		{Name: "eslint", Desc: "Linter", Category: "development", Installer: "npm"},
	})

	for _, cat := range Categories {
		for _, pkg := range cat.Packages {
			if pkg.Name == "warp" {
				assert.True(t, pkg.IsCask, "warp should be cask")
				assert.False(t, pkg.IsNpm)
			}
			if pkg.Name == "eslint" {
				assert.True(t, pkg.IsNpm, "eslint should be npm")
				assert.False(t, pkg.IsCask)
			}
		}
	}
}

// --- loadRemotePackages integration test ---

func TestLoadRemotePackages_UsesCacheThenFallsToNetwork(t *testing.T) {
	dir := t.TempDir()
	origCacheDir := cacheDir
	cacheDir = func() string { return dir }
	t.Cleanup(func() { cacheDir = origCacheDir })

	// No cache, no server → error.
	t.Setenv("OPENBOOT_API_URL", "http://localhost:1")
	_, err := loadRemotePackages()
	assert.Error(t, err)

	// Write fresh cache.
	entry := packagesCacheEntry{
		FetchedAt: time.Now(),
		Packages:  []remotePackage{{Name: "cached-pkg"}},
	}
	data, _ := json.Marshal(entry)
	os.WriteFile(filepath.Join(dir, packagesCacheFile), data, 0600)

	// Cache hit → no network call needed.
	pkgs, err := loadRemotePackages()
	require.NoError(t, err)
	assert.Equal(t, "cached-pkg", pkgs[0].Name)
}

// --- RefreshPackagesFromRemote integration test ---

func TestRefreshPackagesFromRemote_FallsBackSilently(t *testing.T) {
	saveAndRestoreCategories(t)

	dir := t.TempDir()
	origCacheDir := cacheDir
	cacheDir = func() string { return dir }
	t.Cleanup(func() { cacheDir = origCacheDir })

	originalLen := len(Categories)
	t.Setenv("OPENBOOT_API_URL", "http://localhost:1")

	// Should not panic, should not change Categories.
	RefreshPackagesFromRemote()
	assert.Equal(t, originalLen, len(Categories))
}
