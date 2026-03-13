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

// ProxyDockerfile is the Dockerfile for the auth proxy container image.
//
//go:embed auth-proxy.Dockerfile
var ProxyDockerfile []byte
