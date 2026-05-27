package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/openbootdotdev/openboot/internal/config"
	"github.com/openbootdotdev/openboot/internal/system"
)

var (
	accent  = lipgloss.Color("#22c55e")
	subtle  = lipgloss.Color("#666666")
	warning = lipgloss.Color("#eab308")
	danger  = lipgloss.Color("#ef4444")
	info    = lipgloss.Color("#06b6d4")

	titleStyle = lipgloss.NewStyle().
			Foreground(accent).
			Bold(true).
			MarginBottom(1)

	successStyle = lipgloss.NewStyle().
			Foreground(accent)

	errorStyle = lipgloss.NewStyle().
			Foreground(danger)

	mutedStyle = lipgloss.NewStyle().
			Foreground(subtle)

	greenStyle  = lipgloss.NewStyle().Foreground(accent)
	yellowStyle = lipgloss.NewStyle().Foreground(warning)
	redStyle    = lipgloss.NewStyle().Foreground(danger)
	cyanStyle   = lipgloss.NewStyle().Foreground(info)
)

func Green(text string) string {
	return greenStyle.Render(text)
}

func Yellow(text string) string {
	return yellowStyle.Render(text)
}

func Red(text string) string {
	return redStyle.Render(text)
}

func Cyan(text string) string {
	return cyanStyle.Render(text)
}

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

func Warn(text string) {
	fmt.Println(yellowStyle.Render("⚠ " + text))
}

// Println prints plain text followed by a newline to stdout.
// Use for undecorated output lines (e.g. blank lines between sections).
func Println(a ...any) {
	fmt.Println(a...)
}

// Printf prints formatted plain text to stdout.
// Use for structured output that doesn't fit a semantic helper (Info/Warn/etc.).
func Printf(format string, a ...any) {
	fmt.Printf(format, a...)
}

// DryRunMsg prints a single [DRY-RUN] message using the Muted style.
func DryRunMsg(format string, args ...any) {
	Muted(fmt.Sprintf("[DRY-RUN] "+format, args...))
}

// DryRunList prints a "Would {action}:" header via Info, then each item
// formatted with cmdFmt via Muted. Use for dry-run guards that enumerate
// shell commands (e.g. "brew install %s").
func DryRunList(action, cmdFmt string, items []string) {
	Info("Would " + action + ":")
	for _, item := range items {
		Muted(fmt.Sprintf("    "+cmdFmt, item))
	}
}

// PrintScriptPreview displays a shell script in a readable box with line numbers.
// Long lines are truncated with "…" to keep the box at a reasonable width.
func PrintScriptPreview(script string) {
	lines := strings.Split(script, "\n")
	numWidth := len(fmt.Sprintf("%d", len(lines)))

	// Max content width (excluding border/padding): line number + separator + code
	const maxLineWidth = 76
	codeWidth := maxLineWidth - numWidth - 2 // 2 for "  " separator

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(subtle).
		PaddingLeft(1).
		PaddingRight(1)

	commentStyle := lipgloss.NewStyle().Foreground(accent).Bold(true)
	lineNumStyle := lipgloss.NewStyle().Foreground(subtle)

	var sb strings.Builder
	for i, line := range lines {
		num := lineNumStyle.Render(fmt.Sprintf("%*d", numWidth, i+1))

		display := line
		if len(display) > codeWidth {
			display = display[:codeWidth-1] + "…"
		}

		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			fmt.Fprintf(&sb, "%s  %s", num, commentStyle.Render(display))
		} else {
			fmt.Fprintf(&sb, "%s  %s", num, display)
		}
		if i < len(lines)-1 {
			sb.WriteString("\n")
		}
	}

	fmt.Println(boxStyle.Render(sb.String()))
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
	options = append(options, huh.NewOption("scratch - Start from scratch (select individual packages)", "scratch"))

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
	result := defaultVal

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
	return InputWithDefault(title, placeholder, "")
}

func InputWithDefault(title, placeholder, defaultValue string) (string, error) {
	value := defaultValue

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
