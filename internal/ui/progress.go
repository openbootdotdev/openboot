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
	footerLines     = 2
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

	footerDivStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#333"))
)

type StickyProgress struct {
	total      int
	completed  int
	currentPkg string
	barWidth   int
	termWidth  int
	termHeight int
	startTime  time.Time
	mu         sync.Mutex
	spinnerIdx int
	stopCh     chan struct{}
	sigCh      chan os.Signal
	active     bool
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func getTerminalSize() (int, int) {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 || h <= 0 {
		return 80, 24
	}
	return w, h
}

func NewStickyProgress(total int) *StickyProgress {
	w, h := getTerminalSize()
	barWidth := w - statusWidth - etaWidth - 4
	if barWidth < minBarWidth {
		barWidth = minBarWidth
	}
	if barWidth > defaultBarWidth {
		barWidth = defaultBarWidth
	}

	return &StickyProgress{
		total:      total,
		termWidth:  w,
		termHeight: h,
		barWidth:   barWidth,
		startTime:  time.Now(),
		stopCh:     make(chan struct{}),
	}
}

func (sp *StickyProgress) Start() {
	sp.mu.Lock()
	sp.active = true
	sp.mu.Unlock()

	sp.sigCh = make(chan os.Signal, 1)
	signal.Notify(sp.sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-sp.sigCh:
			sp.cleanup()
			os.Exit(1)
		case <-sp.stopCh:
			return
		}
	}()

	fmt.Print("\033[?25l")
	for i := 0; i < footerLines; i++ {
		fmt.Println()
	}
	fmt.Printf("\033[1;%dr", sp.termHeight-footerLines)
	fmt.Printf("\033[%dA", footerLines)

	sp.renderFooter()

	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-sp.stopCh:
				return
			case <-ticker.C:
				sp.mu.Lock()
				sp.spinnerIdx = (sp.spinnerIdx + 1) % len(spinnerFrames)
				if sp.active {
					sp.renderFooter()
				}
				sp.mu.Unlock()
			}
		}
	}()
}

func (sp *StickyProgress) renderFooter() {
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

	spin := ""
	if sp.completed < sp.total {
		spin = spinnerFrames[sp.spinnerIdx] + " "
	}

	divider := footerDivStyle.Render(strings.Repeat("─", sp.termWidth))

	pkgDisplay := ""
	if sp.currentPkg != "" {
		maxPkgWidth := sp.termWidth - 16
		if maxPkgWidth > 0 {
			pkgDisplay = truncate(sp.currentPkg, maxPkgWidth)
		}
	}

	statusLine := fmt.Sprintf(" %s%s%s %s %s",
		spin,
		bar,
		progressTextStyle.Render(status),
		etaStyle.Render(eta),
		currentPkgStyle.Render(pkgDisplay))

	fmt.Print("\033[s")
	fmt.Printf("\033[%d;1H\033[K%s", sp.termHeight-1, divider)
	fmt.Printf("\033[%d;1H\033[K%s", sp.termHeight, statusLine)
	fmt.Print("\033[u")
}

func (sp *StickyProgress) SetCurrent(pkgName string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.currentPkg = pkgName
	if sp.active {
		sp.renderFooter()
	}
}

func (sp *StickyProgress) Increment() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.completed++
	if sp.active {
		sp.renderFooter()
	}
}

func (sp *StickyProgress) PauseForInteractive() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.active = false
	fmt.Print("\033[r")
	for i := sp.termHeight - footerLines + 1; i <= sp.termHeight; i++ {
		fmt.Printf("\033[%d;1H\033[K", i)
	}
	fmt.Printf("\033[%d;1H", sp.termHeight-footerLines)
	fmt.Print("\033[?25h")
}

func (sp *StickyProgress) ResumeAfterInteractive() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	w, h := getTerminalSize()
	sp.termWidth, sp.termHeight = w, h
	sp.barWidth = w - statusWidth - etaWidth - 4
	if sp.barWidth < minBarWidth {
		sp.barWidth = minBarWidth
	}
	if sp.barWidth > defaultBarWidth {
		sp.barWidth = defaultBarWidth
	}
	sp.active = true
	fmt.Print("\033[?25l")
	fmt.Printf("\033[1;%dr", sp.termHeight-footerLines)
	sp.renderFooter()
}

func (sp *StickyProgress) Finish() {
	close(sp.stopCh)
	sp.cleanup()

	elapsed := time.Since(sp.startTime)
	fmt.Printf("\n  Completed in %s\n", formatDuration(elapsed))
}

func (sp *StickyProgress) cleanup() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.active = false
	fmt.Print("\033[r")
	for i := sp.termHeight - footerLines + 1; i <= sp.termHeight; i++ {
		fmt.Printf("\033[%d;1H\033[K", i)
	}
	fmt.Printf("\033[%d;1H", sp.termHeight-footerLines)
	fmt.Print("\033[?25h")
	signal.Stop(sp.sigCh)
}

type ScrollWriter struct {
	progress *StickyProgress
	buf      []byte
}

func NewScrollWriter(progress *StickyProgress) *ScrollWriter {
	return &ScrollWriter{progress: progress}
}

func (sw *ScrollWriter) Write(p []byte) (n int, err error) {
	sw.progress.mu.Lock()
	defer sw.progress.mu.Unlock()

	sw.buf = append(sw.buf, p...)
	for {
		idx := -1
		for i, b := range sw.buf {
			if b == '\n' {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		line := string(sw.buf[:idx])
		sw.buf = sw.buf[idx+1:]
		fmt.Print("\033[s")
		fmt.Print(line)
		fmt.Print("\n")
		fmt.Print("\033[u")
	}

	if sw.progress.active {
		sw.progress.renderFooter()
	}
	return len(p), nil
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

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", mins, secs)
}
