package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/pacificbelt30/codex-dock/internal/engine"
	"github.com/pacificbelt30/codex-dock/internal/kataguard"
	"github.com/spf13/cobra"
)

func init() {
	var asJSON bool
	c := &cobra.Command{Use: "capabilities", Short: "Query the connected node's validated feature set", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		socket := engine.Socket()
		node, err := kataguard.Connect(socket, "unix://"+socket)
		if err != nil {
			return err
		}
		defer func() { _ = node.Close() }()
		if err = node.Validate(cmd.Context(), "io.containerd.kata.v2"); err != nil {
			return err
		}
		version, features := node.Capabilities()
		result := struct {
			BuildVersion string   `json:"build_version"`
			Protocol     int      `json:"protocol"`
			Features     []string `json:"features"`
		}{version, node.ProtocolVersion(), features}
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Node: %s (protocol %d)\n", version, result.Protocol)
		if err != nil {
			return err
		}
		for _, feature := range features {
			if _, err = fmt.Fprintln(cmd.OutOrStdout(), feature); err != nil {
				return err
			}
		}
		return nil
	}}
	c.Flags().BoolVar(&asJSON, "json", false, "Print JSON")
	rootCmd.AddCommand(c)
}
