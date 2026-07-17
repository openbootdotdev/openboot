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
	aborting   bool // first ctrl+c seen; a second one force-quits
	succeeded  int
	failed     int
	skipped    int
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
				if sp.onInterrupt() {
					os.Exit(130)
				}
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

// render draws the live status line in place: carriage-return to the start of
// the current line, clear it, then write the spinner, progress count, current
// package, and elapsed time. It writes no newline, so PrintLine's completed
// rows scroll above it and this line trails the output.
//
// This deliberately does NOT reserve a "sticky" bottom bar via a terminal
// scroll region (DECSTBM). That looked nicer where it worked, but it silently
// corrupted on terminals that report a normal TERM yet don't honour scroll
// regions — the reserved bar got dragged into the log and reprinted on every
// tick, so the install read as garbage. A plain in-place status line renders
// top-to-bottom on every terminal, which is the property that matters for a
// one-shot install log. Don't reintroduce the scroll region.
func (sp *StickyProgress) render() {
	spinner := spinnerFrames[sp.spinnerIdx]
	pkg := truncate(sp.currentPkg, 20)
	elapsed := sp.pkgElapsed()

	fmt.Printf("\r\033[K%s %s %s   %s",
		spinner,
		progressTextStyle.Render(fmt.Sprintf("[%d/%d]", sp.completed+1, sp.total)),
		currentPkgStyle.Render(pkg),
		etaStyle.Render(elapsed))
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

// onInterrupt handles ctrl+c while the bar is running, and reports whether the
// caller should force-quit.
//
// The first interrupt only tears down the sticky rendering and restores the
// terminal. It deliberately does NOT exit: the same signal cancels the install
// context, which kills the running brew/npm subprocess and makes ApplyContext
// bail before any further step mutates the system — an abort that unwinds and
// reports what it managed to do. Exiting here instead (the old behaviour) cut
// that short from a goroutine, skipping every deferred cleanup on the way out.
//
// A second interrupt reports true, so a subprocess that ignores the
// cancellation can still be escaped. That escape matters: signal.NotifyContext
// has already fired by then and swallows further SIGINTs, so without this the
// terminal would look dead.
func (sp *StickyProgress) onInterrupt() (force bool) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if sp.aborting {
		return true
	}
	sp.aborting = true
	sp.active = false
	fmt.Printf("\r\033[K")
	fmt.Println(mutedStyle.Render("  aborting — waiting for the current step to stop · ctrl+c again to force quit"))
	return false
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

func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", mins, secs)
}
