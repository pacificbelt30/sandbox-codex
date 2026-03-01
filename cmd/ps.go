package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/spf13/cobra"
)

var psAll bool

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "List codex-dock worker containers",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := sandbox.NewManager(sandbox.ManagerConfig{Verbose: verbose})
		if err != nil {
			return err
		}

		workers, err := mgr.List(psAll)
		if err != nil {
			return fmt.Errorf("listing containers: %w", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tSTATUS\tUPTIME\tBRANCH\tTASK\tIMAGE")
		for _, wk := range workers {
			uptime := "-"
			if wk.StartedAt != nil {
				uptime = formatDuration(time.Since(*wk.StartedAt))
			}
			branch := wk.Branch
			if branch == "" {
				branch = "-"
			}
			task := wk.Task
			if task == "" {
				task = "(interactive)"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				wk.Name, statusIcon(wk.Status)+wk.Status, uptime, branch, truncate(task, 30), wk.Image)
		}
		return w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(psCmd)
	psCmd.Flags().BoolVarP(&psAll, "all", "a", false, "Show all containers including stopped ones")
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
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
