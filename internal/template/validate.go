package template

import (
	"fmt"
	"strings"
)

// Requirement describes a single validation check for a Dockerfile.
type Requirement struct {
	Description string
	Check       func(content string) bool
}

// ValidationResult holds the outcome of validating a template.
type ValidationResult struct {
	TemplateName string
	Passed       []string
	Failed       []string
}

// IsValid returns true if all checks passed.
func (r ValidationResult) IsValid() bool {
	return len(r.Failed) == 0
}

var baseRequirements = []Requirement{
	{
		Description: "installs @openai/codex (npm)",
		Check: func(c string) bool {
			return strings.Contains(c, "@openai/codex")
		},
	},
	{
		Description: "installs @anthropic-ai/claude-code (npm)",
		Check: func(c string) bool {
			return strings.Contains(c, "@anthropic-ai/claude-code")
		},
	},
	{
		Description: "installs git",
		Check: func(c string) bool {
			return containsAptPkg(c, "git")
		},
	},
	{
		Description: "installs curl",
		Check: func(c string) bool {
			return containsAptPkg(c, "curl")
		},
	},
	{
		Description: "creates non-root user",
		Check: func(c string) bool {
			return strings.Contains(c, "useradd") || strings.Contains(c, "adduser")
		},
	},
	{
		Description: "switches to non-root USER",
		Check: func(c string) bool {
			for _, line := range strings.Split(c, "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(strings.ToUpper(trimmed), "USER ") {
					user := strings.TrimSpace(trimmed[5:])
					if user != "root" && user != "" {
						return true
					}
				}
			}
			return false
		},
	},
	{
		Description: "copies entrypoint.sh",
		Check: func(c string) bool {
			return strings.Contains(c, "entrypoint.sh")
		},
	},
}

// Validate performs static analysis on a template's Dockerfile content.
// Derived templates (FROM codex-dock:*) trust the base and skip checks.
func Validate(tmpl Info) ValidationResult {
	result := ValidationResult{TemplateName: tmpl.Name}

	if tmpl.IsDerived() {
		desc := fmt.Sprintf("FROM references codex-dock base (%s)", tmpl.BaseImage)
		result.Passed = append(result.Passed, desc)
		return result
	}

	content := string(tmpl.Dockerfile)
	for _, req := range baseRequirements {
		if req.Check(content) {
			result.Passed = append(result.Passed, req.Description)
		} else {
			result.Failed = append(result.Failed, req.Description)
		}
	}
	return result
}

// containsAptPkg checks whether a Dockerfile contains the given package name
// as a standalone field in any line.
func containsAptPkg(content, pkg string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimSuffix(trimmed, "\\")
		trimmed = strings.TrimSpace(trimmed)
		for _, f := range strings.Fields(trimmed) {
			if f == pkg {
				return true
			}
		}
	}
	return false
}
