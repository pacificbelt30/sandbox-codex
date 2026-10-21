// Package proxysettings edits only non-secret network policy settings. It never
// restarts proxies or changes the policy of an existing worker.
package proxysettings

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/pacificbelt30/codex-dock/internal/networkpolicy"
	"github.com/pacificbelt30/codex-dock/internal/workeregress"
	"github.com/pelletier/go-toml/v2"
)

var ErrConflict = errors.New("configuration changed; reload before saving")

type Settings struct {
	DefaultDeny   bool     `toml:"forward_default_deny" json:"default_deny"`
	UpstreamProxy string   `toml:"upstream_proxy" json:"upstream_proxy"`
	CAFile        string   `toml:"ca_bundle" json:"ca_bundle"`
	AllowDomains  []string `toml:"forward_allow_domains" json:"allow_domains"`
	DenyDomains   []string `toml:"forward_deny_domains" json:"deny_domains"`
	AllowCIDRs    []string `toml:"forward_allow_cidrs" json:"allow_cidrs"`
	DenyCIDRs     []string `toml:"forward_deny_cidrs" json:"deny_cidrs"`
	AllowPorts    []int    `toml:"forward_allow_ports" json:"allow_ports"`
	DenyPorts     []int    `toml:"forward_deny_ports" json:"deny_ports"`
}

func (s Settings) Rules() networkpolicy.Rules {
	return networkpolicy.Rules{AllowDomains: s.AllowDomains, DenyDomains: s.DenyDomains, AllowCIDRs: s.AllowCIDRs, DenyCIDRs: s.DenyCIDRs, AllowPorts: s.AllowPorts, DenyPorts: s.DenyPorts, BlockPrivate: true, DefaultDeny: s.DefaultDeny}
}
func FromRules(r networkpolicy.Rules) Settings {
	return Settings{AllowDomains: r.AllowDomains, DenyDomains: r.DenyDomains, AllowCIDRs: r.AllowCIDRs, DenyCIDRs: r.DenyCIDRs, AllowPorts: r.AllowPorts, DenyPorts: r.DenyPorts, DefaultDeny: r.DefaultDeny}
}
func (s Settings) Validate() error {
	if s.UpstreamProxy != "" {
		u, err := url.Parse(s.UpstreamProxy)
		if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" || strings.Trim(u.Path, "/") != "" {
			return fmt.Errorf("upstream proxy must be an HTTP(S) origin without credentials")
		}
	}
	if s.CAFile != "" && (!filepath.IsAbs(s.CAFile) || strings.ContainsAny(s.CAFile, "\r\n\x00")) {
		return fmt.Errorf("CA bundle must use an absolute host path")
	}
	_, err := networkpolicy.Compile(s.Rules())
	return err
}
func (s Settings) PolicyID() string { return workeregress.ID(s.Rules()) }

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "codex-dock", "config.toml"), nil
}
func revision(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// Load exposes only the policy allow/deny lists, never other TOML values.
func Load(path string) (Settings, string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Settings{}, revision(nil), nil
	}
	if err != nil {
		return Settings{}, "", err
	}
	var document struct {
		Proxy Settings `toml:"proxy"`
	}
	if err := toml.Unmarshal(data, &document); err != nil {
		return Settings{}, "", fmt.Errorf("parsing proxy configuration: %w", err)
	}
	return document.Proxy, revision(data), document.Proxy.Validate()
}

// Save checks an optimistic revision, preserves unrelated content and comments,
// and replaces the file atomically. Secrets are never rendered into the UI.
func Save(path, expected string, settings Settings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("config path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".cxd.lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	info, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("config must be a regular file, not a symlink")
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if revision(data) != expected {
		return ErrConflict
	}
	// Parse first: do not modify an invalid existing file.
	var existing map[string]any
	if len(data) > 0 {
		if err := toml.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("existing configuration is invalid: %w", err)
		}
	}
	fields := reflect.TypeOf(settings)
	values := reflect.ValueOf(settings)
	changes := map[string]string{}
	for i := 0; i < fields.NumField(); i++ {
		key := fields.Field(i).Tag.Get("toml")
		encoded, err := toml.Marshal(map[string]any{key: values.Field(i).Interface()})
		if err != nil {
			return err
		}
		changes[key] = strings.TrimSpace(string(encoded))
	}
	lines := strings.Split(string(data), "\n")
	out := []string{}
	inProxy := false
	seenProxy := false
	written := map[string]bool{}
	appendMissing := func() {
		for i := 0; i < fields.NumField(); i++ {
			key := fields.Field(i).Tag.Get("toml")
			if !written[key] {
				out = append(out, changes[key])
				written[key] = true
			}
		}
	}
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") {
			if inProxy {
				appendMissing()
			}
			inProxy = trim == "[proxy]" || strings.HasPrefix(trim, "[proxy] #")
			if inProxy {
				if seenProxy {
					return fmt.Errorf("duplicate proxy section")
				}
				seenProxy = true
			}
		}
		if inProxy {
			parts := strings.SplitN(trim, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				if replacement, ok := changes[key]; ok {
					// Multiline arrays are deliberately rejected rather than partially edited.
					if strings.Contains(parts[1], "[") && !strings.Contains(parts[1], "]") {
						return fmt.Errorf("edit multiline proxy field %s in a text editor before using this UI", key)
					}
					out = append(out, replacement)
					written[key] = true
					continue
				}
			}
		}
		out = append(out, line)
	}
	if !seenProxy {
		out = append(out, "", "[proxy]")
	}
	appendMissing()
	result := []byte(strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n")
	var verify struct {
		Proxy Settings `toml:"proxy"`
	}
	if err := toml.Unmarshal(result, &verify); err != nil {
		return fmt.Errorf("refusing invalid resulting configuration: %w", err)
	}
	if verify.Proxy.DefaultDeny != settings.DefaultDeny || verify.Proxy.UpstreamProxy != settings.UpstreamProxy || verify.Proxy.CAFile != settings.CAFile || !slices.Equal(verify.Proxy.AllowDomains, settings.AllowDomains) || !slices.Equal(verify.Proxy.DenyDomains, settings.DenyDomains) || !slices.Equal(verify.Proxy.AllowCIDRs, settings.AllowCIDRs) || !slices.Equal(verify.Proxy.DenyCIDRs, settings.DenyCIDRs) || !slices.Equal(verify.Proxy.AllowPorts, settings.AllowPorts) || !slices.Equal(verify.Proxy.DenyPorts, settings.DenyPorts) {
		return fmt.Errorf("could not safely update all proxy fields")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".cxd-policy-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err = f.Write(result); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	current, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if revision(current) != expected {
		return ErrConflict
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}

func ParsePorts(value string) ([]int, error) {
	var ports []int
	for _, part := range Split(value) {
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("invalid port %q", part)
		}
		ports = append(ports, n)
	}
	return ports, nil
}
func Split(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' ' })
}
