package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pacificbelt30/codex-dock/internal/template"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

const projectConfigName = "codex-dock.toml"

// ProjectConfig contains only portable worker settings. Host credentials,
// runtime selection, proxy endpoints and extra host mounts remain global or
// explicit CLI options so cloning a repository cannot silently change them.
type ProjectConfig struct {
	Root string `toml:"-"`
	Run  struct {
		Image        string `toml:"image"`
		Template     string `toml:"template"`
		Agent        string `toml:"agent"`
		Model        string `toml:"model"`
		ApprovalMode string `toml:"approval_mode"`
		WatchPolling *bool  `toml:"watch_polling"`
		ReadOnly     *bool  `toml:"read_only"`
		NoInternet   *bool  `toml:"no_internet"`
	} `toml:"run"`
	Build struct {
		Dockerfile string `toml:"dockerfile"`
		Template   string `toml:"template"`
		Tag        string `toml:"tag"`
	} `toml:"build"`
}

var projectKeys = map[string]map[string]bool{
	"run": {
		"image": true, "template": true, "agent": true, "model": true,
		"approval_mode": true, "watch_polling": true,
		"read_only": true, "no_internet": true,
	},
	"build": {"dockerfile": true, "template": true, "tag": true},
}

func loadProjectConfig(dir string) (*ProjectConfig, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, projectConfigName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading project config %s: %w", path, err)
	}
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing project config %s: %w", path, err)
	}
	for section, value := range raw {
		allowed, ok := projectKeys[section]
		if !ok {
			return nil, fmt.Errorf("%s: %q is a host setting or unknown section; use the global config for host settings", path, section)
		}
		entries, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: %q must be a TOML table", path, section)
		}
		for key := range entries {
			if !allowed[key] {
				return nil, fmt.Errorf("%s: unsupported project setting %s.%s", path, section, key)
			}
		}
	}
	var cfg ProjectConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid project config %s: %w", path, err)
	}
	cfg.Root = root
	if cfg.Run.Image != "" && cfg.Run.Template != "" {
		return nil, fmt.Errorf("%s: choose either run.image or run.template", path)
	}
	if cfg.Run.Agent != "" && !validAgentChoice(cfg.Run.Agent) {
		return nil, fmt.Errorf("%s: run.agent must be shell, codex, or claude", path)
	}
	if cfg.Build.Dockerfile != "" && cfg.Build.Template != "" {
		return nil, fmt.Errorf("%s: choose either build.dockerfile or build.template", path)
	}
	if cfg.Build.Dockerfile != "" && !filepath.IsLocal(cfg.Build.Dockerfile) {
		return nil, fmt.Errorf("%s: build.dockerfile must be a path inside the project", path)
	}
	if cfg.Build.Dockerfile != "" {
		dockerfile := filepath.Join(root, cfg.Build.Dockerfile)
		resolved, err := filepath.EvalSymlinks(dockerfile)
		if err != nil {
			return nil, fmt.Errorf("%s: build.dockerfile: %w", path, err)
		}
		canonicalRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(canonicalRoot, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("%s: build.dockerfile must resolve inside the project", path)
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s: build.dockerfile must be a regular file", path)
		}
	}
	if cfg.Run.ApprovalMode != "" && cfg.Run.ApprovalMode != "suggest" && cfg.Run.ApprovalMode != "auto-edit" {
		return nil, fmt.Errorf("%s: project approval_mode must be suggest or auto-edit; set stronger modes explicitly on the CLI or in the global config", path)
	}
	if cfg.Run.ReadOnly != nil && !*cfg.Run.ReadOnly {
		return nil, fmt.Errorf("%s: project read_only can only be true; a project cannot loosen a host restriction", path)
	}
	if cfg.Run.NoInternet != nil && !*cfg.Run.NoInternet {
		return nil, fmt.Errorf("%s: project no_internet can only be true; a project cannot loosen a host restriction", path)
	}
	for _, entry := range []struct{ section, name string }{{"run", cfg.Run.Template}, {"build", cfg.Build.Template}} {
		if entry.name != "" {
			if err := validatePortableTemplate(entry.name); err != nil {
				return nil, fmt.Errorf("%s: %s.template: %w", path, entry.section, err)
			}
		}
	}
	if cfg.Build.Dockerfile != "" && cfg.Build.Tag == "" {
		return nil, fmt.Errorf("%s: set build.tag to run.image for a project Dockerfile", path)
	}
	runImage := cfg.Run.Image
	if cfg.Run.Template != "" {
		tmpl, _ := template.Get(cfg.Run.Template)
		runImage = tmpl.Tag("")
	}
	if name := template.MatchTag(runImage); name != "" && cfg.Build.Dockerfile == "" {
		if err := validatePortableTemplate(name); err != nil {
			return nil, fmt.Errorf("%s: run image: %w", path, err)
		}
	}
	buildImage := cfg.Build.Tag
	if buildImage == "" && cfg.Build.Template != "" {
		tmpl, _ := template.Get(cfg.Build.Template)
		buildImage = tmpl.Tag("")
	}
	if runImage != "" && buildImage != "" && runImage != buildImage {
		return nil, fmt.Errorf("%s: build image %q must match run image %q so a fresh clone builds the image it runs", path, buildImage, runImage)
	}
	if runImage != "" && template.MatchTag(runImage) == "" && cfg.Build.Dockerfile == "" && cfg.Build.Template == "" {
		return nil, fmt.Errorf("%s: custom run.image requires build.dockerfile or build.template and a matching build.tag", path)
	}
	return &cfg, nil
}

func validatePortableTemplate(name string) error {
	visited := make(map[string]bool)
	for name != "" {
		if visited[name] {
			return fmt.Errorf("template %q has a cyclic base image", name)
		}
		visited[name] = true
		current, err := template.Get(name)
		if err != nil {
			return err
		}
		bundled, err := template.BuiltIn(name)
		if err != nil || !bytes.Equal(current.Dockerfile, bundled.Dockerfile) {
			return fmt.Errorf("template %q differs from the bundled version; commit a project Dockerfile instead", name)
		}
		name = template.MatchTag(bundled.BaseImage)
	}
	return nil
}

func applyProjectRunConfig(cmd *cobra.Command, cfg *ProjectConfig) {
	if cfg == nil {
		return
	}
	flags := cmd.Flags()
	if !flags.Changed("image") && !flags.Changed("template") {
		if cfg.Run.Image != "" {
			runOpts.Image = cfg.Run.Image
		} else if cfg.Run.Template != "" {
			tmpl, _ := template.Get(cfg.Run.Template) // validated at load
			runOpts.Image = tmpl.Tag("")
		}
	}
	if !flags.Changed("model") && cfg.Run.Model != "" {
		runOpts.Model = cfg.Run.Model
	}
	if !runOpts.DirectAgent && !flags.Changed("agent") && cfg.Run.Agent != "" {
		agentFlag = cfg.Run.Agent
	}
	if !flags.Changed("approval-mode") && cfg.Run.ApprovalMode != "" {
		approvalModeFlag = cfg.Run.ApprovalMode
	}
	if !flags.Changed("watch-polling") && cfg.Run.WatchPolling != nil {
		runOpts.WatchPolling = *cfg.Run.WatchPolling
	}
	if !flags.Changed("read-only") && cfg.Run.ReadOnly != nil && *cfg.Run.ReadOnly {
		runOpts.ReadOnly = true
	}
	if !flags.Changed("no-internet") && cfg.Run.NoInternet != nil && *cfg.Run.NoInternet {
		runOpts.NoInternet = true
	}
}

func applyProjectBuildConfig(cmd *cobra.Command, cfg *ProjectConfig) {
	if cfg == nil {
		return
	}
	flags := cmd.Flags()
	if !flags.Changed("template") && !flags.Changed("dockerfile") {
		if cfg.Build.Template != "" {
			buildTemplate = cfg.Build.Template
			buildDockerfile = ""
		} else if cfg.Build.Dockerfile != "" {
			buildDockerfile = filepath.Join(cfg.Root, cfg.Build.Dockerfile)
			buildTemplate = ""
		}
	}
	if !flags.Changed("tag") && cfg.Build.Tag != "" {
		buildTag = cfg.Build.Tag
	}
}

func (cfg *ProjectConfig) dockerfile() (string, string, bool) {
	if cfg == nil || cfg.Build.Dockerfile == "" {
		return "", "", false
	}
	return filepath.Join(cfg.Root, cfg.Build.Dockerfile), cfg.Root, true
}

func buildProjectImage(ctx context.Context, cfg *ProjectConfig, tag string) error {
	dockerfile, buildCtx, ok := cfg.dockerfile()
	if !ok {
		return fmt.Errorf("no project Dockerfile configured")
	}
	data, err := os.ReadFile(dockerfile)
	if err != nil {
		return err
	}
	// The usual project Dockerfile extends the bundled worker image. Build that
	// base on a fresh clone before asking BuildKit to resolve it locally.
	if projectDockerfileUsesBase(data, "codex-dock:latest") {
		if err := ensureBaseImage(ctx, "codex-dock:latest"); err != nil {
			return err
		}
	}
	return executeBuild(ctx, tag, dockerfile, buildCtx)
}

func projectDockerfileUsesBase(data []byte, image string) bool {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.EqualFold(fields[0], "FROM") {
			continue
		}
		for _, field := range fields[1:] {
			if strings.HasPrefix(field, "--") {
				continue
			}
			if field == image {
				return true
			}
			break
		}
	}
	return false
}

func projectSettingsSummary(cfg *ProjectConfig) string {
	if cfg == nil {
		return "No " + projectConfigName + " in this directory."
	}
	rows := []string{"Project config: " + filepath.Join(cfg.Root, projectConfigName)}
	if cfg.Run.Image != "" {
		rows = append(rows, "run.image = "+cfg.Run.Image)
	}
	if cfg.Run.Template != "" {
		rows = append(rows, "run.template = "+cfg.Run.Template)
	}
	if cfg.Run.Agent != "" {
		rows = append(rows, "run.agent = "+cfg.Run.Agent)
	}
	if cfg.Build.Dockerfile != "" {
		rows = append(rows, "build.dockerfile = "+cfg.Build.Dockerfile)
	}
	if cfg.Build.Template != "" {
		rows = append(rows, "build.template = "+cfg.Build.Template)
	}
	if cfg.Build.Tag != "" {
		rows = append(rows, "build.tag = "+cfg.Build.Tag)
	}
	sort.Strings(rows[1:])
	return strings.Join(rows, "\n")
}
