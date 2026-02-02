package ui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

var (
	progressBarStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#22c55e"))

	progressBgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#333"))

	progressTextStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888"))

	currentPkgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#60a5fa"))
)

type ProgressTracker struct {
	total      int
	completed  int
	currentPkg string
	width      int
	mu         sync.Mutex
}

func NewProgressTracker(total int) *ProgressTracker {
	return &ProgressTracker{
		total: total,
		width: 40,
	}
}

func (p *ProgressTracker) SetCurrent(pkgName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.currentPkg = pkgName
	p.render()
}

func (p *ProgressTracker) Complete(pkgName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.completed++
	p.render()
}

func (p *ProgressTracker) render() {
	percent := float64(p.completed) / float64(p.total)
	filled := int(percent * float64(p.width))
	empty := p.width - filled

	bar := progressBarStyle.Render(strings.Repeat("█", filled)) +
		progressBgStyle.Render(strings.Repeat("░", empty))

	status := fmt.Sprintf(" %d/%d (%.0f%%)", p.completed, p.total, percent*100)

	pkgDisplay := p.currentPkg
	if len(pkgDisplay) > 20 {
		pkgDisplay = pkgDisplay[:17] + "..."
	}

	fmt.Printf("\r\033[K%s%s %s",
		bar,
		progressTextStyle.Render(status),
		currentPkgStyle.Render(pkgDisplay))
}

func (p *ProgressTracker) Finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Println()
}

func (p *ProgressTracker) GetProgress() (int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.completed, p.total
}
