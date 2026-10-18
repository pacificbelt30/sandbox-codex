package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	kataassets "github.com/pacificbelt30/codex-dock/deploy/kata"
	"github.com/spf13/cobra"
)

func init() {
	var bundle, operator, cache string
	var apply, offline, rollback bool
	setup := &cobra.Command{Use: "setup", Short: "Plan or apply a verified Ubuntu 26.04 x86_64 node install/update", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) (runErr error) {
		if !rollback && (bundle == "" || operator == "") {
			return fmt.Errorf("provide --bundle PATH --operator USER; make node-bundle prepares a matching CLI and helpers")
		}
		if apply && os.Geteuid() != 0 {
			return fmt.Errorf("--apply needs root to install services; run the displayed plan first and use sudo for setup only")
		}
		dir, err := os.MkdirTemp("", "codex-dock-setup-")
		if err != nil {
			return err
		}
		defer func() { runErr = errors.Join(runErr, os.RemoveAll(dir)) }()
		entries, err := kataassets.Files.ReadDir(".")
		if err != nil {
			return err
		}
		for _, entry := range entries {
			data, err := kataassets.Files.ReadFile(entry.Name())
			if err != nil {
				return err
			}
			if err = os.WriteFile(filepath.Join(dir, entry.Name()), data, 0600); err != nil {
				return err
			}
		}
		argv := []string{filepath.Join(dir, "setup.py"), "--bundle", bundle, "--operator", operator, "--cache", cache}
		if apply {
			argv = append(argv, "--apply")
		}
		if offline {
			argv = append(argv, "--offline")
		}
		if rollback {
			argv = append(argv, "--rollback")
		}
		child := exec.CommandContext(cmd.Context(), "python3", argv...)
		child.Stdin = cmd.InOrStdin()
		child.Stdout = cmd.OutOrStdout()
		child.Stderr = cmd.ErrOrStderr()
		if err = child.Run(); err != nil {
			return fmt.Errorf("node setup: %w", err)
		}
		return nil
	}}
	setup.Flags().StringVar(&bundle, "bundle", "", "Directory from make node-bundle")
	setup.Flags().StringVar(&operator, "operator", "", "Existing non-root account granted access to this root-equivalent node")
	setup.Flags().StringVar(&cache, "cache", "/var/cache/codex-dock-setup", "Verified upstream archive cache")
	setup.Flags().BoolVar(&apply, "apply", false, "Install/update the dedicated services (default prints a plan)")
	setup.Flags().BoolVar(&offline, "offline", false, "Require cached archives and already installed system packages")
	setup.Flags().BoolVar(&rollback, "rollback", false, "Restore the previous successful setup transaction after all containers are removed")
	rootCmd.AddCommand(setup)
}
