package template

import (
	"testing"
)

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
