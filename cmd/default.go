package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// The named "shell" choice means the authenticated shell. --shell remains
// the separate, unauthenticated debugging mode.
func validAgentChoice(value string) bool {
	return value == "shell" || value == "codex" || value == "claude"
}

func parseAgentChoice(value string) (sandbox.Agent, error) {
	if value == "" || value == "shell" {
		return sandbox.AgentNone, nil
	}
	if validAgentChoice(value) {
		return sandbox.Agent(value), nil
	}
	return "", fmt.Errorf("invalid agent %q; choose shell, codex, or claude", value)
}

func resolveSelectedAgent(cmd *cobra.Command, execCmd string) (sandbox.Agent, error) {
	selected := agentFlag
	if runOpts.ShellMode || (execCmd != "" && !cmd.Flags().Changed("agent") && !runOpts.DirectAgent) {
		selected = "shell"
	}
	return parseAgentChoice(selected)
}

var defaultLocal bool

var defaultCmd = &cobra.Command{
	Use:   "default [shell|codex|claude]",
	Short: "Show or change the agent started by a plain oaiso run",
	Long: `Show the effective agent for a plain oaiso run, or save a new default.
Changes are global unless --local is used to write codex-dock.toml in the current project.
An explicit --agent, --exec, IMAGE COMMAND, or -- COMMAND overrides the default.
The native oaiso codex command always starts Codex.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDefaultCommand,
}

func init() {
	defaultCmd.Flags().BoolVar(&defaultLocal, "local", false, "Save the default in this project's codex-dock.toml")
	rootCmd.AddCommand(defaultCmd)
}

func runDefaultCommand(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		project, err := loadProjectConfig(cwd)
		if err != nil {
			return err
		}
		value, source := effectiveDefaultAgent(project)
		if _, err := parseAgentChoice(value); err != nil {
			return fmt.Errorf("configured default: %w", err)
		}
		if value == "" {
			value = "shell"
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s (%s)\n", value, source)
		return err
	}
	value := args[0]
	if !validAgentChoice(value) {
		return fmt.Errorf("invalid default %q; choose shell, codex, or claude", value)
	}
	var path string
	var err error
	if defaultLocal {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		path = filepath.Join(cwd, projectConfigName)
	} else if cfgFile != "" {
		path, err = filepath.Abs(cfgFile)
		if err != nil {
			return err
		}
	} else {
		configDir, err := defaultConfigDir()
		if err != nil {
			return err
		}
		path = filepath.Join(configDir, "config.toml")
	}
	if err := writeRunAgent(path, value, defaultLocal); err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "run.agent = %s (%s)\n", value, path)
	return err
}

func effectiveDefaultAgent(project *ProjectConfig) (string, string) {
	if project != nil && project.Run.Agent != "" {
		return project.Run.Agent, filepath.Join(project.Root, projectConfigName)
	}
	if viper.IsSet("run.agent") {
		return viper.GetString("run.agent"), "global config"
	}
	return "shell", "built-in default"
}

var (
	runHeaderPattern   = regexp.MustCompile(`^\s*\[\s*run\s*\]\s*(?:#.*)?$`)
	tableHeaderPattern = regexp.MustCompile(`^\s*\[.*\]\s*(?:#.*)?$`)
	agentLinePattern   = regexp.MustCompile(`^(\s*(?:agent|run\.agent)\s*=\s*)(?:"(?:[^"\\]|\\.)*"|'[^']*')(\s*(?:#.*)?)$`)
)

// writeRunAgent changes only run.agent, preserving unrelated keys and comments.
// It validates both the input and result before replacing the file atomically.
func writeRunAgent(path, value string, local bool) error {
	if !validAgentChoice(value) {
		return fmt.Errorf("invalid default %q", value)
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	mode := os.FileMode(0600)
	if local {
		mode = 0644
	}
	if err == nil {
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return statErr
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", path)
		}
		mode = info.Mode().Perm()
	}
	var raw map[string]any
	if len(data) > 0 {
		if err := toml.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
	}
	if local && len(data) > 0 {
		if _, err := loadProjectConfig(filepath.Dir(path)); err != nil {
			return err
		}
	}
	updated, err := replaceRunAgent(data, raw, value)
	if err != nil {
		return fmt.Errorf("editing %s: %w", path, err)
	}
	var check map[string]any
	if err := toml.Unmarshal(updated, &check); err != nil {
		return fmt.Errorf("validating updated %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".cxd-default-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(updated); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("saving default to %s: %w", path, err)
	}
	return nil
}

func replaceRunAgent(data []byte, raw map[string]any, value string) ([]byte, error) {
	lineEnding := "\n"
	if strings.Contains(string(data), "\r\n") {
		lineEnding = "\r\n"
	}
	content := string(data)
	lines := strings.SplitAfter(content, "\n")
	inRun, runHeader, agentLine := false, -1, -1
	for i, line := range lines {
		trimmed := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if runHeaderPattern.MatchString(trimmed) {
			inRun = true
			runHeader = i
			continue
		}
		if tableHeaderPattern.MatchString(trimmed) {
			inRun = false
			continue
		}
		candidate := strings.TrimSpace(trimmed)
		if (inRun && strings.HasPrefix(candidate, "agent")) || (runHeader == -1 && strings.HasPrefix(candidate, "run.agent")) {
			if agentLinePattern.MatchString(trimmed) {
				agentLine = i
				break
			}
		}
	}
	if agentLine >= 0 {
		line := strings.TrimSuffix(strings.TrimSuffix(lines[agentLine], "\n"), "\r")
		parts := agentLinePattern.FindStringSubmatch(line)
		lines[agentLine] = parts[1] + `"` + value + `"` + parts[2] + lineEnding
		return []byte(strings.Join(lines, "")), nil
	}
	if run, ok := raw["run"]; ok {
		table, ok := run.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("run must be a TOML table")
		}
		if _, exists := table["agent"]; exists {
			return nil, fmt.Errorf("run.agent uses unsupported TOML syntax; edit it manually")
		}
		if runHeader < 0 {
			return nil, fmt.Errorf("run table uses unsupported TOML syntax; edit it manually")
		}
	}
	if runHeader >= 0 {
		line := lines[runHeader]
		if !strings.HasSuffix(line, "\n") {
			line += lineEnding
		}
		lines[runHeader] = line + `agent = "` + value + `"` + lineEnding
		return []byte(strings.Join(lines, "")), nil
	}
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += lineEnding
	}
	if len(content) > 0 {
		content += lineEnding
	}
	return []byte(content + `[run]` + lineEnding + `agent = "` + value + `"` + lineEnding), nil
}
