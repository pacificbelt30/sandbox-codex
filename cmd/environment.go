package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/pacificbelt30/codex-dock/internal/engine"
	"github.com/pacificbelt30/codex-dock/internal/environmentlock"
	"github.com/spf13/cobra"
)

func init() {
	var output, root string
	var files []string
	e := &cobra.Command{Use: "environment", Short: "Pin reproducible worker environments"}
	c := &cobra.Command{Use: "lock IMAGE", Short: "Write a new lock for an existing node image and selected sources", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		cli, err := engine.NewClient()
		if err != nil {
			return err
		}
		defer func() { _ = cli.Close() }()
		info, err := cli.ImageInspect(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if info.ID == "" {
			return fmt.Errorf("node does not expose image digests; upgrade the node")
		}
		l, err := environmentlock.New(root, args[0], info.ID, files)
		if err != nil {
			return err
		}
		if output == "-" {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(l)
		}
		return l.Write(output)
	}}
	c.Flags().StringVar(&output, "output", "codex-dock.lock.json", "New lock path, or - for stdout; existing files are never overwritten")
	c.Flags().StringVar(&root, "project", ".", "Source project directory")
	c.Flags().StringArrayVar(&files, "source", nil, "Relative source file to hash (repeatable)")
	e.AddCommand(c)
	rootCmd.AddCommand(e)
}
