package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/spf13/cobra"
)

func init() {
	jobs := &cobra.Command{Use: "jobs", Short: "Start and inspect persistent worker background commands"}
	start := &cobra.Command{Use: "start NAME -- COMMAND [ARG...]", Args: cobra.MinimumNArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		m, err := sandbox.NewManager(managementConfig())
		if err != nil {
			return err
		}
		defer func() { _ = m.Close() }()
		job, err := m.StartJob(cmd.Context(), args[0], sandbox.ExecOptions{Argv: execArguments(args)})
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(job)
	}}
	start.Flags().SetInterspersed(false)
	jobs.AddCommand(start)
	for _, op := range []string{"list", "inspect", "logs", "kill"} {
		op := op
		var offset int64
		c := &cobra.Command{Use: op + " NAME [JOB_ID]", Args: func(cmd *cobra.Command, args []string) error {
			if op == "list" {
				return cobra.ExactArgs(1)(cmd, args)
			}
			return cobra.ExactArgs(2)(cmd, args)
		}, RunE: func(cmd *cobra.Command, args []string) error {
			m, err := sandbox.NewManager(managementConfig())
			if err != nil {
				return err
			}
			defer func() { _ = m.Close() }()
			var result any
			switch op {
			case "list":
				result, err = m.ListJobs(cmd.Context(), args[0])
			case "inspect":
				result, err = m.Job(cmd.Context(), args[0], args[1])
			case "logs":
				result, err = m.JobLogs(cmd.Context(), args[0], args[1], offset)
			case "kill":
				err = m.KillJob(cmd.Context(), args[0], args[1])
				if err == nil {
					_, err = fmt.Fprintln(cmd.OutOrStdout(), "Job stopped")
				}
				return err
			}
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		}}
		if op == "logs" {
			c.Flags().Int64Var(&offset, "offset", 0, "Read retained output starting at this byte offset")
		}
		jobs.AddCommand(c)
	}
	rootCmd.AddCommand(jobs)
}
