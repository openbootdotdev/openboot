package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/openbootdotdev/openboot/internal/config"
)

var (
	tabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(lipgloss.Color("#666"))

	activeTabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(lipgloss.Color("#22c55e")).
			Bold(true).
			Underline(true)

	itemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fff"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#22c55e"))

	descStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#444")).
			MarginTop(1)

	countStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888"))

	badgeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666"))

	boldStyle = lipgloss.NewStyle().
			Bold(true)

	onlineHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#f59e0b")).
				Bold(true)

	onlineSearchingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888")).
				Italic(true)

	searchBarQueryStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#fff")).
				Bold(true)

	searchBarSepStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#444"))

	searchBarStatsStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888"))

	searchBarIconStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#f59e0b"))

	searchBarHintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#555")).
				Italic(true)
)

func (m SelectorModel) getVisibleItems() int {
	if m.height == 0 {
		return 15
	}
	available := m.height - 8
	if available < 5 {
		available = 5
	}
	if available > 20 {
		available = 20
	}
	return available
}

func (m SelectorModel) renderTabBar() string {
	totalTabs := len(m.categories)

	arrowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555"))
	neighborStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666"))
	sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#444"))
	posStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555"))

	cat := m.categories[m.activeTab]
	count := 0
	for _, pkg := range cat.Packages {
		if m.selected[pkg.Name] {
			count++
		}
	}
	activeRendered := activeTabStyle.Render(fmt.Sprintf("%s %s (%d)", cat.Icon, cat.Name, count))

	posRendered := posStyle.Render(fmt.Sprintf("  %d/%d", m.activeTab+1, totalTabs))

	hasLeft := m.activeTab > 0
	hasRight := m.activeTab < totalTabs-1
	leftArrow := "  "
	if hasLeft {
		leftArrow = arrowStyle.Render("‹ ")
	}
	rightArrow := "  "
	if hasRight {
		rightArrow = arrowStyle.Render(" ›")
	}

	termWidth := m.width
	if termWidth == 0 {
		termWidth = 80
	}

	baseWidth := lipgloss.Width(leftArrow) + lipgloss.Width(activeRendered) + lipgloss.Width(rightArrow) + lipgloss.Width(posRendered)
	remaining := termWidth - baseWidth

	sep := sepStyle.Render(" │ ")
	sepW := lipgloss.Width(sep)

	var leftNeighbors []string
	var rightNeighbors []string
	li := m.activeTab - 1
	ri := m.activeTab + 1

	for remaining > 0 && (li >= 0 || ri < totalTabs) {
		added := false
		if li >= 0 {
			rendered := neighborStyle.Render(m.categories[li].Name)
			w := lipgloss.Width(rendered) + sepW
			if w <= remaining {
				leftNeighbors = append([]string{rendered}, leftNeighbors...)
				remaining -= w
				li--
				added = true
			} else {
				li = -1
			}
		}
		if ri < totalTabs {
			rendered := neighborStyle.Render(m.categories[ri].Name)
			w := lipgloss.Width(rendered) + sepW
			if w <= remaining {
				rightNeighbors = append(rightNeighbors, rendered)
				remaining -= w
				ri++
				added = true
			} else {
				ri = totalTabs
			}
		}
		if !added {
			break
		}
	}

	var result strings.Builder
	result.WriteString(leftArrow)
	for _, n := range leftNeighbors {
		result.WriteString(n)
		result.WriteString(sep)
	}
	result.WriteString(activeRendered)
	for _, n := range rightNeighbors {
		result.WriteString(sep)
		result.WriteString(n)
	}
	result.WriteString(rightArrow)
	result.WriteString(posRendered)

	return result.String()
}

func getTypeBadge(pkg config.Package) string {
	if pkg.IsNpm {
		return badgeStyle.Render("📦 ")
	}
	if pkg.IsCask {
		return badgeStyle.Render("🖥 ")
	}
	return badgeStyle.Render("⚙ ")
}

func highlightMatches(text string, matchedIndexes []int) string {
	if len(matchedIndexes) == 0 {
		return text
	}

	var result strings.Builder
	matchSet := make(map[int]bool)
	for _, idx := range matchedIndexes {
		matchSet[idx] = true
	}

	for i, char := range text {
		if matchSet[i] {
			result.WriteString(boldStyle.Render(string(char)))
		} else {
			result.WriteRune(char)
		}
	}

	return result.String()
}

func truncateLine(line string, maxWidth int) string {
	if maxWidth <= 0 {
		return line
	}
	visualWidth := lipgloss.Width(line)
	if visualWidth <= maxWidth {
		return line
	}
	if maxWidth < 10 {
		return lipgloss.NewStyle().MaxWidth(maxWidth).Render(line)
	}
	return lipgloss.NewStyle().MaxWidth(maxWidth-3).Render(line) + "..."
}

// padLine pads a rendered line with spaces to the given width, using visual
// width so that ANSI escape codes do not affect the calculation. This clears
// any ghost text left by a previously longer line in the same terminal row.
func padLine(line string, width int) string {
	if width <= 0 {
		return line
	}
	visualWidth := lipgloss.Width(line)
	if visualWidth >= width {
		return line
	}
	return line + strings.Repeat(" ", width-visualWidth)
}

// padAllLines pads every line in a rendered view to the given terminal width.
// This is the root-cause fix for ghost text: instead of padding individual
// lines (easy to miss), call this once on the final View() output.
func padAllLines(s string, width int) string {
	if width <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = padLine(line, width)
	}
	return strings.Join(lines, "\n")
}

func (m SelectorModel) View() string {
	if m.showConfirmation {
		return m.confirmationView()
	}

	var lines []string

	if m.searchMode {
		return m.viewSearch()
	}

	lines = append(lines, m.renderTabBar())
	lines = append(lines, "")

	cat := m.categories[m.activeTab]
	visibleItems := m.getVisibleItems()

	if m.scrollOffset > len(cat.Packages)-visibleItems {
		m.scrollOffset = len(cat.Packages) - visibleItems
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	endIdx := m.scrollOffset + visibleItems
	if endIdx > len(cat.Packages) {
		endIdx = len(cat.Packages)
	}

	for i := m.scrollOffset; i < endIdx; i++ {
		pkg := cat.Packages[i]
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		checkbox := "[ ]"
		style := itemStyle
		if m.selected[pkg.Name] {
			checkbox = "[✓]"
			style = selectedStyle
		}

		line := fmt.Sprintf("%s%s %s %s", cursor, checkbox, style.Render(pkg.Name), descStyle.Render(pkg.Description))
		if m.width > 0 {
			line = padLine(truncateLine(line, m.width-2), m.width)
		}
		lines = append(lines, line)
	}

	clearWidth := m.width
	if clearWidth <= 0 {
		clearWidth = 80
	}
	clearLine := strings.Repeat(" ", clearWidth)
	for len(lines) < visibleItems+2 {
		lines = append(lines, clearLine)
	}

	totalSelected := 0
	for _, v := range m.selected {
		if v {
			totalSelected++
		}
	}

	lines = append(lines, "")
	if m.toastMessage != "" {
		toastStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")).Italic(true)
		if !m.toastIsAdd {
			toastStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888")).Italic(true)
		}
		lines = append(lines, toastStyle.Render(m.toastMessage))
	} else {
		lines = append(lines, countStyle.Render(fmt.Sprintf("Selected: %d packages", totalSelected)))
	}
	lines = append(lines, "")
	lines = append(lines, helpStyle.Render("Tab/←→: switch • ↑↓: navigate • Space: toggle • /: search • a: all • Enter: confirm • q: quit"))

	return padAllLines(strings.Join(lines, "\n"), m.width)
}

func (m SelectorModel) confirmationView() string {
	var formulae, casks, npm []string
	for name, selected := range m.selected {
		if !selected {
			continue
		}

		var pkg *config.Package
		for _, cat := range m.categories {
			for i := range cat.Packages {
				if cat.Packages[i].Name == name {
					pkg = &cat.Packages[i]
					break
				}
			}
			if pkg != nil {
				break
			}
		}

		if pkg == nil {
			if op, ok := m.selectedOnline[name]; ok {
				pkg = &op
			}
		}

		if pkg != nil {
			if pkg.IsNpm {
				npm = append(npm, pkg.Name)
			} else if pkg.IsCask {
				casks = append(casks, pkg.Name)
			} else {
				formulae = append(formulae, pkg.Name)
			}
		}
	}

	totalPackages := len(formulae) + len(casks) + len(npm)

	estimatedSeconds := len(formulae)*15 + len(casks)*30 + len(npm)*5
	estimatedMinutes := estimatedSeconds / 60

	boxWidth := 60
	if m.width > 0 && m.width < 70 {
		boxWidth = m.width - 10
		if boxWidth < 40 {
			boxWidth = 40
		}
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#22c55e")).
		Padding(1, 2).
		Width(boxWidth)

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#22c55e")).
		Bold(true)

	sectionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#fff")).
		Bold(true)

	listStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888"))

	instructionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666")).
		Italic(true)

	var content strings.Builder

	content.WriteString(headerStyle.Render("Install Summary"))
	content.WriteString("\n\n")
	fmt.Fprintf(&content, "Total: %d packages\n\n", totalPackages)

	if len(formulae) > 0 {
		content.WriteString(sectionStyle.Render(fmt.Sprintf("⚙  Formulae (%d)", len(formulae))))
		content.WriteString("\n")
		if len(formulae) <= 10 {
			content.WriteString(listStyle.Render("  " + strings.Join(formulae, ", ")))
		} else {
			content.WriteString(listStyle.Render("  " + strings.Join(formulae[:10], ", ")))
			content.WriteString(listStyle.Render(fmt.Sprintf(" and %d more...", len(formulae)-10)))
		}
		content.WriteString("\n\n")
	}

	if len(casks) > 0 {
		content.WriteString(sectionStyle.Render(fmt.Sprintf("🖥  Applications (%d)", len(casks))))
		content.WriteString("\n")
		if len(casks) <= 10 {
			content.WriteString(listStyle.Render("  " + strings.Join(casks, ", ")))
		} else {
			content.WriteString(listStyle.Render("  " + strings.Join(casks[:10], ", ")))
			content.WriteString(listStyle.Render(fmt.Sprintf(" and %d more...", len(casks)-10)))
		}
		content.WriteString("\n\n")
	}

	if len(npm) > 0 {
		content.WriteString(sectionStyle.Render(fmt.Sprintf("📦  NPM (%d)", len(npm))))
		content.WriteString("\n")
		if len(npm) <= 10 {
			content.WriteString(listStyle.Render("  " + strings.Join(npm, ", ")))
		} else {
			content.WriteString(listStyle.Render("  " + strings.Join(npm[:10], ", ")))
			content.WriteString(listStyle.Render(fmt.Sprintf(" and %d more...", len(npm)-10)))
		}
		content.WriteString("\n\n")
	}

	fmt.Fprintf(&content, "Estimated time: ~%d minutes\n\n", estimatedMinutes)
	content.WriteString(instructionStyle.Render("[Enter] Confirm & Install"))
	content.WriteString("\n")
	content.WriteString(instructionStyle.Render("[Esc] Go Back"))

	return padAllLines(boxStyle.Render(content.String()), m.width)
}

func (m SelectorModel) viewSearch() string {
	var lines []string

	query := m.searchQuery + "▌"
	searchBar := searchBarIconStyle.Render("🔍 ") + searchBarQueryStyle.Render(query)

	localCount := len(m.filteredPkgs)
	onlineCount := len(m.onlineResults)

	var statsText string
	if m.searchQuery == "" {
		statsText = searchBarHintStyle.Render("Type to search all categories and online...")
	} else if m.onlineSearching {
		spinner := searchSpinnerFrames[m.searchSpinnerIdx]
		statsText = searchBarStatsStyle.Render(fmt.Sprintf("%d local", localCount)) +
			searchBarSepStyle.Render(" · ") +
			scanActiveStyle.Render(spinner+" searching...")
	} else if onlineCount > 0 {
		statsText = searchBarStatsStyle.Render(fmt.Sprintf("%d local", localCount)) +
			searchBarSepStyle.Render(" · ") +
			searchBarStatsStyle.Render(fmt.Sprintf("%d online", onlineCount))
	} else if localCount > 0 {
		statsText = searchBarStatsStyle.Render(fmt.Sprintf("%d found", localCount))
	} else {
		statsText = searchBarStatsStyle.Render("no results")
	}

	searchBar += "  " + searchBarSepStyle.Render("│") + "  " + statsText

	lines = append(lines, searchBar)
	lines = append(lines, "")

	visibleItems := m.getVisibleItems()
	itemsRendered := 0

	if len(m.filteredPkgs) == 0 && len(m.onlineResults) == 0 && !m.onlineSearching {
		if m.searchQuery == "" {
			lines = append(lines, "")
			lines = append(lines, descStyle.Render("  Search across all categories and discover new packages"))
		} else {
			lines = append(lines, descStyle.Render("  No matching packages"))
		}
	} else {
		endIdx := visibleItems
		if endIdx > len(m.filteredPkgs) {
			endIdx = len(m.filteredPkgs)
		}

		for i := 0; i < endIdx; i++ {
			pkg := m.filteredPkgs[i]
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}

			checkbox := "[ ]"
			style := itemStyle
			if m.selected[pkg.Name] {
				checkbox = "[✓]"
				style = selectedStyle
			}

			badge := getTypeBadge(pkg)

			var displayName string
			if i < len(m.fuzzyMatches) {
				displayName = highlightMatches(pkg.Name, m.fuzzyMatches[i].MatchedIndexes)
			} else {
				displayName = pkg.Name
			}

			line := fmt.Sprintf("%s%s %s%s %s", cursor, checkbox, badge, style.Render(displayName), descStyle.Render(pkg.Description))
			if m.width > 0 {
				line = padLine(truncateLine(line, m.width-2), m.width)
			}
			lines = append(lines, line)
			itemsRendered++
		}

		if m.onlineSearching {
			lines = append(lines, "")
			lines = append(lines, descStyle.Render("  ── Loading online results ──"))
			itemsRendered += 2
		} else if len(m.onlineResults) > 0 {
			lines = append(lines, "")
			lines = append(lines, onlineHeaderStyle.Render("── Online Results ──"))
			itemsRendered += 2

			onlineVisibleLimit := visibleItems - itemsRendered
			if onlineVisibleLimit < 1 {
				onlineVisibleLimit = 1
			}
			onlineEnd := onlineVisibleLimit
			if onlineEnd > len(m.onlineResults) {
				onlineEnd = len(m.onlineResults)
			}

			offlineCount := len(m.filteredPkgs)
			for i := 0; i < onlineEnd; i++ {
				pkg := m.onlineResults[i]
				globalIdx := offlineCount + i
				cursor := "  "
				if globalIdx == m.cursor {
					cursor = "> "
				}

				checkbox := "[ ]"
				style := itemStyle
				if m.selected[pkg.Name] {
					checkbox = "[✓]"
					style = selectedStyle
				}

				badge := getTypeBadge(pkg)
				line := fmt.Sprintf("%s%s %s%s %s", cursor, checkbox, badge, style.Render(pkg.Name), descStyle.Render(pkg.Description))
				if m.width > 0 {
					line = padLine(truncateLine(line, m.width-2), m.width)
				}
				lines = append(lines, line)
				itemsRendered++
			}
		}
	}

	clearWidth := m.width
	if clearWidth <= 0 {
		clearWidth = 80
	}
	clearLine := strings.Repeat(" ", clearWidth)
	for len(lines) < visibleItems+2 {
		lines = append(lines, clearLine)
	}

	totalSelected := 0
	for _, v := range m.selected {
		if v {
			totalSelected++
		}
	}

	lines = append(lines, "")
	if m.toastMessage != "" {
		toastStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")).Italic(true)
		if !m.toastIsAdd {
			toastStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888")).Italic(true)
		}
		lines = append(lines, toastStyle.Render(m.toastMessage))
	} else {
		lines = append(lines, countStyle.Render(fmt.Sprintf("Selected: %d packages", totalSelected)))
	}
	lines = append(lines, "")
	lines = append(lines, helpStyle.Render("↑↓: navigate • Space: toggle • Esc: exit search • Enter: confirm"))

	return padAllLines(strings.Join(lines, "\n"), m.width)
}
