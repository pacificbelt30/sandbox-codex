package sandbox

import (
	"fmt"
	"net/url"
	"strings"
)

// Compatibility contains explicit host-side certificate and client settings.
// No SSH private key or agent socket is ever forwarded by these settings.
type Compatibility struct {
	SSHProxy bool
	JVMProxy bool
	CABundle string
}

func compatibilityEnv(opts RunOptions, env []string) ([]string, error) {
	var endpoint, noProxy string
	for _, entry := range env {
		if strings.HasPrefix(entry, "NO_PROXY=") {
			noProxy = strings.TrimPrefix(entry, "NO_PROXY=")
		}
		if strings.HasPrefix(entry, "HTTP_PROXY=") {
			endpoint = strings.TrimPrefix(entry, "HTTP_PROXY=")
		}
	}
	if (opts.Compatibility.SSHProxy || opts.Compatibility.JVMProxy) && endpoint == "" {
		return nil, fmt.Errorf("SSH/JVM proxy compatibility requires general egress")
	}
	if opts.Compatibility.SSHProxy {
		env = append(env, "DOCK_SSH_PROXY=1")
	}
	if opts.Compatibility.JVMProxy {
		u, err := url.Parse(endpoint)
		if err != nil || u.Hostname() == "" || u.Port() == "" {
			return nil, fmt.Errorf("invalid JVM proxy endpoint")
		}
		env = append(env, "JAVA_TOOL_OPTIONS=-Dhttp.proxyHost="+u.Hostname()+" -Dhttp.proxyPort="+u.Port()+" -Dhttps.proxyHost="+u.Hostname()+" -Dhttps.proxyPort="+u.Port()+" -Dhttp.nonProxyHosts="+strings.ReplaceAll(noProxy, ",", "|"))
	}
	if opts.Compatibility.CABundle != "" {
		env = append(env, "DOCK_CA_BUNDLE=/run/cxd-custom-ca.pem")
	}
	return env, nil
}
