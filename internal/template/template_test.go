package template

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupUserTemplate creates <home>/.config/codex-dock/templates/<name>/Dockerfile
// with the given content and points HOME at it for the duration of the test.
func setupUserTemplate(t *testing.T, name, content string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".config", "codex-dock", "templates", name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return home
}

func TestGetPlain(t *testing.T) {
	tmpl, err := Get("plain")
	if err != nil {
		t.Fatalf("Get(plain): %v", err)
	}
	if tmpl.Name != "plain" {
		t.Errorf("Name = %q; want plain", tmpl.Name)
	}
	if !tmpl.IsBase {
		t.Error("IsBase = false; want true")
	}
	if tmpl.IsDerived() {
		t.Error("IsDerived() = true; want false")
	}
	if len(tmpl.Dockerfile) == 0 {
		t.Error("Dockerfile is empty")
	}
}

func TestGetEmptyName(t *testing.T) {
	tmpl, err := Get("")
	if err != nil {
		t.Fatalf("Get(''): %v", err)
	}
	if tmpl.Name != "plain" {
		t.Errorf("Name = %q; want plain", tmpl.Name)
	}
}

func TestGetPwn(t *testing.T) {
	tmpl, err := Get("pwn")
	if err != nil {
		t.Fatalf("Get(pwn): %v", err)
	}
	if tmpl.Name != "pwn" {
		t.Errorf("Name = %q; want pwn", tmpl.Name)
	}
	if tmpl.IsBase {
		t.Error("IsBase = true; want false")
	}
	if !tmpl.IsDerived() {
		t.Error("IsDerived() = false; want true")
	}
	if tmpl.BaseImage != "codex-dock:latest" {
		t.Errorf("BaseImage = %q; want codex-dock:latest", tmpl.BaseImage)
	}
}

func TestGetUnknown(t *testing.T) {
	_, err := Get("nonexistent")
	if err == nil {
		t.Fatal("Get(nonexistent) should return an error")
	}
}

func TestList(t *testing.T) {
	templates, err := List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(templates) < 2 {
		t.Fatalf("List() returned %d templates; want at least 2", len(templates))
	}

	names := make(map[string]bool)
	for _, tmpl := range templates {
		names[tmpl.Name] = true
		if len(tmpl.Dockerfile) == 0 {
			t.Errorf("template %q has empty Dockerfile", tmpl.Name)
		}
	}
	for _, want := range []string{"plain", "pwn"} {
		if !names[want] {
			t.Errorf("List() missing template %q", want)
		}
	}
}

func TestTagPlainDefault(t *testing.T) {
	tmpl := plainTemplate()
	if got := tmpl.Tag(""); got != "codex-dock:latest" {
		t.Errorf("Tag('') = %q; want codex-dock:latest", got)
	}
}

func TestTagPwnDefault(t *testing.T) {
	tmpl, _ := Get("pwn")
	if got := tmpl.Tag(""); got != "codex-dock:pwn" {
		t.Errorf("Tag('') = %q; want codex-dock:pwn", got)
	}
}

func TestTagCustomOverride(t *testing.T) {
	tmpl := plainTemplate()
	if got := tmpl.Tag("my-image:v1"); got != "my-image:v1" {
		t.Errorf("Tag('my-image:v1') = %q; want my-image:v1", got)
	}
}

func TestParseBaseImage_CodexDock(t *testing.T) {
	df := []byte("FROM codex-dock:latest\nRUN echo hello\n")
	if got := parseBaseImage(df); got != "codex-dock:latest" {
		t.Errorf("parseBaseImage = %q; want codex-dock:latest", got)
	}
}

func TestParseBaseImage_External(t *testing.T) {
	df := []byte("FROM node:22-slim\nRUN echo hello\n")
	if got := parseBaseImage(df); got != "" {
		t.Errorf("parseBaseImage = %q; want empty", got)
	}
}

func TestParseBaseImage_CodexDockBare(t *testing.T) {
	df := []byte("FROM codex-dock\nRUN echo hello\n")
	if got := parseBaseImage(df); got != "codex-dock" {
		t.Errorf("parseBaseImage = %q; want codex-dock", got)
	}
}

func TestParseBaseImage_Empty(t *testing.T) {
	df := []byte("")
	if got := parseBaseImage(df); got != "" {
		t.Errorf("parseBaseImage = %q; want empty", got)
	}
}

func TestMatchTag_Latest(t *testing.T) {
	if got := MatchTag("codex-dock:latest"); got != "plain" {
		t.Errorf("MatchTag(codex-dock:latest) = %q; want plain", got)
	}
}

func TestMatchTag_Bare(t *testing.T) {
	if got := MatchTag("codex-dock"); got != "plain" {
		t.Errorf("MatchTag(codex-dock) = %q; want plain", got)
	}
}

func TestMatchTag_Pwn(t *testing.T) {
	if got := MatchTag("codex-dock:pwn"); got != "pwn" {
		t.Errorf("MatchTag(codex-dock:pwn) = %q; want pwn", got)
	}
}

func TestMatchTag_Unknown(t *testing.T) {
	if got := MatchTag("codex-dock:nosuch"); got != "" {
		t.Errorf("MatchTag(codex-dock:nosuch) = %q; want empty", got)
	}
}

func TestMatchTag_Unrelated(t *testing.T) {
	if got := MatchTag("ubuntu:latest"); got != "" {
		t.Errorf("MatchTag(ubuntu:latest) = %q; want empty", got)
	}
}

func TestUserTemplatesDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir, err := UserTemplatesDir()
	if err != nil {
		t.Fatalf("UserTemplatesDir: %v", err)
	}
	want := filepath.Join(home, ".config", "codex-dock", "templates")
	if dir != want {
		t.Errorf("UserTemplatesDir() = %q; want %q", dir, want)
	}
}

func TestGet_UserTemplateNew(t *testing.T) {
	setupUserTemplate(t, "custom", "FROM codex-dock:latest\nRUN echo custom\n")

	tmpl, err := Get("custom")
	if err != nil {
		t.Fatalf("Get(custom): %v", err)
	}
	if !tmpl.UserDefined {
		t.Error("UserDefined = false; want true")
	}
	if !tmpl.IsDerived() {
		t.Error("IsDerived() = false; want true")
	}
	if tmpl.BaseImage != "codex-dock:latest" {
		t.Errorf("BaseImage = %q; want codex-dock:latest", tmpl.BaseImage)
	}
}

func TestGet_UserTemplateOverridesBuiltin(t *testing.T) {
	setupUserTemplate(t, "pwn", "FROM codex-dock:latest\nRUN echo overridden-pwn\n")

	tmpl, err := Get("pwn")
	if err != nil {
		t.Fatalf("Get(pwn): %v", err)
	}
	if !tmpl.UserDefined {
		t.Error("UserDefined = false; want true")
	}
	if string(tmpl.Dockerfile) != "FROM codex-dock:latest\nRUN echo overridden-pwn\n" {
		t.Errorf("Dockerfile = %q; want user override content", tmpl.Dockerfile)
	}
}

func TestGet_NoUserTemplate_FallsBackToBuiltin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	tmpl, err := Get("pwn")
	if err != nil {
		t.Fatalf("Get(pwn): %v", err)
	}
	if tmpl.UserDefined {
		t.Error("UserDefined = true; want false (no user override present)")
	}
}

func TestList_IncludesUserTemplates(t *testing.T) {
	setupUserTemplate(t, "custom", "FROM codex-dock:latest\nRUN echo custom\n")

	templates, err := List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}

	var found *Info
	for i := range templates {
		if templates[i].Name == "custom" {
			found = &templates[i]
		}
	}
	if found == nil {
		t.Fatal("List() missing user-defined template \"custom\"")
	}
	if !found.UserDefined {
		t.Error("UserDefined = false; want true")
	}
}

func TestList_UserTemplateOverridesBuiltinEntry(t *testing.T) {
	setupUserTemplate(t, "pwn", "FROM codex-dock:latest\nRUN echo overridden-pwn\n")

	templates, err := List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}

	count := 0
	for _, tmpl := range templates {
		if tmpl.Name != "pwn" {
			continue
		}
		count++
		if !tmpl.UserDefined {
			t.Error("UserDefined = false; want true")
		}
		if string(tmpl.Dockerfile) != "FROM codex-dock:latest\nRUN echo overridden-pwn\n" {
			t.Errorf("Dockerfile = %q; want user override content", tmpl.Dockerfile)
		}
	}
	if count != 1 {
		t.Errorf("found %d entries named %q; want exactly 1 (no duplicate)", count, "pwn")
	}
}

func TestGet_UserTemplate_SetsSourcePath(t *testing.T) {
	home := setupUserTemplate(t, "custom", "FROM codex-dock:latest\n")

	tmpl, err := Get("custom")
	if err != nil {
		t.Fatalf("Get(custom): %v", err)
	}
	want := filepath.Join(home, ".config", "codex-dock", "templates", "custom", "Dockerfile")
	if tmpl.SourcePath != want {
		t.Errorf("SourcePath = %q; want %q", tmpl.SourcePath, want)
	}
}

func TestGet_BuiltinTemplate_NoSourcePath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	tmpl, err := Get("pwn")
	if err != nil {
		t.Fatalf("Get(pwn): %v", err)
	}
	if tmpl.SourcePath != "" {
		t.Errorf("SourcePath = %q; want empty for a binary-embedded template", tmpl.SourcePath)
	}
}

func TestStaleAgainst_NoSourcePath(t *testing.T) {
	tmpl := plainTemplate()
	stale, err := tmpl.StaleAgainst(time.Now())
	if err != nil {
		t.Fatalf("StaleAgainst: %v", err)
	}
	if stale {
		t.Error("StaleAgainst = true; want false when there is no SourcePath to compare")
	}
}

func TestStaleAgainst_ModifiedAfterBuild(t *testing.T) {
	setupUserTemplate(t, "custom", "FROM codex-dock:latest\n")
	tmpl, err := Get("custom")
	if err != nil {
		t.Fatalf("Get(custom): %v", err)
	}

	builtAt := time.Now().Add(-1 * time.Hour)
	stale, err := tmpl.StaleAgainst(builtAt)
	if err != nil {
		t.Fatalf("StaleAgainst: %v", err)
	}
	if !stale {
		t.Error("StaleAgainst = false; want true when the file was modified after builtAt")
	}
}

func TestStaleAgainst_NotModifiedSinceBuild(t *testing.T) {
	setupUserTemplate(t, "custom", "FROM codex-dock:latest\n")
	tmpl, err := Get("custom")
	if err != nil {
		t.Fatalf("Get(custom): %v", err)
	}

	builtAt := time.Now().Add(1 * time.Hour)
	stale, err := tmpl.StaleAgainst(builtAt)
	if err != nil {
		t.Fatalf("StaleAgainst: %v", err)
	}
	if stale {
		t.Error("StaleAgainst = true; want false when the file predates builtAt")
	}
}

func TestStaleAgainst_MissingFile(t *testing.T) {
	tmpl := Info{SourcePath: filepath.Join(t.TempDir(), "does-not-exist", "Dockerfile")}
	if _, err := tmpl.StaleAgainst(time.Now()); err == nil {
		t.Error("StaleAgainst should return an error when SourcePath does not exist")
	}
}
