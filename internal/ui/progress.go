package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

const (
	minBarWidth     = 20
	defaultBarWidth = 40
	minPkgWidth     = 10
	defaultPkgWidth = 25
	statusWidth     = 16 // " 48/48 (100%)"
	etaWidth        = 8  // "~10m30s "
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

	etaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666"))
)

type ProgressTracker struct {
	total         int
	completed     int
	active        map[string]bool
	barWidth      int
	pkgWidth      int
	startTime     time.Time
	mu            sync.Mutex
	spinnerIdx    int
	spinnerStop   chan bool
	lastDisplayed string
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func getTerminalWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}

func NewProgressTracker(total int) *ProgressTracker {
	termWidth := getTerminalWidth()
	barWidth, pkgWidth := calculateWidths(termWidth)

	p := &ProgressTracker{
		total:       total,
		barWidth:    barWidth,
		pkgWidth:    pkgWidth,
		startTime:   time.Now(),
		active:      make(map[string]bool),
		spinnerStop: make(chan bool),
	}

	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-p.spinnerStop:
				return
			case <-ticker.C:
				p.mu.Lock()
				p.spinnerIdx = (p.spinnerIdx + 1) % len(spinnerFrames)
				if len(p.active) > 0 {
					p.render()
				}
				p.mu.Unlock()
			}
		}
	}()

	return p
}

func calculateWidths(termWidth int) (barWidth, pkgWidth int) {
	fixed := statusWidth + etaWidth
	available := termWidth - fixed - 2

	if available < minBarWidth+minPkgWidth {
		return minBarWidth, minPkgWidth
	}

	barWidth = available * 55 / 100
	pkgWidth = available - barWidth

	if barWidth > defaultBarWidth {
		barWidth = defaultBarWidth
		pkgWidth = available - barWidth
	}
	if pkgWidth > defaultPkgWidth+10 {
		pkgWidth = defaultPkgWidth + 10
	}

	return barWidth, pkgWidth
}

func (p *ProgressTracker) SetCurrent(pkgName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.active[pkgName] = true
	p.render()
}

func (p *ProgressTracker) Complete(pkgName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.active, pkgName)
	if p.lastDisplayed == pkgName {
		p.lastDisplayed = ""
	}
	p.completed++
	p.render()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func (p *ProgressTracker) render() {
	percent := float64(p.completed) / float64(p.total)
	filled := int(percent * float64(p.barWidth))
	empty := p.barWidth - filled

	bar := progressBarStyle.Render(strings.Repeat("█", filled)) +
		progressBgStyle.Render(strings.Repeat("░", empty))

	status := fmt.Sprintf(" %d/%d (%3.0f%%)", p.completed, p.total, percent*100)

	eta := p.estimateRemaining()
	if eta != "" {
		eta = fmt.Sprintf("%-6s", eta)
	}

	activeDisplay := ""
	activeCount := len(p.active)
	if activeCount > 0 {
		if p.lastDisplayed != "" && p.active[p.lastDisplayed] {
			activeDisplay = p.lastDisplayed
		} else {
			for pkg := range p.active {
				p.lastDisplayed = pkg
				activeDisplay = pkg
				break
			}
		}
		suffixLen := 0
		if activeCount > 1 {
			suffixLen = len(fmt.Sprintf(" +%d", activeCount-1))
		}
		activeDisplay = truncate(activeDisplay, p.pkgWidth-suffixLen)
		if activeCount > 1 {
			activeDisplay = fmt.Sprintf("%s +%d", activeDisplay, activeCount-1)
		}
	}

	fmt.Printf("\r\033[K%s%s %s %s",
		bar,
		progressTextStyle.Render(status),
		etaStyle.Render(eta),
		currentPkgStyle.Render(activeDisplay))
}

func (p *ProgressTracker) estimateRemaining() string {
	if p.completed == 0 {
		return ""
	}

	elapsed := time.Since(p.startTime)
	avgPerPkg := elapsed / time.Duration(p.completed)
	remaining := p.total - p.completed
	eta := avgPerPkg * time.Duration(remaining)

	if eta < time.Second {
		return "< 1s"
	}

	if eta < time.Minute {
		return fmt.Sprintf("~%ds", int(eta.Seconds()))
	}

	mins := int(eta.Minutes())
	secs := int(eta.Seconds()) % 60
	if secs > 0 {
		return fmt.Sprintf("~%dm%ds", mins, secs)
	}
	return fmt.Sprintf("~%dm", mins)
}

func (p *ProgressTracker) Finish() {
	close(p.spinnerStop)

	p.mu.Lock()
	defer p.mu.Unlock()

	elapsed := time.Since(p.startTime)
	fmt.Printf("\n  Completed in %s\n", formatDuration(elapsed))
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", mins, secs)
}

func (p *ProgressTracker) GetProgress() (int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.completed, p.total
}
