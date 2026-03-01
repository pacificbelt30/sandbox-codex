package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var buildTag string
var buildDockerfile string

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the codex-dock base Docker image",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Find Dockerfile relative to binary or current dir
		if buildDockerfile == "" {
			buildDockerfile = findDockerfile()
		}

		if buildDockerfile == "" {
			return fmt.Errorf("Dockerfile not found; use --dockerfile to specify path")
		}

		fmt.Printf("Building image %s from %s...\n", buildTag, buildDockerfile)

		c := exec.Command("docker", "build", "-t", buildTag, "-f", buildDockerfile, ".")
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("docker build failed: %w", err)
		}
		fmt.Printf("Image %s built successfully.\n", buildTag)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)
	buildCmd.Flags().StringVarP(&buildTag, "tag", "t", "codex-dock:latest", "Image tag")
	buildCmd.Flags().StringVar(&buildDockerfile, "dockerfile", "", "Path to Dockerfile")
}

func findDockerfile() string {
	candidates := []string{
		"docker/Dockerfile",
		"Dockerfile",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
