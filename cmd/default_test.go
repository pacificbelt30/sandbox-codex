package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestWriteRunAgentPreservesConfigAndComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	original := "# keep this comment\ndefault_image = 'custom:dev'\n[run]\nmodel = 'model-a'\nagent = 'shell' # keep inline comment\n[proxy]\nadmin_secret = 'hidden'\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeRunAgent(path, "codex", false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# keep this comment", "default_image = 'custom:dev'", "model = 'model-a'", "agent = \"codex\" # keep inline comment", "admin_secret = 'hidden'"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("updated config lost %q:\n%s", want, data)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissions = %v; want 0600", info.Mode().Perm())
	}
	if err := writeRunAgent(path, "claude", false); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if strings.Count(string(data), "agent =") != 1 || !strings.Contains(string(data), `agent = "claude"`) {
		t.Fatalf("repeated update = %s", data)
	}
}

func TestWriteRunAgentCreatesProjectConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, projectConfigName)
	if err := writeRunAgent(path, "codex", true); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadProjectConfig(dir)
	if err != nil || cfg.Run.Agent != "codex" {
		t.Fatalf("project config = %+v, error = %v", cfg, err)
	}
	if err := os.WriteFile(path, []byte(projectConfigExample), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeRunAgent(path, "claude", true); err != nil {
		t.Fatal(err)
	}
	cfg, err = loadProjectConfig(dir)
	if err != nil || cfg.Run.Agent != "claude" || cfg.Build.Template != "plain" {
		t.Fatalf("updated scaffold = %+v, error = %v", cfg, err)
	}
	if err := os.WriteFile(path, []byte("[run]\nimage = 'custom:dev'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeRunAgent(path, "shell", true); err == nil {
		t.Fatal("accepted invalid project config with an unreproducible custom image")
	}
}

func TestWriteRunAgentCreatesPrivateGlobalConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "config.toml")
	if err := writeRunAgent(path, "codex", false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("new global config permissions = %v; want 0600", info.Mode().Perm())
	}
}

func TestWriteLocalDefaultRequiresProjectFilenameMigration(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "codex-dock.toml")
	const content = "[run]\nread_only = true\n"
	if err := os.WriteFile(old, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "oaiso.toml")
	if err := writeRunAgent(path, "codex", true); err == nil || !strings.Contains(err.Error(), "obsolete project config") {
		t.Fatalf("default --local should require migration: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("new project file was unexpectedly written: %v", err)
	}
	data, err := os.ReadFile(old)
	if err != nil || string(data) != content {
		t.Fatalf("old settings changed: %q, %v", data, err)
	}
}

func TestWriteRunAgentRejectsMalformedInputWithoutChangingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	bad := []byte("[run]\nagent = [\n")
	if err := os.WriteFile(path, bad, 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeRunAgent(path, "codex", false); err == nil {
		t.Fatal("accepted malformed TOML")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(bad) {
		t.Fatalf("malformed input changed: %q", got)
	}
}

func TestDefaultAgentPrecedenceAndCommands(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	oldOpts, oldAgent := runOpts, agentFlag
	t.Cleanup(func() { runOpts, agentFlag = oldOpts, oldAgent })
	viper.Set("run.agent", "codex")
	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("agent", "", "")
	applyRunConfigDefaults(cmd)
	if agentFlag != "codex" {
		t.Fatalf("global default = %q", agentFlag)
	}
	project := &ProjectConfig{}
	project.Run.Agent = "claude"
	applyProjectRunConfig(cmd, project)
	if agentFlag != "claude" {
		t.Fatalf("project default = %q", agentFlag)
	}
	for _, tc := range []struct {
		name string
		exec string
		want sandbox.Agent
	}{
		{"plain run", "", sandbox.AgentClaude},
		{"run command", "npm test", sandbox.AgentNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveSelectedAgent(cmd, tc.exec)
			if err != nil || got != tc.want {
				t.Fatalf("agent = %q, error = %v; want %q", got, err, tc.want)
			}
		})
	}
	runOpts.ShellMode = true
	if got, err := resolveSelectedAgent(cmd, ""); err != nil || got != sandbox.AgentNone {
		t.Fatalf("raw shell override = %q, error = %v", got, err)
	}
	runOpts.ShellMode = false
	if err := cmd.Flags().Set("agent", "codex"); err != nil {
		t.Fatal(err)
	}
	agentFlag = "codex"
	applyProjectRunConfig(cmd, project)
	if got, err := resolveSelectedAgent(cmd, "npm test"); err != nil || got != sandbox.AgentCodex {
		t.Fatalf("explicit flag with command = %q, error = %v", got, err)
	}
	runOpts.DirectAgent = true
	agentFlag = "codex"
	directCmd := &cobra.Command{Use: "codex"}
	applyRunConfigDefaults(directCmd)
	applyProjectRunConfig(directCmd, project)
	if got, err := resolveSelectedAgent(directCmd, ""); err != nil || got != sandbox.AgentCodex {
		t.Fatalf("direct codex = %q, error = %v", got, err)
	}
}
