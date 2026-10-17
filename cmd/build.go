package cmd

import (
	"context"
	"fmt"
	"github.com/pacificbelt30/codex-dock/internal/engine"
	"io"
	"os"
	"path/filepath"
	"strings"

	dockerdefaults "github.com/pacificbelt30/codex-dock/docker"
	"github.com/pacificbelt30/codex-dock/internal/template"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var buildTag string
var buildDockerfile string
var buildTemplate string
var buildPrintEntrypoint bool

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the codex-dock base Docker image",
	RunE: func(cmd *cobra.Command, args []string) error {
		if buildPrintEntrypoint {
			return writeEntrypoint(os.Stdout)
		}

		applyBuildConfigDefaults(cmd)

		if buildTemplate == "list" {
			return listTemplates()
		}

		if buildTemplate != "" {
			if buildDockerfile != "" {
				return fmt.Errorf("--template and --dockerfile (-f) are mutually exclusive")
			}
			customTag := ""
			if cmd.Flags().Changed("tag") || viper.GetString("build.tag") != "" {
				customTag = buildTag
			}
			return buildFromTemplate(cmd.Context(), buildTemplate, customTag)
		}

		dockerfile, buildCtx, err := resolveDockerfile(buildDockerfile)
		if err != nil {
			return err
		}
		return executeBuild(cmd.Context(), buildTag, dockerfile, buildCtx)
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)
	buildCmd.Flags().StringVarP(&buildTag, "tag", "t", "codex-dock:latest", "Image tag")
	buildCmd.Flags().StringVarP(&buildDockerfile, "dockerfile", "f", "", "Path to Dockerfile")
	buildCmd.Flags().StringVarP(&buildTemplate, "template", "T", "", `Sandbox image template (e.g. "plain", "pwn"). Use --template list to see available templates.`)
	buildCmd.Flags().BoolVar(&buildPrintEntrypoint, "print-entrypoint", false, `Print the embedded entrypoint.sh to stdout and exit (no build). Use this to vendor the canonical entrypoint into a custom, non-derived Dockerfile (i.e. one that does not FROM codex-dock:*), so it stays in sync with the auth-proxy protocol: codex-dock build --print-entrypoint > entrypoint.sh`)
}

func applyBuildConfigDefaults(cmd *cobra.Command) {
	flags := cmd.Flags()

	if !flags.Changed("template") {
		if v := viper.GetString("build.template"); v != "" {
			buildTemplate = v
		} else if v := viper.GetString("default_template"); v != "" {
			buildTemplate = v
		}
	}

	if !flags.Changed("tag") {
		if v := viper.GetString("build.tag"); v != "" {
			buildTag = v
		}
	}
}

// writeEntrypoint writes the embedded, canonical entrypoint.sh to w. Used by
// --print-entrypoint so a custom, non-derived Dockerfile (one that does not
// FROM codex-dock:*) can vendor the exact script the built-in images ship,
// keeping it in sync with the auth-proxy protocol instead of relying on a
// hand-copied file that can drift.
func writeEntrypoint(w io.Writer) error {
	_, err := w.Write(dockerdefaults.Entrypoint)
	return err
}

// resolveDockerfile returns the Dockerfile path and build context directory to use.
// Priority: explicit -f flag > Dockerfile / docker/Dockerfile in CWD > config-dir default.
func resolveDockerfile(flagValue string) (string, string, error) {
	if flagValue != "" {
		return flagValue, ".", nil
	}

	// Check well-known locations relative to the current directory.
	// docker/sandbox/Dockerfile is the current layout; docker/Dockerfile is
	// kept for backward compatibility with older checkouts.
	for _, p := range []string{"Dockerfile", "docker/sandbox/Dockerfile", "docker/Dockerfile"} {
		if _, err := os.Stat(p); err == nil {
			return p, filepath.Dir(p), nil
		}
	}

	// Fall back to the default Dockerfile written into the config directory.
	configDir, err := defaultConfigDir()
	if err != nil {
		return "", "", fmt.Errorf("dockerfile not found; use -f to specify path")
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
// --progress=plain forces BuildKit to stream every step's full output line-by-line
// instead of collapsing it into a live-updating TTY view, so a build triggered from
// `run`'s auto-build never looks stalled while it's actually still working.
func executeBuild(ctx context.Context, tag, dockerfile, buildCtx string) error {
	fmt.Printf("Building image %s from %s...\n", tag, dockerfile)
	if err := engine.Build(ctx, tag, dockerfile, buildCtx, os.Stderr); err != nil {
		return err
	}
	fmt.Printf("Image %s built successfully.\n", tag)
	return nil
}

func listTemplates() error {
	templates, err := template.List()
	if err != nil {
		return fmt.Errorf("listing templates: %w", err)
	}
	fmt.Println("Available templates:")
	for _, t := range templates {
		tag := t.Tag("")
		extra := ""
		if t.IsDerived() {
			extra = fmt.Sprintf(" (extends %s)", t.BaseImage)
		}
		if t.UserDefined {
			extra += " [user]"
		}
		fmt.Printf("  %-12s %s%s\n", t.Name, tag, extra)
	}
	if dir, err := template.UserTemplatesDir(); err == nil {
		fmt.Printf("\nUser templates directory: %s\n", dir)
		fmt.Println("Add <dir>/<name>/Dockerfile to add a template or override a built-in one.")
	}
	return nil
}

func buildFromTemplate(ctx context.Context, name, customTag string) error {
	tmpl, err := template.Get(name)
	if err != nil {
		return err
	}

	result := template.Validate(tmpl)
	if !result.IsValid() {
		return fmt.Errorf("template %q failed validation:\n  - %s",
			name, strings.Join(result.Failed, "\n  - "))
	}

	tag := tmpl.Tag(customTag)

	if tmpl.IsDerived() {
		if err := ensureBaseImage(ctx, tmpl.BaseImage); err != nil {
			return fmt.Errorf("building base image for template %q: %w", name, err)
		}
	}

	return buildTemplateImage(ctx, tmpl, tag)
}

func ensureBaseImage(ctx context.Context, baseImage string) error {
	c, err := engine.NewClient()
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	if _, err := c.ImageInspect(ctx, baseImage); err == nil {
		return nil
	}

	fmt.Printf("Base image %s not found, building...\n", baseImage)
	plainTmpl, err := template.Get("plain")
	if err != nil {
		return err
	}
	return buildTemplateImage(ctx, plainTmpl, baseImage)
}

func buildTemplateImage(ctx context.Context, tmpl template.Info, tag string) error {
	tmpDir, err := os.MkdirTemp("", "codex-dock-template-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	dfPath := filepath.Join(tmpDir, "Dockerfile")
	if err := os.WriteFile(dfPath, tmpl.Dockerfile, 0644); err != nil {
		return err
	}

	if tmpl.IsBase {
		epPath := filepath.Join(tmpDir, "entrypoint.sh")
		if err := os.WriteFile(epPath, dockerdefaults.Entrypoint, 0755); err != nil {
			return err
		}
	}

	return executeBuild(ctx, tag, dfPath, tmpDir)
}
