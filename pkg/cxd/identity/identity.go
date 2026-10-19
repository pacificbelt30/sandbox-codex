// Package identity contains the optional execution identity attached to a
// short-lived auth proxy token. It is never sent to the upstream AI service.
package identity

import (
	"fmt"
	"strings"
)

type Identity struct {
	CaseID            string `json:"case_id"`
	RunID             string `json:"run_id"`
	ConnectionVersion string `json:"connection_version"`
	Provider          string `json:"provider"` // openai or anthropic
}

func (v Identity) Empty() bool { return v == (Identity{}) }

func (v Identity) Validate() error {
	for _, field := range []string{v.CaseID, v.RunID, v.ConnectionVersion} {
		if field == "" || len(field) > 256 || strings.ContainsAny(field, "\x00\r\n") {
			return fmt.Errorf("invalid token identity")
		}
	}
	if v.Provider != "openai" && v.Provider != "anthropic" {
		return fmt.Errorf("invalid token provider")
	}
	return nil
}
