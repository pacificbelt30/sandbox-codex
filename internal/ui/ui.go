package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/rivo/tview"
)

const pollInterval = 2 * time.Second

// Run starts the dock-ui TUI.
func Run() error {
	app := tview.NewApplication()

	mgr, err := sandbox.NewManager(sandbox.ManagerConfig{})
	if err != nil {
		return fmt.Errorf("connecting to Docker: %w", err)
	}

	// Header
	header := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)

	// Container list table
	table := tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false).
		SetFixed(1, 0)

	setupTableHeader(table)

	// Footer / help
	footer := tview.NewTextView().
		SetDynamicColors(true).
		SetText("[yellow][↑↓][white] Select  [yellow][Enter][white] Logs  [yellow][S][white] Stop  [yellow][D][white] Delete  [yellow][R][white] Restart  [yellow][A][white] Stop All  [yellow][Q][white] Quit")

	// Log view panel
	logView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)
	logView.SetTitle(" Logs ").SetBorder(true)

	// Layout
	mainFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 1, 0, false).
		AddItem(table, 0, 1, true).
		AddItem(footer, 1, 0, false)

	pages := tview.NewPages().
		AddPage("main", mainFlex, true, true).
		AddPage("logs", tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(logView, 0, 1, true).
			AddItem(tview.NewTextView().SetText("[Q] Back"), 1, 0, false),
			true, false)

	var workers []sandbox.Worker

	refreshData := func() {
		ws, err := mgr.List(true)
		if err == nil {
			workers = ws
		}
		app.QueueUpdateDraw(func() {
			// Update header
			running := 0
			for _, w := range workers {
				if w.Status == "running" {
					running++
				}
			}
			header.SetText(fmt.Sprintf("[green]codex-dock[white]  [%d workers running / %d total]", running, len(workers)))

			// Update table
			for i := table.GetRowCount() - 1; i > 0; i-- {
				table.RemoveRow(i)
			}
			for i, w := range workers {
				uptime := "-"
				if w.StartedAt != nil {
					d := time.Since(*w.StartedAt)
					uptime = formatDuration(d)
				}
				branch := w.Branch
				if branch == "" {
					branch = "-"
				}
				task := w.Task
				if task == "" {
					task = "(interactive)"
				}
				table.SetCell(i+1, 0, tview.NewTableCell(w.Name).SetExpansion(1))
				table.SetCell(i+1, 1, tview.NewTableCell(branch).SetExpansion(1))
				table.SetCell(i+1, 2, tview.NewTableCell(statusIcon(w.Status)+w.Status).SetExpansion(1))
				table.SetCell(i+1, 3, tview.NewTableCell(uptime).SetExpansion(0))
				table.SetCell(i+1, 4, tview.NewTableCell(truncate(task, 30)).SetExpansion(2))
			}
		})
	}

	// showLogs fetches real container logs and displays them in the log view (F-UI-03).
	showLogs := func(w sandbox.Worker) {
		logView.Clear()
		// Switch to log page immediately so the user sees activity.
		app.QueueUpdateDraw(func() {
			pages.SwitchToPage("logs")
		})
		go func() {
			lw := &tviewWriter{view: logView, app: app}
			opts := sandbox.LogOptions{
				Name:   w.ID,
				Tail:   200,
				Output: lw,
			}
			if err := mgr.Logs(opts); err != nil {
				_, _ = fmt.Fprintf(lw, "[red]Error fetching logs: %v[-]", err)
			}
		}()
	}

	// Key handling
	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		row, _ := table.GetSelection()
		idx := row - 1 // header offset

		switch event.Rune() {
		case 'q', 'Q':
			app.Stop()

		case 's', 'S':
			if idx >= 0 && idx < len(workers) {
				go func(w sandbox.Worker) {
					_ = mgr.Stop(w.ID, 10)
					refreshData()
				}(workers[idx])
			}

		case 'd', 'D':
			if idx >= 0 && idx < len(workers) {
				go func(w sandbox.Worker) {
					_ = mgr.Remove(w.ID, true)
					refreshData()
				}(workers[idx])
			}

		case 'r', 'R':
			// F-UI-02: Restart a stopped/exited container via Docker start.
			if idx >= 0 && idx < len(workers) {
				go func(w sandbox.Worker) {
					if err := mgr.Start(w.ID); err != nil {
						// Silently ignore; container may already be running.
						_ = err
					}
					refreshData()
				}(workers[idx])
			}

		case 'a', 'A':
			go func() {
				for _, w := range workers {
					if w.Status == "running" {
						_ = mgr.Stop(w.ID, 10)
					}
				}
				refreshData()
			}()
		}

		switch event.Key() {
		case tcell.KeyEnter:
			if idx >= 0 && idx < len(workers) {
				showLogs(workers[idx])
			}
		}

		return event
	})

	// Log page key handling
	logView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'q' || event.Rune() == 'Q' {
			pages.SwitchToPage("main")
		}
		return event
	})

	// Polling ticker
	go func() {
		refreshData()
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for range ticker.C {
			refreshData()
		}
	}()

	return app.SetRoot(pages, true).EnableMouse(true).Run()
}

func setupTableHeader(table *tview.Table) {
	headers := []string{"NAME", "BRANCH", "STATUS", "UPTIME", "TASK"}
	for i, h := range headers {
		table.SetCell(0, i, tview.NewTableCell("[::b]"+h).
			SetSelectable(false).
			SetTextColor(tcell.ColorYellow))
	}
}

func statusIcon(status string) string {
	switch status {
	case "running":
		return "✅ "
	case "stopped", "exited":
		return "⏹ "
	case "error":
		return "❌ "
	default:
		return "  "
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

func truncate(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n-3]) + "..."
}

// tviewWriter is an io.Writer that appends text to a tview TextView.
type tviewWriter struct {
	view *tview.TextView
	app  *tview.Application
	buf  strings.Builder
}

func (w *tviewWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	text := w.buf.String()
	w.app.QueueUpdateDraw(func() {
		w.view.SetText(text)
		w.view.ScrollToEnd()
	})
	return len(p), nil
}
