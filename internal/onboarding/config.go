// Package onboarding prepares independent login settings without replacing
// unrelated configuration, comments or network restrictions.
package onboarding

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

// SelectProfile changes only auth_profile and, when absent, the admin-secret
// file setting. It returns validated TOML and never writes credentials.
func SelectProfile(data []byte, profile, secretFile string) ([]byte, error) {
	var config map[string]any
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("invalid existing configuration")
	}
	quote := func(value string) string {
		encoded, _ := toml.Marshal(map[string]string{"value": value})
		return strings.TrimSpace(strings.SplitN(string(encoded), "=", 2)[1])
	}
	newline := "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		newline = "\r\n"
	}
	var table []string
	var p unstable.Parser
	p.Reset(data)
	type edit struct {
		start, end int
		value      string
	}
	var edits []edit
	foundProfile, proxyHeader := false, -1
	for p.NextExpression() {
		n := p.Expression()
		var key []string
		keyOffset := -1
		iter := n.Key()
		for iter.Next() {
			if keyOffset < 0 {
				keyOffset = int(iter.Node().Raw.Offset)
			}
			key = append(key, string(iter.Node().Data))
		}
		if n.Kind == unstable.Table || n.Kind == unstable.ArrayTable {
			table = key
			if n.Kind == unstable.Table && len(key) == 1 && key[0] == "proxy" {
				start := keyOffset
				// Table expressions have no value. Locate the end of their
				// header line without disturbing the header's trailing comment.
				if start < 0 || start >= len(data) {
					return nil, fmt.Errorf("unsupported proxy table layout")
				}
				if end := bytes.IndexByte(data[start:], '\n'); end >= 0 {
					proxyHeader = start + end + 1
				} else {
					proxyHeader = len(data)
				}
			}
			continue
		}
		if n.Kind == unstable.KeyValue && len(table) == 0 && len(key) == 1 && key[0] == "auth_profile" {
			if n.Value().Kind != unstable.String {
				return nil, fmt.Errorf("auth_profile must be a string")
			}
			r := n.Value().Raw
			edits = append(edits, edit{int(r.Offset), int(r.Offset + r.Length), quote(profile)})
			foundProfile = true
		}
	}
	if err := p.Error(); err != nil {
		return nil, fmt.Errorf("could not inspect existing configuration")
	}
	if secretFile != "" {
		proxy, ok := config["proxy"].(map[string]any)
		if config["proxy"] != nil && !ok {
			return nil, fmt.Errorf("proxy configuration must be a table")
		}
		if proxy["admin_secret_file"] == nil && proxy["admin_secret"] == nil {
			line := "admin_secret_file = " + quote(secretFile) + newline
			switch {
			case proxyHeader >= 0:
				if proxyHeader == len(data) && len(data) > 0 && data[len(data)-1] != '\n' {
					line = newline + line
				}
				edits = append(edits, edit{proxyHeader, proxyHeader, line})
			case proxy == nil:
				edits = append(edits, edit{len(data), len(data), newline + "[proxy]" + newline + line})
			default:
				return nil, fmt.Errorf("add proxy.admin_secret_file to the existing inline or dotted proxy table before selecting this profile")
			}
		}
	}
	// Edits are few and may share insertion offsets; applying later offsets
	// first preserves original source positions and all unrelated text.
	for i := 0; i < len(edits); i++ {
		for j := i + 1; j < len(edits); j++ {
			if edits[j].start > edits[i].start {
				edits[i], edits[j] = edits[j], edits[i]
			}
		}
	}
	updated := append([]byte{}, data...)
	for _, e := range edits {
		if e.start < 0 || e.end < e.start || e.end > len(updated) {
			return nil, fmt.Errorf("unsupported configuration layout")
		}
		updated = append(append(append([]byte{}, updated[:e.start]...), e.value...), updated[e.end:]...)
	}
	if !foundProfile {
		updated = append([]byte("auth_profile = "+quote(profile)+newline), updated...)
	}
	var verify map[string]any
	if err := toml.Unmarshal(updated, &verify); err != nil || verify["auth_profile"] != profile {
		return nil, fmt.Errorf("could not safely update authentication settings")
	}
	return updated, nil
}
