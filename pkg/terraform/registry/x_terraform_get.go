package registry

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	getter "github.com/hashicorp/go-getter"
)

// moduleSourceDigestLen is the hex-digest length used to key cached module sources.
const moduleSourceDigestLen = 32

// sourceExt is the archive extension appended to the _source path so the Terraform-side
// go-getter detects an archive and unpacks it. An extension is more reliable than the
// ?archive= query param: go-getter forces file+decompress mode off the path suffix even
// when the module installer requests directory mode (a bare URL would instead be parsed
// as HTML looking for a <meta> redirect, which chokes on binary archive bytes). It must
// be ".tar.gz" specifically — Terraform/OpenTofu's module installer registers a limited
// decompressor set that includes tar.gz/tgz/zip but NOT a bare ".tar", so an uncompressed
// tar is not recognized and falls through to the (failing) HTML path.
const sourceExt = ".tar.gz"

// splitModuleSource separates a go-getter source string into its base source (forced
// protocol + URL + query, e.g. "git::https://github.com/org/repo.git?ref=v1") and its
// optional "//subdir" selector. It delegates to go-getter so the split matches exactly
// what the Terraform-side go-getter does when it later reattaches the subdir.
func splitModuleSource(source string) (base, subdir string) {
	return getter.SourceDirSubdir(source)
}

// moduleSourceProxyURL builds the X-Terraform-Get value that routes a module download
// back through the proxy's _source sub-route. The base source (with its pinned ref) is
// encoded into the path so the mirror can resolve it on a miss; any //subdir is
// reattached so the Terraform-side go-getter extracts it from the unpacked tar. Because
// the subdir is carried client-side and stripped from the cache key, a mono-repo
// referenced by several modules at different subdirs is fetched and cached once.
func moduleSourceProxyURL(proxyBaseURL, base, subdir string) string {
	url := proxyBaseURL + "modules/" + moduleSourceSegment + "/" + encodeModuleSource(base) + sourceExt
	if subdir != "" {
		url += "//" + subdir
	}
	return url
}

// encodeModuleSource encodes a base source string for use as a single URL path segment.
func encodeModuleSource(base string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(base))
}

// decodeModuleSource reverses encodeModuleSource.
func decodeModuleSource(encoded string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("%w: undecodable source ref: %w", ErrInvalidModulePath, err)
	}
	return string(raw), nil
}

// moduleSourceDigest derives a stable cache-key digest from a base source string.
func moduleSourceDigest(base string) string {
	sum := sha256.Sum256([]byte(base))
	return hex.EncodeToString(sum[:])[:moduleSourceDigestLen]
}
