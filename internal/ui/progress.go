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

// Phase tracks whether the progress bar is in formula or cask mode.
type Phase int

const (
	PhaseFormula Phase = iota
	PhaseCask
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

	// Phase tracks whether we're installing formulae or casks. Cask phase
	// switches to byte-based ETA when bytes are available.
	phase Phase

	// Cask-only: real-time download tracking.
	currentBytes int64
	totalBytes   int64
	speed        float64 // bytes/sec, EMA
	lastBytes    int64
	lastTime     time.Time

	// Scroll region rendering (nil when terminal doesn't support it).
	region *ScrollRegion
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func getTerminalWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 { //nolint:gosec // os.Stdout.Fd() returns a valid file descriptor; uintptr fits in int on all supported platforms
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
		sp.region.DrawTop(sp.formatLines())
		return
	}
	sp.renderInline()
}

// renderInline is the existing inline-line rendering used when scroll region
// is unavailable.
func (sp *StickyProgress) renderInline() {
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

// formatLines returns the two strings to render in the scroll region:
// row 1 = data + bar, row 2 = divider line.
func (sp *StickyProgress) formatLines() []string {
	var head string
	switch sp.phase {
	case PhaseCask:
		head = sp.formatCaskHead()
	default:
		head = sp.formatFormulaHead()
	}

	cols := 80
	if sp.region != nil {
		cols = sp.region.Cols()
	}

	// barWidth is whatever's left after head + " " + bar + " 100%" suffix.
	// Clamp [minBarWidth, defaultBarWidth] so the bar stays readable on
	// narrow terminals and doesn't dominate wide ones.
	barWidth := cols - len(head) - 6
	if barWidth < minBarWidth {
		barWidth = minBarWidth
	}
	if barWidth > defaultBarWidth {
		barWidth = defaultBarWidth
	}

	pct := float64(0)
	if sp.total > 0 {
		pct = float64(sp.completed) / float64(sp.total)
	}
	filled := int(pct * float64(barWidth))
	empty := barWidth - filled
	bar := progressBarStyle.Render(strings.Repeat("█", filled)) +
		progressBgStyle.Render(strings.Repeat("░", empty))

	line1 := fmt.Sprintf("%s %s %3d%%", head, bar, int(pct*100))
	line2 := strings.Repeat("─", cols)
	return []string{line1, line2}
}

func (sp *StickyProgress) formatFormulaHead() string {
	pkg := truncate(sp.currentPkg, 24)
	return fmt.Sprintf("[%d/%d] %-24s", sp.completed, sp.total, pkg)
}

func (sp *StickyProgress) formatCaskHead() string {
	pkg := truncate(sp.currentPkg, 18)
	bytes := "—"
	speed := "—"
	if sp.totalBytes > 0 {
		bytes = fmt.Sprintf("%s/%s", humanBytes(sp.currentBytes), humanBytes(sp.totalBytes))
	}
	if sp.speed > 0 {
		speed = fmt.Sprintf("%s/s", humanBytes(int64(sp.speed)))
	}
	eta := sp.estimateCurrentCaskETA()
	if eta == "" {
		eta = "—"
	}
	return fmt.Sprintf("[%d/%d] %-18s %s · %s · %s", sp.completed, sp.total, pkg, bytes, speed, eta)
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%dM", n/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%dK", n/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// SetPhase switches between formula and cask data displays. Per-cask byte
// state is cleared on each transition.
func (sp *StickyProgress) SetPhase(p Phase) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.phase = p
	sp.currentBytes = 0
	sp.totalBytes = 0
	sp.speed = 0
	sp.lastBytes = 0
	sp.lastTime = time.Time{}
}

// SetCurrentBytes records progress for the currently-installing cask. The
// total comes from a pre-flight HEAD on the cask URL; current comes from
// polling the brew cache directory. Updates the EMA speed estimate.
func (sp *StickyProgress) SetCurrentBytes(current, total int64) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.observeBytesAt(current, time.Now())
	sp.totalBytes = total
}

// observeBytesAt is the testable inner loop of SetCurrentBytes (lets tests
// inject a clock).
func (sp *StickyProgress) observeBytesAt(current int64, now time.Time) {
	if !sp.lastTime.IsZero() && current > sp.lastBytes {
		dt := now.Sub(sp.lastTime).Seconds()
		if dt > 0 {
			instant := float64(current-sp.lastBytes) / dt
			if sp.speed == 0 {
				sp.speed = instant
			} else {
				// EMA with alpha=0.3 (favors recent samples, smooths jitter).
				sp.speed = 0.3*instant + 0.7*sp.speed
			}
		}
	}
	sp.currentBytes = current
	sp.lastBytes = current
	sp.lastTime = now
}

func (sp *StickyProgress) estimateCurrentCaskETA() string {
	if sp.speed <= 0 || sp.totalBytes <= 0 || sp.currentBytes >= sp.totalBytes {
		if sp.totalBytes > 0 && sp.currentBytes < sp.totalBytes {
			return "estimating..."
		}
		return ""
	}
	remaining := sp.totalBytes - sp.currentBytes
	secs := float64(remaining) / sp.speed
	if secs < 60 {
		return fmt.Sprintf("~%ds", int(secs))
	}
	mins := int(secs) / 60
	rem := int(secs) % 60
	if rem == 0 {
		return fmt.Sprintf("~%dm", mins)
	}
	return fmt.Sprintf("~%dm%ds", mins, rem)
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

// Deprecated: scroll region keeps the bar visible during interactive
// subprocess output, so callers no longer need to pause. Will be removed
// once brew_install.go no longer calls these stubs.
func (sp *StickyProgress) PauseForInteractive() {}

// Deprecated: see PauseForInteractive.
func (sp *StickyProgress) ResumeAfterInteractive() {}

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
