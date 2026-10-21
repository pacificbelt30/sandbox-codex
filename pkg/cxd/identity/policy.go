package identity

import (
	"fmt"
	"slices"
	"strings"
)

// ModelPolicy limits this token to exact upstream model IDs. A nil policy
// inherits the proxy policy; a non-nil policy with no IDs denies every model.
// Model selection in an agent command is not an authorization boundary.
type ModelPolicy struct {
	AllowedModels []string `json:"allowed_models"`
}

func (p *ModelPolicy) Validate() error {
	if p == nil {
		return nil
	}
	if len(p.AllowedModels) > 256 {
		return fmt.Errorf("at most 256 allowed models may be configured")
	}
	for _, model := range p.AllowedModels {
		if model == "" || len(model) > 256 || strings.ContainsAny(model, "*?,\\\" \t\r\n\x00") {
			return fmt.Errorf("allowed models must be exact, nonempty upstream IDs")
		}
	}
	return nil
}

func (p *ModelPolicy) Clone() *ModelPolicy {
	if p == nil {
		return nil
	}
	return &ModelPolicy{AllowedModels: slices.Clone(p.AllowedModels)}
}
