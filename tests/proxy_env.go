package tests

import (
	"net/url"
	"strings"
)

// noProxyEnvVars lists both spellings of the no-proxy variable: git/libcurl honors the lowercase
// form and Go's net/http honors both, so the mirror host has to be present in each.
var noProxyEnvVars = []string{"NO_PROXY", "no_proxy"}

// ensureNoProxyForMirror makes sure every test case's environment exempts the local git mirror's
// host from any configured HTTP(S) proxy. The mirror insteadOf rules route credentialed GitHub
// URLs (Basic-Auth header carrying the ambient token) to the loopback server; if a developer or
// CI machine exports HTTPS_PROXY without exempting loopback, that header would be sent through the
// proxy in cleartext. Existing entries are preserved; the host is appended only when absent.
// mirrorURL is the mirror server's base URL (e.g. "http://127.0.0.1:54321").
func ensureNoProxyForMirror(env map[string]string, lookup func(string) string, mirrorURL string) {
	parsed, err := url.Parse(mirrorURL)
	if err != nil || parsed.Hostname() == "" {
		return
	}
	host := parsed.Hostname()
	for _, name := range noProxyEnvVars {
		current, set := env[name]
		if !set {
			current = lookup(name)
		}
		if noProxyContains(current, host) {
			// Already exempt via the case's own env or the ambient process env; if it came from
			// the process env, leave the case env untouched so the process value keeps applying.
			continue
		}
		if current == "" {
			env[name] = host
			continue
		}
		env[name] = current + "," + host
	}
}

// noProxyContains reports whether a comma-separated no-proxy value already exempts host.
func noProxyContains(value, host string) bool {
	for _, entry := range strings.Split(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if entry == "*" || strings.EqualFold(entry, host) {
			return true
		}
	}
	return false
}
