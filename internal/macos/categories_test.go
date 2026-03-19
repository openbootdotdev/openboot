package macos

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultCategories_NotEmpty(t *testing.T) {
	assert.Greater(t, len(DefaultCategories), 0)
}

func TestDefaultCategories_HasRequiredFields(t *testing.T) {
	for _, cat := range DefaultCategories {
		assert.NotEmpty(t, cat.Name, "category Name should not be empty")
		assert.NotEmpty(t, cat.Icon, "category Icon should not be empty")
		assert.Greater(t, len(cat.Prefs), 0, "category %q should have at least one preference", cat.Name)
	}
}

func TestDefaultCategories_ExpectedNames(t *testing.T) {
	names := make(map[string]bool)
	for _, cat := range DefaultCategories {
		names[cat.Name] = true
	}
	for _, expected := range []string{"System", "Finder", "Dock", "Screenshots", "Safari", "TextEdit", "TimeMachine"} {
		assert.True(t, names[expected], "expected category %q to exist", expected)
	}
}

func TestDefaultPreferences_DerivedFromCategories(t *testing.T) {
	// DefaultPreferences must be exactly the flat concatenation of all category prefs.
	var expected []Preference
	for _, cat := range DefaultCategories {
		expected = append(expected, cat.Prefs...)
	}
	assert.Equal(t, expected, DefaultPreferences,
		"DefaultPreferences must equal the flat concat of DefaultCategories prefs")
}

func TestDefaultPreferences_NoDuplicateKeys(t *testing.T) {
	seen := make(map[string]bool)
	for _, p := range DefaultPreferences {
		k := PrefKey(p)
		assert.False(t, seen[k], "duplicate PrefKey %q", k)
		seen[k] = true
	}
}

func TestPrefKey_Format(t *testing.T) {
	p := Preference{Domain: "com.apple.finder", Key: "ShowPathbar"}
	assert.Equal(t, "com.apple.finder/ShowPathbar", PrefKey(p))
}

func TestPrefKey_UniqueAcrossCategories(t *testing.T) {
	keys := make(map[string]bool)
	for _, cat := range DefaultCategories {
		for _, p := range cat.Prefs {
			k := PrefKey(p)
			assert.False(t, keys[k], "duplicate PrefKey %q in categories", k)
			keys[k] = true
		}
	}
}

func TestAllPrefsSelected_CountMatchesDefaultPreferences(t *testing.T) {
	selected := AllPrefsSelected()
	assert.Equal(t, len(DefaultPreferences), len(selected))
}

func TestAllPrefsSelected_AllTrue(t *testing.T) {
	selected := AllPrefsSelected()
	for k, v := range selected {
		assert.True(t, v, "expected preference %q to be selected", k)
	}
}

func TestAllPrefsSelected_KeysMatchDefaultCategories(t *testing.T) {
	selected := AllPrefsSelected()
	for _, cat := range DefaultCategories {
		for _, p := range cat.Prefs {
			k := PrefKey(p)
			_, ok := selected[k]
			assert.True(t, ok, "expected key %q to be present in AllPrefsSelected", k)
		}
	}
}

func TestDefaultCategories_PrefsHaveRequiredFields(t *testing.T) {
	validTypes := map[string]bool{"bool": true, "int": true, "float": true, "string": true}
	for _, cat := range DefaultCategories {
		for _, p := range cat.Prefs {
			assert.NotEmpty(t, p.Domain, "pref in category %q has empty Domain", cat.Name)
			assert.NotEmpty(t, p.Key, "pref in category %q has empty Key", cat.Name)
			assert.NotEmpty(t, p.Desc, "pref in category %q has empty Desc", cat.Name)
			assert.True(t, validTypes[p.Type], "pref %q in category %q has invalid Type %q", p.Key, cat.Name, p.Type)
		}
	}
}

func TestDefaultCategories_FinderCategory(t *testing.T) {
	var finder *PrefCategory
	for i := range DefaultCategories {
		if DefaultCategories[i].Name == "Finder" {
			finder = &DefaultCategories[i]
			break
		}
	}
	require.NotNil(t, finder, "Finder category must exist")
	assert.Equal(t, "📁", finder.Icon)
	assert.Greater(t, len(finder.Prefs), 0)
}

func TestDefaultCategories_DockCategory(t *testing.T) {
	var dock *PrefCategory
	for i := range DefaultCategories {
		if DefaultCategories[i].Name == "Dock" {
			dock = &DefaultCategories[i]
			break
		}
	}
	require.NotNil(t, dock, "Dock category must exist")
	assert.Greater(t, len(dock.Prefs), 0)
}
