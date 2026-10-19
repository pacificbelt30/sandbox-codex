package cxd

// Capabilities describes the validated node connected by Open. Features are
// additive strings; callers must check a feature before relying on it.
type Capabilities struct {
	BuildVersion    string
	ProtocolVersion int
	Features        []string
}

func (c *Client) Capabilities() Capabilities {
	version, features := c.node.Capabilities()
	return Capabilities{BuildVersion: version, ProtocolVersion: c.node.ProtocolVersion(), Features: features}
}
