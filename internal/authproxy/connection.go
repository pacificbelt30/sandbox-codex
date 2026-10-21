package authproxy

// ModelPolicyActive reports the proxy-wide constraint, independent of the
// additional immutable policy carried by an individual worker token.
func (p *Proxy) ModelPolicyActive() bool { return len(p.cfg.AllowedModels) != 0 }
