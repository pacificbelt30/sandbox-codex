package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// TestResolveDockerfile_ExplicitFlag verifies that an explicit -f value is
// returned as-is with "." as the build context.
func TestResolveDockerfile_ExplicitFlag(t *testing.T) {
	df, ctx, err := resolveDockerfile("/custom/path/Dockerfile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if df != "/custom/path/Dockerfile" {
		t.Errorf("dockerfile = %q; want /custom/path/Dockerfile", df)
	}
	if ctx != "." {
		t.Errorf("buildCtx = %q; want .", ctx)
	}
}

// TestResolveDockerfile_CWDDockerfile verifies that a Dockerfile in the current
// directory is detected, with "." as the build context.
func TestResolveDockerfile_CWDDockerfile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	df, ctx, err := resolveDockerfile("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if df != "Dockerfile" {
		t.Errorf("dockerfile = %q; want Dockerfile", df)
	}
	if ctx != "." {
		t.Errorf("buildCtx = %q; want .", ctx)
	}
}

// TestResolveDockerfile_DockerSubdir verifies that docker/Dockerfile in the
// current directory is detected, with "docker" as the build context.
func TestResolveDockerfile_DockerSubdir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docker"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docker", "Dockerfile"), []byte("FROM scratch\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	df, ctx, err := resolveDockerfile("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if df != "docker/Dockerfile" {
		t.Errorf("dockerfile = %q; want docker/Dockerfile", df)
	}
	if ctx != "docker" {
		t.Errorf("buildCtx = %q; want docker", ctx)
	}
}

// TestResolveDockerfile_FallbackToConfigDir verifies that when no Dockerfile
// exists in the current directory, the embedded default is written to the
// config dir and that path is returned.
func TestResolveDockerfile_FallbackToConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	df, ctx, err := resolveDockerfile("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedDir := filepath.Join(homeDir, ".config", "codex-dock")
	if ctx != expectedDir {
		t.Errorf("buildCtx = %q; want %q", ctx, expectedDir)
	}
	if !strings.HasSuffix(df, "Dockerfile") {
		t.Errorf("dockerfile = %q; expected to end with Dockerfile", df)
	}

	// Both files should have been written to the config dir.
	for _, name := range []string{"Dockerfile", "entrypoint.sh"} {
		if _, err := os.Stat(filepath.Join(expectedDir, name)); err != nil {
			t.Errorf("%s not written to config dir: %v", name, err)
		}
	}
}

// TestEnsureDefaultDockerfile_CreatesFiles verifies that Dockerfile and
// entrypoint.sh are written with non-zero content.
func TestEnsureDefaultDockerfile_CreatesFiles(t *testing.T) {
	dir := t.TempDir()
	if err := ensureDefaultDockerfile(dir); err != nil {
		t.Fatalf("ensureDefaultDockerfile: %v", err)
	}

	for _, name := range []string{"Dockerfile", "entrypoint.sh"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("%s not created: %v", name, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty", name)
		}
	}
}

// TestEnsureDefaultDockerfile_Idempotent verifies that calling
// ensureDefaultDockerfile twice does not overwrite existing files.
func TestEnsureDefaultDockerfile_Idempotent(t *testing.T) {
	dir := t.TempDir()
	if err := ensureDefaultDockerfile(dir); err != nil {
		t.Fatalf("first call: %v", err)
	}

	const customContent = "# custom Dockerfile\n"
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(customContent), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ensureDefaultDockerfile(dir); err != nil {
		t.Fatalf("second call: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != customContent {
		t.Errorf("existing Dockerfile was overwritten; got %q; want %q", string(data), customContent)
	}
}

// TestEnsureDefaultDockerfile_DockerfileContent verifies that the embedded
// Dockerfile contains expected markers.
func TestEnsureDefaultDockerfile_DockerfileContent(t *testing.T) {
	dir := t.TempDir()
	if err := ensureDefaultDockerfile(dir); err != nil {
		t.Fatalf("ensureDefaultDockerfile: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, marker := range []string{"FROM node:", "COPY entrypoint.sh", "USER codex"} {
		if !strings.Contains(content, marker) {
			t.Errorf("Dockerfile missing expected content %q", marker)
		}
	}
}

func TestApplyBuildConfigDefaults_GlobalTemplate(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prev := buildTemplate
	t.Cleanup(func() { buildTemplate = prev })

	buildTemplate = ""
	viper.Set("default_template", "pwn")

	cmd := &cobra.Command{Use: "build"}
	cmd.Flags().String("template", "", "")
	cmd.Flags().String("tag", "codex-dock:latest", "")

	applyBuildConfigDefaults(cmd)

	if buildTemplate != "pwn" {
		t.Errorf("buildTemplate = %q; want pwn", buildTemplate)
	}
}

func TestApplyBuildConfigDefaults_SectionOverridesGlobal(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prev := buildTemplate
	t.Cleanup(func() { buildTemplate = prev })

	buildTemplate = ""
	viper.Set("default_template", "plain")
	viper.Set("build.template", "pwn")

	cmd := &cobra.Command{Use: "build"}
	cmd.Flags().String("template", "", "")
	cmd.Flags().String("tag", "codex-dock:latest", "")

	applyBuildConfigDefaults(cmd)

	if buildTemplate != "pwn" {
		t.Errorf("buildTemplate = %q; want pwn (build.template overrides default_template)", buildTemplate)
	}
}

func TestApplyBuildConfigDefaults_FlagOverridesConfig(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prev := buildTemplate
	t.Cleanup(func() { buildTemplate = prev })

	buildTemplate = "plain"
	viper.Set("build.template", "pwn")

	cmd := &cobra.Command{Use: "build"}
	cmd.Flags().String("template", "", "")
	cmd.Flags().String("tag", "codex-dock:latest", "")

	if err := cmd.Flags().Set("template", "plain"); err != nil {
		t.Fatalf("set template: %v", err)
	}

	applyBuildConfigDefaults(cmd)

	if buildTemplate != "plain" {
		t.Errorf("buildTemplate = %q; want plain (flag overrides config)", buildTemplate)
	}
}

func TestApplyBuildConfigDefaults_Tag(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prev := buildTag
	t.Cleanup(func() { buildTag = prev })

	buildTag = "codex-dock:latest"
	viper.Set("build.tag", "custom:v1")

	cmd := &cobra.Command{Use: "build"}
	cmd.Flags().String("template", "", "")
	cmd.Flags().String("tag", "codex-dock:latest", "")

	applyBuildConfigDefaults(cmd)

	if buildTag != "custom:v1" {
		t.Errorf("buildTag = %q; want custom:v1", buildTag)
	}
}
