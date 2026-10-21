package sandbox

import (
	"context"
	"fmt"

	"github.com/pacificbelt30/codex-dock/internal/kataguard"
)

func (m *Manager) resourceProfile(ctx context.Context, name string) (kataguard.ResourceProfile, error) {
	if p, ok := kataguard.LookupResourceProfile(name); ok {
		return p, nil
	}
	g, err := m.kataNode()
	if err != nil {
		return kataguard.ResourceProfile{}, err
	}
	source, ok := g.(interface {
		Profiles(context.Context) ([]kataguard.ResourceProfile, error)
	})
	if !ok {
		return kataguard.ResourceProfile{}, fmt.Errorf("node does not support operator resource profiles")
	}
	profiles, err := source.Profiles(ctx)
	if err != nil {
		return kataguard.ResourceProfile{}, err
	}
	for _, p := range profiles {
		if p.Name == name {
			if err := kataguard.ValidateCustomResourceProfile(p); err != nil {
				return kataguard.ResourceProfile{}, err
			}
			return p, nil
		}
	}
	return kataguard.ResourceProfile{}, fmt.Errorf("unknown resource profile %q", name)
}

// ResourceProfiles queries node-registered budgets without reserving capacity.
func (m *Manager) ResourceProfiles(ctx context.Context) ([]kataguard.ResourceProfile, error) {
	g, err := m.kataNode()
	if err != nil {
		return nil, err
	}
	source, ok := g.(interface {
		Profiles(context.Context) ([]kataguard.ResourceProfile, error)
	})
	if !ok {
		return nil, fmt.Errorf("node does not report resource profiles")
	}
	return source.Profiles(ctx)
}
