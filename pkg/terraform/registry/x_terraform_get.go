package registry

import "strings"

// archiveExtensions are the HTTP(S) archive suffixes the module mirror can cache.
var archiveExtensions = []string{".tar.gz", ".tgz", ".tar.bz2", ".tar", ".zip"}

// getterPrefixes are go-getter forced-protocol prefixes that the HTTP proxy cannot
// cache (git, mercurial, object stores). These pass through unchanged.
var getterPrefixes = []string{"git::", "hg::", "s3::", "gcs::", "git@", "ssh://"}

// classifyXTerraformGet decides whether a module's X-Terraform-Get value is a plain
// HTTP(S) archive the proxy can cache. It is deliberately conservative: anything with
// a go-getter forced protocol, a git source, a "//" subdir selector, or a
// non-archive target is treated as non-cacheable (passthrough) so behavior is
// identical to no-cache. The git:: case — the common one for the public registry and
// mono-repos — is completed later by the git mirror.
func classifyXTerraformGet(value string) (archiveURL string, cacheable bool) {
	v := strings.TrimSpace(value)
	if v == "" || hasGetterPrefix(v) {
		return "", false
	}

	scheme := schemeOf(v)
	if scheme == "" {
		return "", false
	}

	// A go-getter "//" subdir selector after the scheme means the archive contains a
	// subdirectory to extract — not handled in V1; pass through.
	if strings.Contains(v[len(scheme):], "//") {
		return "", false
	}

	if !hasArchiveExtension(v) {
		return "", false
	}
	return v, true
}

// hasGetterPrefix reports whether v begins with a go-getter forced-protocol prefix.
func hasGetterPrefix(v string) bool {
	for _, p := range getterPrefixes {
		if strings.HasPrefix(v, p) {
			return true
		}
	}
	return false
}

// schemeOf returns the HTTP(S) scheme prefix of v, or "" if it has none.
func schemeOf(v string) string {
	switch {
	case strings.HasPrefix(v, "https://"):
		return "https://"
	case strings.HasPrefix(v, "http://"):
		return "http://"
	default:
		return ""
	}
}

// hasArchiveExtension reports whether v (ignoring any query string) ends in a known
// archive extension.
func hasArchiveExtension(v string) bool {
	pathPart := v
	if i := strings.IndexByte(pathPart, '?'); i >= 0 {
		pathPart = pathPart[:i]
	}
	lower := strings.ToLower(pathPart)
	for _, ext := range archiveExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}
