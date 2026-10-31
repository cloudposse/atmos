package source

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	// A Git object id, abbreviated or full.
	commitRefPattern = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
	// A release tag such as v1, 1.2.3, v1.2.3-rc.1, or 2024.01.
	releaseRefPattern = regexp.MustCompile(`^v?[0-9]+([.][0-9]+)*([-+][0-9A-Za-z.+-]+)?$`)
)

// floatingRef returns the Git ref that the source follows when it can move: a branch
// or any name that is neither a commit id nor a release tag. A fetched copy of such a
// source is never refreshed unless a TTL expires or the user pulls with --force.
func floatingRef(spec *schema.VendorComponentSource) (string, bool) {
	if spec == nil || spec.Uri == "" || isLocalSource(spec.Uri) {
		return "", false
	}
	ref := spec.Version
	if _, query, found := strings.Cut(spec.Uri, "?"); found {
		if values, err := url.ParseQuery(query); err == nil && values.Get("ref") != "" {
			ref = values.Get("ref")
		}
	}
	ref = strings.TrimPrefix(ref, "refs/tags/")
	if ref == "" || commitRefPattern.MatchString(ref) || releaseRefPattern.MatchString(ref) {
		return "", false
	}
	return ref, true
}

// floatingRefNotice explains why a cached copy of a branch-pinned source is reused.
// It returns an empty string when the source cannot drift or a TTL already governs it.
func floatingRefNotice(spec *schema.VendorComponentSource, component, stack string) string {
	ref, floating := floatingRef(spec)
	if !floating || spec.TTL != "" {
		return ""
	}
	target := component
	if stack != "" {
		target += " -s " + stack
	}
	return fmt.Sprintf("Source %s follows the floating ref %q and is not re-fetched automatically; "+
		"refresh it with `atmos terraform source pull %s --force`, or set a source ttl", component, ref, target)
}
