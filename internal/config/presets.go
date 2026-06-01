package config

import (
	"embed"

	"gopkg.in/yaml.v3"
)

//go:embed data/presets.yaml
var presetsYAML embed.FS

var Presets map[string]Preset
var presetOrder = []string{"minimal", "developer", "full"}

func init() {
	data, err := presetsYAML.ReadFile("data/presets.yaml")
	if err != nil {
		panic("corrupt binary: embedded presets.yaml unreadable: " + err.Error())
	}

	var pd presetsData
	if err := yaml.Unmarshal(data, &pd); err != nil {
		panic("corrupt binary: embedded presets.yaml unparseable: " + err.Error())
	}

	Presets = pd.Presets
}

func GetPreset(name string) (Preset, bool) {
	p, ok := Presets[name]
	return p, ok
}

func GetPresetNames() []string {
	return presetOrder
}
