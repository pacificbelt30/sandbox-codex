package cmd

import (
	"fmt"
	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/spf13/cobra"
	"os"
)

func init() {
	files := &cobra.Command{Use: "files", Short: "Read/write guest workspace files without following symlinks"}
	for _, op := range []string{"get", "put", "list"} {
		op := op
		var local string
		c := &cobra.Command{Use: op + " NAME RELATIVE_PATH", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
			m, err := sandbox.NewManager(managementConfig())
			if err != nil {
				return err
			}
			defer func() { _ = m.Close() }()
			if op == "put" {
				in := cmd.InOrStdin()
				if local != "-" {
					f, err := os.Open(local)
					if err != nil {
						return err
					}
					defer func() { _ = f.Close() }()
					in = f
				}
				return m.WriteFile(cmd.Context(), args[0], args[1], in)
			}
			out := cmd.OutOrStdout()
			if local != "-" {
				f, err := os.OpenFile(local, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if err != nil {
					return err
				}
				defer func() { _ = f.Close() }()
				out = f
			}
			if op == "get" {
				return m.ReadFile(cmd.Context(), args[0], args[1], out)
			}
			return m.ListFiles(cmd.Context(), args[0], args[1], out)
		}}
		c.Flags().StringVar(&local, "local", "-", "Local path; - uses stdin/stdout. Downloads require a new file.")
		files.AddCommand(c)
	}
	rootCmd.AddCommand(files)
	var output, message string
	a := &cobra.Command{Use: "artifact", Short: "Recover worker changes"}
	diff := &cobra.Command{Use: "diff NAME", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		m, err := sandbox.NewManager(managementConfig())
		if err != nil {
			return err
		}
		defer func() { _ = m.Close() }()
		out := cmd.OutOrStdout()
		if output != "-" {
			f, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()
			out = f
		}
		return m.ExportDiff(cmd.Context(), args[0], out)
	}}
	diff.Flags().StringVar(&output, "output", "-", "New patch file, or - for stdout")
	commit := &cobra.Command{Use: "commit NAME", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if message == "" {
			return fmt.Errorf("--message is required")
		}
		m, err := sandbox.NewManager(managementConfig())
		if err != nil {
			return err
		}
		defer func() { _ = m.Close() }()
		return m.Commit(cmd.Context(), args[0], message)
	}}
	commit.Flags().StringVarP(&message, "message", "m", "", "Commit staged guest changes")
	a.AddCommand(diff, commit)
	rootCmd.AddCommand(a)
}
