// Package registry implements Terraform provider and module registry mirrors as
// adapters for the generic caching proxy (pkg/http/proxy). The provider mirror
// translates the Provider Network Mirror Protocol (what Terraform speaks to the
// proxy) into the upstream Provider Registry Protocol; the module mirror caches
// the module registry protocol and routes module archives through the proxy while
// passing git:: sources through unchanged (the future git mirror completes those).
package registry

import (
	"strings"
)

// keyJoin builds a cache key from segments. Cache keys always use forward slashes
// (the store converts to OS paths), so this is a plain "/" join rather than
// path/filepath.Join.
func keyJoin(parts ...string) string {
	return strings.Join(parts, "/")
}

const (
	// The providerPrefix namespaces provider network-mirror requests. The cache wires
	// Terraform's provider_installation network_mirror url to "<proxy>/providers/".
	providerPrefix = "/providers/"
	// The modulePrefix namespaces module registry requests. The cache wires each host's
	// modules.v1 service to "<proxy>/modules/<host>/".
	modulePrefix = "/modules/"
	// The moduleArchiveSegment is the sub-route under which the module mirror serves and
	// caches HTTP-archive modules whose X-Terraform-Get was rewritten back to the proxy.
	moduleArchiveSegment = "_archive"
)

// providerIndexKey is the canonical cache key (and filesystem_mirror path) for a
// provider's version index. Keys always use forward slashes; the store converts to
// OS paths.
func providerIndexKey(host, namespace, providerType string) string {
	return keyJoin("providers", host, namespace, providerType, "index.json")
}

// providerVersionKey is the canonical key for a provider version's package listing.
func providerVersionKey(host, namespace, providerType, version string) string {
	return keyJoin("providers", host, namespace, providerType, version+".json")
}

// providerArchiveKey is the canonical key for a provider zip. The zip sits directly
// under <type>/ so the directory is a valid filesystem_mirror.
func providerArchiveKey(host, namespace, providerType, filename string) string {
	return keyJoin("providers", host, namespace, providerType, filename)
}

// moduleVersionsKey is the canonical key for a module's version listing.
func moduleVersionsKey(host, namespace, name, provider string) string {
	return keyJoin("modules", host, namespace, name, provider, "versions.json")
}

// moduleArchiveKey is the canonical key for a cached HTTP-archive module, keyed by a
// stable digest of the upstream archive URL.
func moduleArchiveKey(digest string) string {
	return keyJoin("modules", moduleArchiveSegment, digest)
}

// providerArchiveRef identifies a provider zip's version and platform parsed from
// its filename.
type providerArchiveRef struct {
	version string
	os      string
	arch    string
}

// minProviderArchiveParts is the number of underscore-separated fields expected in a
// provider zip filename tail (<type>_<version>_<os>_<arch>).
const minProviderArchiveParts = 4

// parseProviderArchive extracts the version, os, and arch from a provider zip
// filename of the form terraform-provider-<type>_<version>_<os>_<arch>.zip. The
// provider type may itself contain underscores, so os/arch/version are taken from
// the tail.
func parseProviderArchive(filename string) (providerArchiveRef, bool) {
	base := strings.TrimSuffix(filename, ".zip")
	base = strings.TrimPrefix(base, "terraform-provider-")
	parts := strings.Split(base, "_")
	if len(parts) < minProviderArchiveParts {
		return providerArchiveRef{}, false
	}
	ref := providerArchiveRef{
		version: parts[len(parts)-3],
		os:      parts[len(parts)-2],
		arch:    parts[len(parts)-1],
	}
	if ref.version == "" || ref.os == "" || ref.arch == "" {
		return providerArchiveRef{}, false
	}
	return ref, true
}
