package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pacificbelt30/codex-dock/internal/sandbox"
	"github.com/pacificbelt30/codex-dock/internal/template"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func writeProjectConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, projectConfigName), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestProjectConfigScaffoldLoads(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, projectConfigExample)
	cfg, err := loadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Run.Image != "codex-dock:latest" || cfg.Build.Template != "plain" {
		t.Fatalf("scaffold did not produce matching run/build config: %+v", cfg)
	}
}

func TestProjectConfigAllowsUnchangedInstalledTemplate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	plain, err := template.BuiltIn("plain")
	if err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(home, ".config", "codex-dock", "templates", "plain")
	if err := os.MkdirAll(installed, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(installed, "Dockerfile")
	if err := os.WriteFile(path, plain.Dockerfile, 0644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeProjectConfig(t, dir, projectConfigExample)
	if _, err := loadProjectConfig(dir); err != nil {
		t.Fatalf("bundled template copy rejected: %v", err)
	}
	if err := os.WriteFile(path, []byte("FROM scratch\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadProjectConfig(dir); err == nil || !strings.Contains(err.Error(), "differs from the bundled version") {
		t.Fatalf("modified template error = %v", err)
	}
	writeProjectConfig(t, dir, "[run]\nimage = 'codex-dock:pwn'\n[build]\ntemplate = 'pwn'\n")
	if _, err := loadProjectConfig(dir); err == nil || !strings.Contains(err.Error(), "template \"plain\" differs") {
		t.Fatalf("modified base template error = %v", err)
	}
}

func TestProjectConfigOverridesGlobalButNotFlags(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, `[run]
image = "project:dev"
model = "project-model"
approval_mode = "suggest"
watch_polling = true
read_only = true
no_internet = true
[build]
dockerfile = "Dockerfile.codex-dock"
tag = "project:dev"
`)
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile.codex-dock"), []byte("FROM codex-dock:latest\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	viper.Reset()
	t.Cleanup(viper.Reset)
	oldOpts, oldApproval := runOpts, approvalModeFlag
	t.Cleanup(func() { runOpts, approvalModeFlag = oldOpts, oldApproval })
	runOpts = sandbox.RunOptions{Image: "codex-dock:latest"}
	approvalModeFlag = "suggest"
	viper.Set("default_template", "pwn")
	viper.Set("run.model", "global-model")
	viper.Set("run.approval_mode", "full-auto")
	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().String("image", "", "")
	cmd.Flags().String("template", "", "")
	cmd.Flags().String("model", "", "")
	cmd.Flags().String("approval-mode", "", "")
	cmd.Flags().Bool("read-only", false, "")
	cmd.Flags().Bool("no-internet", false, "")
	cmd.Flags().Bool("watch-polling", false, "")
	applyRunConfigDefaults(cmd)
	applyProjectRunConfig(cmd, cfg)
	if runOpts.Image != "project:dev" || runOpts.Model != "project-model" || approvalModeFlag != "suggest" || !runOpts.WatchPolling || !runOpts.ReadOnly || !runOpts.NoInternet {
		t.Fatalf("project config did not override globals: opts=%+v approval=%q", runOpts, approvalModeFlag)
	}
	runOpts.Image = "flag:dev"
	runOpts.Model = "flag-model"
	approvalModeFlag = "auto-edit"
	if err := cmd.Flags().Set("image", "flag:dev"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("model", "flag-model"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("approval-mode", "auto-edit"); err != nil {
		t.Fatal(err)
	}
	applyProjectRunConfig(cmd, cfg)
	if runOpts.Image != "flag:dev" || runOpts.Model != "flag-model" || approvalModeFlag != "auto-edit" {
		t.Fatalf("project config overrode explicit flags: opts=%+v approval=%q", runOpts, approvalModeFlag)
	}
}

func TestProjectConfigRejectsHostSettingsAndMismatchedBuild(t *testing.T) {
	for _, tc := range []struct{ name, content, want string }{
		{"runtime", "[run]\nruntime = 'runc'\n", "unsupported project setting"},
		{"mounts", "[run]\nmounts = ['/home/user:/host']\n", "unsupported project setting"},
		{"proxy", "[proxy]\nadmin_secret = 'secret'\n", "host setting"},
		{"approval", "[run]\napproval_mode = 'danger'\n", "approval_mode"},
		{"writable", "[run]\nread_only = false\n", "read_only can only be true"},
		{"internet", "[run]\nno_internet = false\n", "no_internet can only be true"},
		{"mismatch", "[run]\nimage = 'other:dev'\n[build]\ntemplate = 'plain'\n", "must match"},
		{"escape", "[build]\ndockerfile = '../Dockerfile'\ntag = 'project:dev'\n", "inside the project"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeProjectConfig(t, dir, tc.content)
			_, err := loadProjectConfig(dir)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v; want %q", err, tc.want)
			}
		})
	}
}

func TestProjectBuildConfigUsesProjectDockerfile(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, "[run]\nimage = 'project:dev'\n[build]\ndockerfile = 'Dockerfile.codex-dock'\ntag = 'project:dev'\n")
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile.codex-dock"), []byte("FROM scratch\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	oldFile, oldTag, oldTemplate := buildDockerfile, buildTag, buildTemplate
	t.Cleanup(func() { buildDockerfile, buildTag, buildTemplate = oldFile, oldTag, oldTemplate })
	buildDockerfile, buildTag, buildTemplate = "", "codex-dock:latest", "plain"
	cmd := &cobra.Command{Use: "build"}
	cmd.Flags().String("dockerfile", "", "")
	cmd.Flags().String("template", "", "")
	cmd.Flags().String("tag", "", "")
	applyProjectBuildConfig(cmd, cfg)
	if buildDockerfile != filepath.Join(dir, "Dockerfile.codex-dock") || buildTag != "project:dev" || buildTemplate != "" {
		t.Fatalf("build settings = %q, %q, %q", buildDockerfile, buildTag, buildTemplate)
	}
}

func TestProjectDockerfileRecognizesPlatformFlagBeforeBase(t *testing.T) {
	data := []byte("# FROM codex-dock:latest\nFROM --platform=linux/amd64 codex-dock:latest AS base\nFROM scratch\n")
	if !projectDockerfileUsesBase(data, "codex-dock:latest") {
		t.Fatal("platform-qualified base image was not recognized")
	}
	if projectDockerfileUsesBase([]byte("FROM scratch\n"), "codex-dock:latest") {
		t.Fatal("unrelated base image was recognized")
	}
}
