package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const projectConfigExample = `# Commit this file with the project. Host credentials, proxy and runtime
# settings stay in ~/.config/codex-dock/config.toml.
[run]
image = "codex-dock:latest"
approval_mode = "suggest"
# agent = "codex" # oaiso default codex --local also sets this
# model = "..."
# watch_polling = true
# read_only = true
# no_internet = true

[build]
template = "plain"

# For a custom image, change run.image, replace build.template with
# build.dockerfile = "Dockerfile.codex-dock", and set build.tag to the same
# image name. See the configuration guide for a complete example.
# You can also commit packages.dock for packages installed at worker startup.
`

func init() {
	project := &cobra.Command{Use: "project", Short: "Manage repository-local worker configuration"}
	project.AddCommand(&cobra.Command{Use: "init", Short: "Create a tracked codex-dock.toml in the current directory", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		path := filepath.Join(cwd, projectConfigName)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return fmt.Errorf("creating %s: %w", path, err)
		}
		if _, err = f.WriteString(projectConfigExample); err != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), path)
		return err
	}})
	project.AddCommand(&cobra.Command{Use: "check", Short: "Validate the repository-local config without starting a worker", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		cfg, err := loadProjectConfig(cwd)
		if err != nil {
			return err
		}
		if cfg == nil {
			return fmt.Errorf("no %s in %s; run `oaiso project init`", projectConfigName, cwd)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), projectSettingsSummary(cfg))
		return err
	}})
	rootCmd.AddCommand(project)
}
