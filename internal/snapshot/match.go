package snapshot

import (
	"github.com/openbootdotdev/openboot/internal/config"
)

// MatchPackages compares the snapshot's installed packages against the catalog.
// Returns a CatalogMatch with matched/unmatched packages and match rate.
func MatchPackages(snap *Snapshot) *CatalogMatch {
	catalogSet := make(map[string]bool)
	for _, cat := range config.Categories {
		for _, pkg := range cat.Packages {
			catalogSet[pkg.Name] = true
		}
	}

	allPkgs := append([]string{}, snap.Packages.Formulae...)
	allPkgs = append(allPkgs, snap.Packages.Casks...)
	allPkgs = append(allPkgs, snap.Packages.Npm...)

	matched := []string{}
	unmatched := []string{}

	for _, pkg := range allPkgs {
		if catalogSet[pkg] {
			matched = append(matched, pkg)
		} else {
			unmatched = append(unmatched, pkg)
		}
	}

	matchRate := 0.0
	if len(allPkgs) > 0 {
		matchRate = float64(len(matched)) / float64(len(allPkgs))
	}

	return &CatalogMatch{
		Matched:   matched,
		Unmatched: unmatched,
		MatchRate: matchRate,
	}
}

// DetectBestPreset finds the preset with highest Jaccard similarity to the snapshot.
// Returns preset name if similarity >= 0.3, otherwise returns empty string.
func DetectBestPreset(snap *Snapshot) string {
	snapshotSet := make(map[string]bool)
	for _, pkg := range snap.Packages.Formulae {
		snapshotSet[pkg] = true
	}
	for _, pkg := range snap.Packages.Casks {
		snapshotSet[pkg] = true
	}
	for _, pkg := range snap.Packages.Npm {
		snapshotSet[pkg] = true
	}

	snapshotPkgs := make([]string, 0, len(snapshotSet))
	for pkg := range snapshotSet {
		snapshotPkgs = append(snapshotPkgs, pkg)
	}

	bestPreset := ""
	bestSimilarity := 0.0

	for presetName, preset := range config.Presets {
		presetPkgs := append([]string{}, preset.CLI...)
		presetPkgs = append(presetPkgs, preset.Cask...)
		presetPkgs = append(presetPkgs, preset.Npm...)

		similarity := jaccardSimilarity(snapshotPkgs, presetPkgs)

		if similarity > bestSimilarity {
			bestSimilarity = similarity
			bestPreset = presetName
		}
	}

	if bestSimilarity >= 0.3 {
		return bestPreset
	}

	return ""
}

// jaccardSimilarity computes |A ∩ B| / |A ∪ B| for two string slices.
func jaccardSimilarity(setA, setB []string) float64 {
	mapA := make(map[string]bool)
	for _, item := range setA {
		mapA[item] = true
	}

	mapB := make(map[string]bool)
	for _, item := range setB {
		mapB[item] = true
	}

	intersection := 0
	for item := range mapA {
		if mapB[item] {
			intersection++
		}
	}

	union := len(mapA) + len(mapB) - intersection

	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}
