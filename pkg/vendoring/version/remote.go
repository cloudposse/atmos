// Package version resolves the latest allowed upstream version for a vendored
// Git source: it lists remote tags, parses semantic versions, and applies the
// configured version constraints.
package version

import (
	"context"
	"fmt"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage/memory"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RemoteLister lists tag names from a remote Git repository. It is an interface
// so update logic can be unit-tested without network access.
type RemoteLister interface {
	ListTags(ctx context.Context, gitURI string) ([]string, error)
}

// GoGitLister lists remote tags using go-git's in-memory remote (no shelling out
// to the `git` binary, no working tree). Public repositories work without auth;
// private repositories are out of scope for the initial version.
type GoGitLister struct{}

// DefaultLister is the production RemoteLister backed by go-git.
var DefaultLister RemoteLister = &GoGitLister{}

// ListTags returns the short tag names advertised by the remote at gitURI.
func (l *GoGitLister) ListTags(ctx context.Context, gitURI string) ([]string, error) {
	defer perf.Track(nil, "version.GoGitLister.ListTags")()

	remote := gogit.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{gitURI},
	})

	refs, err := remote.ListContext(ctx, &gogit.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", errUtils.ErrGitLsRemoteFailed, gitURI, err)
	}

	tags := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.Name().IsTag() {
			tags = append(tags, ref.Name().Short())
		}
	}
	return tags, nil
}

// ExtractGitURI extracts a clean Git URI from a vendor source string. It handles
// `git::` prefixes, `github.com/` shorthand, query parameters, and `.git`
// suffixes so the result can be handed to a remote lister.
func ExtractGitURI(source string) string {
	defer perf.Track(nil, "version.ExtractGitURI")()

	source = strings.TrimPrefix(source, "git::")

	if strings.HasPrefix(source, "github.com/") {
		source = "https://" + source
	}

	if idx := strings.Index(source, "?"); idx != -1 {
		source = source[:idx]
	}

	if idx := strings.Index(source, ".git//"); idx != -1 {
		source = source[:idx+len(".git")]
	} else {
		start := 0
		if schemeIdx := strings.Index(source, "://"); schemeIdx != -1 {
			start = schemeIdx + len("://")
		}
		if idx := strings.Index(source[start:], "//"); idx != -1 {
			source = source[:start+idx]
		}
	}

	// go-git accepts URLs with or without the .git suffix; keep it canonical.
	if !strings.HasSuffix(source, ".git") && strings.HasPrefix(source, "https://") {
		source += ".git"
	}
	return source
}

// IsGitSource reports whether a vendor source string looks like a Git repository
// (the only source type supported by update/diff in the initial version).
func IsGitSource(source string) bool {
	defer perf.Track(nil, "version.IsGitSource")()

	switch {
	case strings.HasPrefix(source, "git::"):
		return true
	case strings.HasPrefix(source, "github.com/"):
		return true
	case strings.Contains(source, "github.com/"),
		strings.Contains(source, "gitlab.com/"),
		strings.Contains(source, "bitbucket.org/"):
		return true
	default:
		return false
	}
}
