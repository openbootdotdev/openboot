package ui

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/openbootdotdev/openboot/internal/snapshot"
	"github.com/openbootdotdev/openboot/internal/system"
)

var (
	scanCheckStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#22c55e"))

	scanErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ef4444"))

	scanActiveStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#06b6d4"))

	scanPendingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#666666"))

	scanCountStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888"))
)

var scanSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type stepState struct {
	name    string
	status  string
	count   int
	elapsed time.Duration
}

type ScanProgress struct {
	steps          []stepState
	totalSteps     int
	spinnerIdx     int
	spinnerStop    chan bool
	closeOnce      sync.Once
	mu             sync.Mutex
	isTTY          bool
	rendered       bool
	overallStart   time.Time
	stepStartTimes []time.Time
	completedCount int
}

func NewScanProgress(totalSteps int) *ScanProgress {
	steps := make([]stepState, totalSteps)
	for i := range steps {
		steps[i].status = "pending"
	}

	sp := &ScanProgress{
		steps:          steps,
		totalSteps:     totalSteps,
		spinnerStop:    make(chan bool),
		isTTY:          system.HasTTY(),
		overallStart:   time.Now(),
		stepStartTimes: make([]time.Time, totalSteps),
	}

	if sp.isTTY {
		go func() {
			ticker := time.NewTicker(80 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-sp.spinnerStop:
					return
				case <-ticker.C:
					sp.mu.Lock()
					sp.spinnerIdx = (sp.spinnerIdx + 1) % len(scanSpinnerFrames)
					hasActive := false
					for _, s := range sp.steps {
						if s.status == "scanning" {
							hasActive = true
							break
						}
					}
					if hasActive {
						sp.render()
					}
					sp.mu.Unlock()
				}
			}
		}()
	}

	return sp
}

func (sp *ScanProgress) Update(step snapshot.ScanStep) {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	if step.Index < 0 || step.Index >= sp.totalSteps {
		return
	}

	if step.Status == "scanning" && sp.steps[step.Index].status != "scanning" {
		sp.stepStartTimes[step.Index] = time.Now()
	}

	if (step.Status == "done" || step.Status == "error") && sp.steps[step.Index].status == "scanning" {
		sp.steps[step.Index].elapsed = time.Since(sp.stepStartTimes[step.Index])
		sp.completedCount++
	}

	sp.steps[step.Index].name = step.Name
	sp.steps[step.Index].status = step.Status
	sp.steps[step.Index].count = step.Count

	sp.render()
}

func (sp *ScanProgress) Finish() {
	sp.closeOnce.Do(func() { close(sp.spinnerStop) })

	sp.mu.Lock()
	defer sp.mu.Unlock()

	sp.render()
	fmt.Fprintf(os.Stderr, "\n")
}

func (sp *ScanProgress) render() {
	if sp.isTTY {
		sp.renderTTY()
	} else {
		sp.renderPlain()
	}
}

func (sp *ScanProgress) renderTTY() {
	if sp.rendered {
		fmt.Fprintf(os.Stderr, "\033[%dA", sp.totalSteps+1)
	}

	if !sp.rendered {
		sp.rendered = true
	}
	fmt.Fprintf(os.Stderr, "\033[K  Scanning your Mac... [%d/%d]\n", sp.completedCount, sp.totalSteps)

	for i, s := range sp.steps {
		fmt.Fprintf(os.Stderr, "\033[K")

		switch s.status {
		case "done":
			elapsed := sp.formatStepDuration(s.elapsed)
			countText := formatStepCount(s.count)
			fmt.Fprintf(os.Stderr, "  %s %s\n",
				scanCheckStyle.Render("✓ "+s.name),
				scanCountStyle.Render(fmt.Sprintf("%s, %s", countText, elapsed)))
		case "error":
			elapsed := sp.formatStepDuration(s.elapsed)
			fmt.Fprintf(os.Stderr, "  %s %s\n",
				scanErrorStyle.Render("✗ "+s.name),
				scanCountStyle.Render(fmt.Sprintf("failed, %s", elapsed)))
		case "scanning":
			spinner := scanSpinnerFrames[sp.spinnerIdx]
			live := time.Since(sp.stepStartTimes[i])
			liveStr := sp.formatStepDuration(live)
			fmt.Fprintf(os.Stderr, "  %s %s\n",
				scanActiveStyle.Render(spinner+" "+s.name),
				scanCountStyle.Render(fmt.Sprintf("%s...", liveStr)))
		default:
			name := s.name
			if name == "" {
				name = "..."
			}
			fmt.Fprintf(os.Stderr, "  %s\n",
				scanPendingStyle.Render("  "+name))
		}
	}
}

func (sp *ScanProgress) renderPlain() {
	for i, s := range sp.steps {
		switch s.status {
		case "done":
			if !sp.rendered {
				fmt.Fprintf(os.Stderr, "  Scanning your Mac...\n")
				sp.rendered = true
			}
			elapsed := sp.formatStepDuration(s.elapsed)
			countText := formatStepCount(s.count)
			fmt.Fprintf(os.Stderr, "  ✓ %s (%s, %s)\n", s.name, countText, elapsed)
			sp.steps[i].status = "done_printed"
		case "error":
			if !sp.rendered {
				fmt.Fprintf(os.Stderr, "  Scanning your Mac...\n")
				sp.rendered = true
			}
			elapsed := sp.formatStepDuration(s.elapsed)
			fmt.Fprintf(os.Stderr, "  ✗ %s (failed, %s)\n", s.name, elapsed)
			sp.steps[i].status = "error_printed"
		}
	}
}

func (sp *ScanProgress) formatStepDuration(d time.Duration) string {
	if d < time.Second {
		return "< 1s"
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func formatStepCount(count int) string {
	if count == 1 {
		return "1 found"
	}
	return fmt.Sprintf("%d found", count)
}
