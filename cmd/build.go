package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	dockerdefaults "github.com/pacificbelt30/codex-dock/docker"
	"github.com/spf13/cobra"
)

var buildTag string
var buildDockerfile string

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the codex-dock base Docker image",
	RunE: func(cmd *cobra.Command, args []string) error {
		dockerfile, buildCtx, err := resolveDockerfile(buildDockerfile)
		if err != nil {
			return err
		}
		return executeBuild(buildTag, dockerfile, buildCtx)
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)
	buildCmd.Flags().StringVarP(&buildTag, "tag", "t", "codex-dock:latest", "Image tag")
	buildCmd.Flags().StringVarP(&buildDockerfile, "dockerfile", "f", "", "Path to Dockerfile")
}

// resolveDockerfile returns the Dockerfile path and build context directory to use.
// Priority: explicit -f flag > Dockerfile / docker/Dockerfile in CWD > config-dir default.
func resolveDockerfile(flagValue string) (string, string, error) {
	if flagValue != "" {
		return flagValue, ".", nil
	}

	// Check well-known locations relative to the current directory.
	for _, p := range []string{"Dockerfile", "docker/Dockerfile"} {
		if _, err := os.Stat(p); err == nil {
			return p, filepath.Dir(p), nil
		}
	}

	// Fall back to the default Dockerfile written into the config directory.
	configDir, err := defaultConfigDir()
	if err != nil {
		return "", "", fmt.Errorf("Dockerfile not found; use -f to specify path")
	}
	if err := ensureDefaultDockerfile(configDir); err != nil {
		return "", "", fmt.Errorf("writing default Dockerfile to config dir: %w", err)
	}
	return filepath.Join(configDir, "Dockerfile"), configDir, nil
}

// defaultConfigDir returns ~/.config/codex-dock.
func defaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "codex-dock"), nil
}

// ensureDefaultDockerfile writes the embedded Dockerfile and entrypoint.sh into
// dir if they are not already present.
func ensureDefaultDockerfile(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	dfPath := filepath.Join(dir, "Dockerfile")
	if _, err := os.Stat(dfPath); os.IsNotExist(err) {
		if err := os.WriteFile(dfPath, dockerdefaults.Dockerfile, 0644); err != nil {
			return err
		}
	}
	epPath := filepath.Join(dir, "entrypoint.sh")
	if _, err := os.Stat(epPath); os.IsNotExist(err) {
		if err := os.WriteFile(epPath, dockerdefaults.Entrypoint, 0755); err != nil {
			return err
		}
	}
	return nil
}

// executeBuild runs "docker build" with the given tag, Dockerfile, and build context.
func executeBuild(tag, dockerfile, buildCtx string) error {
	fmt.Printf("Building image %s from %s...\n", tag, dockerfile)
	c := exec.Command("docker", "build", "-t", tag, "-f", dockerfile, buildCtx)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}
	fmt.Printf("Image %s built successfully.\n", tag)
	return nil
}
