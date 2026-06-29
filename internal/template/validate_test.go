package template

import (
	"strings"
	"testing"
)

func TestValidatePlain(t *testing.T) {
	tmpl := plainTemplate()
	result := Validate(tmpl)
	if !result.IsValid() {
		t.Errorf("plain template failed validation: %v", result.Failed)
	}
	if len(result.Passed) != len(baseRequirements) {
		t.Errorf("passed %d checks; want %d", len(result.Passed), len(baseRequirements))
	}
}

func TestValidateDerived(t *testing.T) {
	tmpl := Info{
		Name:       "test-derived",
		Dockerfile: []byte("FROM codex-dock:latest\nRUN echo hello\n"),
		BaseImage:  "codex-dock:latest",
	}
	result := Validate(tmpl)
	if !result.IsValid() {
		t.Errorf("derived template should pass: %v", result.Failed)
	}
	if len(result.Passed) != 1 {
		t.Errorf("passed %d checks; want 1", len(result.Passed))
	}
}

func TestValidatePwn(t *testing.T) {
	tmpl, err := Get("pwn")
	if err != nil {
		t.Fatalf("Get(pwn): %v", err)
	}
	result := Validate(tmpl)
	if !result.IsValid() {
		t.Errorf("pwn template failed validation: %v", result.Failed)
	}
}

func TestValidateMissingCodex(t *testing.T) {
	tmpl := Info{
		Name: "bad",
		Dockerfile: []byte(`FROM node:22-slim
RUN apt-get update && apt-get install -y git curl
RUN npm install -g @anthropic-ai/claude-code
RUN useradd -m codex
COPY entrypoint.sh /entrypoint.sh
USER codex
`),
		IsBase: true,
	}
	result := Validate(tmpl)
	if result.IsValid() {
		t.Error("should fail when @openai/codex is missing")
	}
	if !containsFailure(result, "@openai/codex") {
		t.Errorf("expected failure about @openai/codex; got %v", result.Failed)
	}
}

func TestValidateMissingClaude(t *testing.T) {
	tmpl := Info{
		Name: "bad",
		Dockerfile: []byte(`FROM node:22-slim
RUN apt-get update && apt-get install -y git curl
RUN npm install -g @openai/codex
RUN useradd -m codex
COPY entrypoint.sh /entrypoint.sh
USER codex
`),
		IsBase: true,
	}
	result := Validate(tmpl)
	if result.IsValid() {
		t.Error("should fail when claude-code is missing")
	}
	if !containsFailure(result, "claude-code") {
		t.Errorf("expected failure about claude-code; got %v", result.Failed)
	}
}

func TestValidateMissingGit(t *testing.T) {
	tmpl := Info{
		Name: "bad",
		Dockerfile: []byte(`FROM node:22-slim
RUN apt-get update && apt-get install -y curl
RUN npm install -g @openai/codex @anthropic-ai/claude-code
RUN useradd -m codex
COPY entrypoint.sh /entrypoint.sh
USER codex
`),
		IsBase: true,
	}
	result := Validate(tmpl)
	if result.IsValid() {
		t.Error("should fail when git is missing")
	}
	if !containsFailure(result, "git") {
		t.Errorf("expected failure about git; got %v", result.Failed)
	}
}

func TestValidateMissingUser(t *testing.T) {
	tmpl := Info{
		Name: "bad",
		Dockerfile: []byte(`FROM node:22-slim
RUN apt-get update && apt-get install -y git curl
RUN npm install -g @openai/codex @anthropic-ai/claude-code
COPY entrypoint.sh /entrypoint.sh
USER root
`),
		IsBase: true,
	}
	result := Validate(tmpl)
	if result.IsValid() {
		t.Error("should fail when non-root user is missing")
	}
}

func TestValidateMissingEntrypoint(t *testing.T) {
	tmpl := Info{
		Name: "bad",
		Dockerfile: []byte(`FROM node:22-slim
RUN apt-get update && apt-get install -y git curl
RUN npm install -g @openai/codex @anthropic-ai/claude-code
RUN useradd -m codex
USER codex
`),
		IsBase: true,
	}
	result := Validate(tmpl)
	if result.IsValid() {
		t.Error("should fail when entrypoint.sh is missing")
	}
	if !containsFailure(result, "entrypoint") {
		t.Errorf("expected failure about entrypoint; got %v", result.Failed)
	}
}

func TestContainsAptPkg(t *testing.T) {
	tests := []struct {
		name    string
		content string
		pkg     string
		want    bool
	}{
		{
			name:    "simple line",
			content: "RUN apt-get install -y git curl",
			pkg:     "git",
			want:    true,
		},
		{
			name:    "continuation",
			content: "    git \\",
			pkg:     "git",
			want:    true,
		},
		{
			name:    "not present",
			content: "RUN apt-get install -y curl",
			pkg:     "git",
			want:    false,
		},
		{
			name:    "substring mismatch",
			content: "RUN apt-get install -y git-lfs",
			pkg:     "git",
			want:    false,
		},
		{
			name:    "multiline",
			content: "RUN apt-get install -y \\\n    git \\\n    curl",
			pkg:     "curl",
			want:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsAptPkg(tt.content, tt.pkg); got != tt.want {
				t.Errorf("containsAptPkg(%q, %q) = %v; want %v", tt.content, tt.pkg, got, tt.want)
			}
		})
	}
}

func containsFailure(result ValidationResult, substr string) bool {
	for _, f := range result.Failed {
		if strings.Contains(f, substr) {
			return true
		}
	}
	return false
}
