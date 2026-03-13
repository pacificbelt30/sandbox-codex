package dockerdefaults

import _ "embed"

// Dockerfile is the default sandbox image definition.
//
//go:embed Dockerfile
var Dockerfile []byte

// Entrypoint is the default container entrypoint script.
//
//go:embed entrypoint.sh
var Entrypoint []byte

// AuthProxyDockerfile is the default auth proxy image definition.
//
//go:embed auth-proxy.Dockerfile
var AuthProxyDockerfile []byte
