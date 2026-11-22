package docker

import (
	"net"
	"net/url"
	"strings"
)

// convertToHostname normalizes a registry address that may carry a scheme,
// path or trailing slash (for example "https://host/" or "host/v2/") to the
// bare host[:port] Docker uses as its credential key.
//
// It is a port of docker/cli's credentials.ConvertToHostname
// (cli/config/credentials/file_store.go), which `docker login` applies to the
// server address before storing credentials (cli/command/registry.go,
// GetDefaultAuthConfig) and which the file store uses to match legacy
// scheme-prefixed auths keys (fileStore.Get). It is ported rather than
// imported so this package does not depend on docker/cli.
func convertToHostname(maybeURL string) string {
	stripped := maybeURL
	if strings.Contains(stripped, "://") {
		u, err := url.Parse(stripped)
		if err == nil && u.Hostname() != "" {
			if u.Port() == "" {
				return u.Hostname()
			}
			return net.JoinHostPort(u.Hostname(), u.Port())
		}

		if hostName := hostFromURLFallback(stripped); hostName != "" {
			return hostName
		}
	}
	hostName, _, _ := strings.Cut(stripped, "/")
	return hostName
}

// hostFromURLFallback extracts a host from scheme URLs that net/url rejects,
// such as unbracketed IPv6 literals. It mirrors docker/cli's helper of the same
// name in cli/config/credentials/file_store.go.
func hostFromURLFallback(maybeURL string) string {
	_, rest, ok := strings.Cut(maybeURL, "://")
	if !ok {
		return ""
	}

	hostName, _, _ := strings.Cut(rest, "/")
	if hostName == "" {
		return ""
	}

	if strings.Count(hostName, ":") > 1 && !strings.HasPrefix(hostName, "[") {
		portStart := strings.LastIndex(hostName, ":")
		addr, port := hostName[:portStart], hostName[portStart+1:]
		if addr != "" && isPort(port) {
			return net.JoinHostPort(addr, port)
		}
	}

	return hostName
}

// isPort reports whether port is a non-empty string of ASCII digits.
func isPort(port string) bool {
	if port == "" {
		return false
	}
	for _, r := range port {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
