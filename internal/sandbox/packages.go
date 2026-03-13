package sandbox

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Package represents a package to install with its manager.
type Package struct {
	Manager string // apt, pip, npm, or auto
	Name    string
}

// ParsePackage parses a package spec like "apt:libssl-dev" or "pwntools".
// When no manager prefix is given, detectManager is used to infer the appropriate
// package manager (F-PKG-05).
func ParsePackage(spec string) Package {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) == 2 {
		return Package{Manager: strings.ToLower(parts[0]), Name: parts[1]}
	}
	return Package{Manager: detectManager(spec), Name: spec}
}

// detectManager infers the package manager for a package name that has no explicit prefix.
// Rules (in order):
//  1. Names starting with "@" are npm scoped packages (e.g. @types/node).
//  2. Names containing PEP 508 version specifiers (==, >=, <=, ~=, !=) are pip packages.
//  3. Everything else defaults to apt.
func detectManager(name string) string {
	// npm scoped packages always start with @
	if strings.HasPrefix(name, "@") {
		return "npm"
	}
	// pip version specifiers per PEP 508
	for _, op := range []string{"==", ">=", "<=", "~=", "!="} {
		if strings.Contains(name, op) {
			return "pip"
		}
	}
	// Default: treat as apt (system package)
	return "apt"
}

// LoadPackageFile reads packages from a packages.dock file.
func LoadPackageFile(path string) ([]Package, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening package file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var pkgs []Package
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip inline comment
		if idx := strings.Index(line, " #"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}
		pkgs = append(pkgs, ParsePackage(line))
	}
	return pkgs, scanner.Err()
}

// BuildInstallScript generates a shell script to install the given packages.
func BuildInstallScript(pkgs []Package) string {
	if len(pkgs) == 0 {
		return ""
	}

	var apt, pip, npm []string
	for _, p := range pkgs {
		switch p.Manager {
		case "apt":
			apt = append(apt, p.Name)
		case "pip":
			pip = append(pip, p.Name)
		case "npm":
			npm = append(npm, p.Name)
		default:
			// auto: try apt first (simplest heuristic)
			apt = append(apt, p.Name)
		}
	}

	var sb strings.Builder
	sb.WriteString("set -e\n")
	if len(apt) > 0 {
		sb.WriteString("apt-get update -qq && apt-get install -y --no-install-recommends ")
		sb.WriteString(strings.Join(apt, " "))
		sb.WriteString(" && rm -rf /var/lib/apt/lists/*\n")
	}
	if len(pip) > 0 {
		sb.WriteString("pip3 install --quiet ")
		sb.WriteString(strings.Join(pip, " "))
		sb.WriteString("\n")
	}
	if len(npm) > 0 {
		sb.WriteString("npm install -g --quiet ")
		sb.WriteString(strings.Join(npm, " "))
		sb.WriteString("\n")
	}
	return sb.String()
}
