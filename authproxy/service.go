package authproxy

// Service describes the minimal functionality required by sandbox manager.
// Implementations can be in-process (Proxy) or external (RemoteProxy).
type Service interface {
	IssueToken(containerName string, ttlSec int) (string, error)
	RevokeToken(containerName string)
	IsOAuthMode() bool
	ContainerEndpoint() string
}
