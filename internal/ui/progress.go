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
)

var (
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
	pkgStart   time.Time
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

	// Scroll region rendering (nil when terminal doesn't support it).
	region *ScrollRegion
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func NewStickyProgress(total int) *StickyProgress {
	return &StickyProgress{
		total:  total,
		stopCh: make(chan struct{}),
		sigCh:  make(chan os.Signal, 1),
	}
}

func (sp *StickyProgress) Start() {
	sp.mu.Lock()
	sp.active = true
	sp.startTime = time.Now()
	if IsScrollRegionSupported() {
		sp.region = NewScrollRegion(2)
		sp.region.Start()
	}
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
	if sp.region != nil {
		sp.region.DrawBottom(sp.formatLines())
		return
	}
	sp.renderInline()
}

// renderInline is the fallback renderer used when scroll region is unavailable.
func (sp *StickyProgress) renderInline() {
	spinner := spinnerFrames[sp.spinnerIdx]
	pkg := truncate(sp.currentPkg, 20)
	elapsed := sp.pkgElapsed()

	fmt.Printf("\r\033[K%s %s %s   %s",
		spinner,
		progressTextStyle.Render(fmt.Sprintf("[%d/%d]", sp.completed+1, sp.total)),
		currentPkgStyle.Render(pkg),
		etaStyle.Render(elapsed))
}

// formatLines returns the two strings to render in the bottom-reserved scroll
// region: a divider and a status line.
func (sp *StickyProgress) formatLines() []string {
	cols := 80
	if sp.region != nil {
		cols = sp.region.Cols()
	}

	spinner := spinnerFrames[sp.spinnerIdx]
	pkg := truncate(sp.currentPkg, 20)
	elapsed := sp.pkgElapsed()

	divider := strings.Repeat("─", cols)
	status := fmt.Sprintf("%s %s %s   %s",
		spinner,
		progressTextStyle.Render(fmt.Sprintf("[%d/%d]", sp.completed+1, sp.total)),
		currentPkgStyle.Render(pkg),
		etaStyle.Render(elapsed))
	return []string{divider, status}
}

// pkgElapsed returns elapsed seconds for the current package, or "" if not yet started.
func (sp *StickyProgress) pkgElapsed() string {
	if sp.pkgStart.IsZero() {
		return ""
	}
	secs := int(time.Since(sp.pkgStart).Seconds())
	if secs == 0 {
		return ""
	}
	return fmt.Sprintf("%ds", secs)
}

func (sp *StickyProgress) SetCurrent(pkgName string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.currentPkg = pkgName
	sp.pkgStart = time.Now()
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

func (sp *StickyProgress) Finish() {
	signal.Stop(sp.sigCh)
	sp.closeOnce.Do(func() { close(sp.stopCh) })
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.active = false
	if sp.region != nil {
		sp.region.Stop()
		sp.region = nil
	} else {
		fmt.Printf("\r\033[K")
	}

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

func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", mins, secs)
}
