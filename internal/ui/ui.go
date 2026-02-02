package ui

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/system"
)

var (
	accent    = lipgloss.Color("#22c55e")
	subtle    = lipgloss.Color("#666666")
	highlight = lipgloss.Color("#60a5fa")

	titleStyle = lipgloss.NewStyle().
			Foreground(accent).
			Bold(true).
			MarginBottom(1)

	successStyle = lipgloss.NewStyle().
			Foreground(accent)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ef4444"))

	mutedStyle = lipgloss.NewStyle().
			Foreground(subtle)
)

func Header(text string) {
	fmt.Println(titleStyle.Render("=== " + text + " ==="))
}

func Success(text string) {
	fmt.Println(successStyle.Render("✓ " + text))
}

func Error(text string) {
	fmt.Println(errorStyle.Render("✗ " + text))
}

func Info(text string) {
	fmt.Println("  " + text)
}

func Muted(text string) {
	fmt.Println(mutedStyle.Render(text))
}

func InputGitConfig() (name, email string, err error) {
	existingName, existingEmail := system.GetExistingGitConfig()

	name = existingName
	email = existingEmail

	nameInput := huh.NewInput().
		Title("Your name").
		Value(&name)

	emailInput := huh.NewInput().
		Title("Your email").
		Value(&email)

	if existingName == "" {
		nameInput.Placeholder("John Doe")
	}
	if existingEmail == "" {
		emailInput.Placeholder("john@example.com")
	}

	form := huh.NewForm(
		huh.NewGroup(nameInput, emailInput),
	)

	err = form.Run()
	return
}

func SelectPreset() (string, error) {
	var preset string

	options := make([]huh.Option[string], 0)
	for _, name := range config.GetPresetNames() {
		p, _ := config.GetPreset(name)
		label := fmt.Sprintf("%s - %s", name, p.Description)
		options = append(options, huh.NewOption(label, name))
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Choose your preset").
				Options(options...).
				Value(&preset),
		),
	)

	err := form.Run()
	return preset, err
}

func Confirm(question string, defaultVal bool) (bool, error) {
	var result bool = defaultVal

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(question).
				Affirmative("Yes").
				Negative("No").
				Value(&result),
		),
	)

	err := form.Run()
	return result, err
}

func SelectOption(title string, options []string) (string, error) {
	var selected string

	opts := make([]huh.Option[string], len(options))
	for i, o := range options {
		opts[i] = huh.NewOption(o, o)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(opts...).
				Value(&selected),
		),
	)

	err := form.Run()
	return selected, err
}

func Input(title, placeholder string) (string, error) {
	var value string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(title).
				Placeholder(placeholder).
				Value(&value),
		),
	)

	err := form.Run()
	return value, err
}
