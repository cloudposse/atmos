package source

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	// A Git object id, abbreviated or full.
	commitRefPattern = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
	// A release tag such as v1, 1.2.3, v1.2.3-rc.1, or 2024.01.
	releaseRefPattern = regexp.MustCompile(`^v?[0-9]+([.][0-9]+)*([-+][0-9A-Za-z.+-]+)?$`)
)

// floatingRef returns the Git ref that the source follows when it can move: a branch
// or any name that is neither a commit id nor a release tag. A Git source without any
// ref follows its default branch, which is reported as floating with an empty ref. A
// fetched copy of such a source is never refreshed unless a TTL expires or the user
// pulls with --force.
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
	if ref == "" {
		return "", isGitSource(spec.Uri)
	}
	ref = strings.TrimPrefix(ref, "refs/tags/")
	if commitRefPattern.MatchString(ref) || releaseRefPattern.MatchString(ref) {
		return "", false
	}
	return ref, true
}

// isGitSource reports whether a remote source is fetched with Git: a forced `git::` source,
// a scheme-less GitHub, GitLab, or Bitbucket path, or a repository path ending in `.git`.
// Archives and object-store sources have no branch, so they never float.
func isGitSource(uri string) bool {
	if strings.HasPrefix(uri, "git::") {
		return true
	}
	for _, host := range []string{"github.com/", "gitlab.com/", "bitbucket.org/"} {
		if strings.HasPrefix(uri, host) {
			return true
		}
	}
	repository, _, _ := strings.Cut(uri, "?")
	if scheme, rest, found := strings.Cut(repository, "://"); found {
		repository = scheme + "://" + strings.SplitN(rest, "//", 2)[0]
	} else {
		repository = strings.SplitN(repository, "//", 2)[0]
	}
	return strings.HasSuffix(repository, ".git")
}

// floatingRefNotice explains why a cached copy of a branch-pinned source is reused, and
// names the pull command of the component's own type. It returns an empty string when the
// source cannot drift or a TTL already governs it.
func floatingRefNotice(spec *schema.VendorComponentSource, componentType, component, stack string) string {
	ref, floating := floatingRef(spec)
	if !floating || spec.TTL != "" {
		return ""
	}
	target := component
	if stack != "" {
		target += " -s " + stack
	}
	follows := fmt.Sprintf("the floating ref %q", ref)
	if ref == "" {
		follows = "its default branch"
	}
	return fmt.Sprintf("Source %s follows %s and is not re-fetched automatically; "+
		"refresh it with `%s %s --force`, or set a source ttl", component, follows, sourcePullCommand(componentType), target)
}

// sourcePullCommand returns the `source pull` command of a component type, such as
// `atmos helmfile source pull` or `atmos aws cloudformation source pull`.
func sourcePullCommand(componentType string) string {
	if componentType == "" {
		componentType = cfg.TerraformComponentType
	}
	return "atmos " + strings.ReplaceAll(componentType, "/", " ") + " source pull"
}
