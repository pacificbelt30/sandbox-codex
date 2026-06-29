package template

import (
	"fmt"
	"io/fs"
	"path"
	"strings"

	dockerdefaults "github.com/pacificbelt30/codex-dock/docker"
)

// Info describes a sandbox image template.
type Info struct {
	Name       string
	Dockerfile []byte
	BaseImage  string
	IsBase     bool
}

// Tag returns the Docker image tag for this template.
// plain -> "codex-dock:latest", others -> "codex-dock:<name>".
// If customTag is non-empty it is returned instead.
func (t Info) Tag(customTag string) string {
	if customTag != "" {
		return customTag
	}
	if t.IsBase {
		return "codex-dock:latest"
	}
	return "codex-dock:" + t.Name
}

// IsDerived returns true if the template depends on another codex-dock image.
func (t Info) IsDerived() bool {
	return t.BaseImage != ""
}

// Get returns the Info for a named template.
// "plain" or "" returns the built-in base template.
func Get(name string) (Info, error) {
	if name == "plain" || name == "" {
		return plainTemplate(), nil
	}

	dfPath := path.Join("templates", name, "Dockerfile")
	content, err := fs.ReadFile(dockerdefaults.Templates, dfPath)
	if err != nil {
		return Info{}, fmt.Errorf("template %q not found", name)
	}

	return Info{
		Name:       name,
		Dockerfile: content,
		BaseImage:  parseBaseImage(content),
	}, nil
}

// List returns all available templates by scanning the embedded FS.
func List() ([]Info, error) {
	templates := []Info{plainTemplate()}

	entries, err := fs.ReadDir(dockerdefaults.Templates, "templates")
	if err != nil {
		return templates, nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		dfPath := path.Join("templates", name, "Dockerfile")
		content, err := fs.ReadFile(dockerdefaults.Templates, dfPath)
		if err != nil {
			continue
		}
		templates = append(templates, Info{
			Name:       name,
			Dockerfile: content,
			BaseImage:  parseBaseImage(content),
		})
	}

	return templates, nil
}

// MatchTag returns the template name if tag matches "codex-dock:<name>" and a
// template with that name exists. Returns "" if no match.
func MatchTag(tag string) string {
	if tag == "codex-dock:latest" || tag == "codex-dock" {
		return "plain"
	}
	const prefix = "codex-dock:"
	if !strings.HasPrefix(tag, prefix) {
		return ""
	}
	name := tag[len(prefix):]
	if _, err := Get(name); err == nil {
		return name
	}
	return ""
}

func plainTemplate() Info {
	return Info{
		Name:       "plain",
		Dockerfile: dockerdefaults.Dockerfile,
		IsBase:     true,
	}
}

// parseBaseImage extracts the image from the first FROM instruction.
// Returns "" if the FROM does not reference a codex-dock image.
func parseBaseImage(dockerfile []byte) string {
	for _, line := range strings.Split(string(dockerfile), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(trimmed), "FROM ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				img := parts[1]
				if strings.HasPrefix(img, "codex-dock:") || img == "codex-dock" {
					return img
				}
			}
			return ""
		}
	}
	return ""
}
