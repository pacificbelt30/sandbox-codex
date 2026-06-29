package dockerdefaults

import "embed"

// Dockerfile is the default sandbox image definition.
//
//go:embed sandbox/Dockerfile
var Dockerfile []byte

// Entrypoint is the default container entrypoint script.
//
//go:embed sandbox/entrypoint.sh
var Entrypoint []byte

// ProxyDockerfile is the Dockerfile for the auth proxy container image.
//
//go:embed proxy/Dockerfile
var ProxyDockerfile []byte

// Templates holds the embedded template Dockerfiles under templates/<name>/.
//
//go:embed templates
var Templates embed.FS
