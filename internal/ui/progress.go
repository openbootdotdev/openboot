package ui

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

const (
	minBarWidth     = 20
	defaultBarWidth = 40
	statusWidth     = 16
	etaWidth        = 8
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

type StickyProgress struct {
	total      int
	completed  int
	currentPkg string
	barWidth   int
	pkgWidth   int
	startTime  time.Time
	mu         sync.Mutex
	spinnerIdx int
	stopCh     chan struct{}
	closeOnce  sync.Once
	sigCh      chan os.Signal
	active     bool
	succeeded  int
	failed     int
	skipped    int
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func getTerminalWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}

func NewStickyProgress(total int) *StickyProgress {
	termWidth := getTerminalWidth()
	barWidth := termWidth - statusWidth - etaWidth - 4
	if barWidth < minBarWidth {
		barWidth = minBarWidth
	}
	if barWidth > defaultBarWidth {
		barWidth = defaultBarWidth
	}
	pkgWidth := termWidth - barWidth - statusWidth - etaWidth - 4
	if pkgWidth < 10 {
		pkgWidth = 10
	}

	return &StickyProgress{
		total:    total,
		barWidth: barWidth,
		pkgWidth: pkgWidth,
		stopCh:   make(chan struct{}),
		sigCh:    make(chan os.Signal, 1),
	}
}

func (sp *StickyProgress) Start() {
	sp.mu.Lock()
	sp.active = true
	sp.startTime = time.Now()
	sp.mu.Unlock()

	signal.Stop(sp.sigCh)
	signal.Notify(sp.sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-sp.stopCh:
				return
			case <-sp.sigCh:
				sp.Finish()
				os.Exit(130)
			case <-ticker.C:
				sp.mu.Lock()
				sp.spinnerIdx = (sp.spinnerIdx + 1) % len(spinnerFrames)
				if sp.active {
					sp.render()
				}
				sp.mu.Unlock()
			}
		}
	}()
}

func (sp *StickyProgress) render() {
	pct := float64(0)
	if sp.total > 0 {
		pct = float64(sp.completed) / float64(sp.total)
	}
	filled := int(pct * float64(sp.barWidth))
	empty := sp.barWidth - filled

	bar := progressBarStyle.Render(strings.Repeat("█", filled)) +
		progressBgStyle.Render(strings.Repeat("░", empty))

	status := fmt.Sprintf(" %d/%d (%3.0f%%)", sp.completed, sp.total, pct*100)
	eta := sp.estimateRemaining()
	if eta != "" {
		eta = fmt.Sprintf("%-6s", eta)
	}

	pkgDisplay := ""
	if sp.currentPkg != "" {
		pkgDisplay = truncate(sp.currentPkg, sp.pkgWidth)
	}

	fmt.Printf("\r\033[K%s%s %s %s",
		bar,
		progressTextStyle.Render(status),
		etaStyle.Render(eta),
		currentPkgStyle.Render(pkgDisplay))
}

func (sp *StickyProgress) SetCurrent(pkgName string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.currentPkg = pkgName
	if sp.active {
		sp.render()
	}
}

func (sp *StickyProgress) Increment() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.completed++
	sp.succeeded++
	if sp.active {
		sp.render()
	}
}

func (sp *StickyProgress) SetSkipped(count int) {
	sp.mu.Lock()
	sp.skipped = count
	sp.mu.Unlock()
}

func (sp *StickyProgress) IncrementWithStatus(success bool) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.completed++
	if success {
		sp.succeeded++
	} else {
		sp.failed++
	}
	if sp.active {
		sp.render()
	}
}

func (sp *StickyProgress) PrintLine(format string, args ...interface{}) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	fmt.Printf("\r\033[K")
	fmt.Printf(format, args...)
	fmt.Println()
	if sp.active {
		sp.render()
	}
}

func (sp *StickyProgress) PauseForInteractive() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.active = false
	fmt.Printf("\r\033[K")
}

func (sp *StickyProgress) ResumeAfterInteractive() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.active = true
	sp.render()
}

func (sp *StickyProgress) Finish() {
	signal.Stop(sp.sigCh)
	sp.closeOnce.Do(func() { close(sp.stopCh) })
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.active = false
	fmt.Printf("\r\033[K")

	elapsed := time.Since(sp.startTime)

	var parts []string
	if sp.succeeded > 0 {
		parts = append(parts, successStyle.Render(fmt.Sprintf("✔ %d installed", sp.succeeded)))
	}
	if sp.skipped > 0 {
		parts = append(parts, mutedStyle.Render(fmt.Sprintf("○ %d skipped", sp.skipped)))
	}
	if sp.failed > 0 {
		parts = append(parts, errorStyle.Render(fmt.Sprintf("✗ %d failed", sp.failed)))
	}

	if len(parts) > 0 {
		fmt.Printf("  %s  %s\n", strings.Join(parts, "  "), etaStyle.Render(fmt.Sprintf("(%s)", FormatDuration(elapsed))))
	} else {
		fmt.Printf("  Completed in %s\n", FormatDuration(elapsed))
	}
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

func (sp *StickyProgress) estimateRemaining() string {
	if sp.completed == 0 {
		return ""
	}

	elapsed := time.Since(sp.startTime)
	avgPerPkg := elapsed / time.Duration(sp.completed)
	remaining := sp.total - sp.completed
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

func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", mins, secs)
}
