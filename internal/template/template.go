package template

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	dockerdefaults "github.com/pacificbelt30/codex-dock/docker"
)

// Info describes a sandbox image template.
type Info struct {
	Name        string
	Dockerfile  []byte
	BaseImage   string
	IsBase      bool
	UserDefined bool // loaded from UserTemplatesDir rather than embedded in the binary
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

// UserTemplatesDir returns the directory where user-provided templates are
// stored: ~/.config/codex-dock/templates/<name>/Dockerfile. Templates placed
// there are picked up by Get and List alongside the built-in templates, and
// take precedence over a built-in template of the same name — useful for
// overriding a template locally or adding private ones without a rebuild.
func UserTemplatesDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "codex-dock", "templates"), nil
}

// readUserTemplate reads <UserTemplatesDir>/<name>/Dockerfile, if present.
func readUserTemplate(name string) ([]byte, bool) {
	dir, err := UserTemplatesDir()
	if err != nil {
		return nil, false
	}
	content, err := os.ReadFile(filepath.Join(dir, name, "Dockerfile"))
	if err != nil {
		return nil, false
	}
	return content, true
}

// Get returns the Info for a named template.
// "plain" or "" returns the built-in base template, unless a user template
// of the same name overrides it (see UserTemplatesDir).
func Get(name string) (Info, error) {
	if name == "" {
		name = "plain"
	}

	if content, ok := readUserTemplate(name); ok {
		return Info{
			Name:        name,
			Dockerfile:  content,
			BaseImage:   parseBaseImage(content),
			IsBase:      name == "plain",
			UserDefined: true,
		}, nil
	}

	if name == "plain" {
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

// List returns all available templates: built-in templates embedded in the
// binary plus any user-provided templates under UserTemplatesDir. A
// user-provided template overrides a built-in template of the same name.
func List() ([]Info, error) {
	byName := map[string]Info{}
	var order []string

	add := func(info Info) {
		if _, exists := byName[info.Name]; !exists {
			order = append(order, info.Name)
		}
		byName[info.Name] = info
	}

	add(plainTemplate())

	if entries, err := fs.ReadDir(dockerdefaults.Templates, "templates"); err == nil {
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
			add(Info{Name: name, Dockerfile: content, BaseImage: parseBaseImage(content)})
		}
	}

	if dir, err := UserTemplatesDir(); err == nil {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				name := entry.Name()
				content, err := os.ReadFile(filepath.Join(dir, name, "Dockerfile"))
				if err != nil {
					continue
				}
				add(Info{
					Name:        name,
					Dockerfile:  content,
					BaseImage:   parseBaseImage(content),
					IsBase:      name == "plain",
					UserDefined: true,
				})
			}
		}
	}

	templates := make([]Info, 0, len(order))
	for _, name := range order {
		templates = append(templates, byName[name])
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
