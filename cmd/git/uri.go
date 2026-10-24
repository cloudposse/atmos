package git

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	// Go-getter forcing prefix for Git.
	gitGetterPrefix = "git::"
	// URI query parameter name that maps to branch or ref.
	queryParamRef = "ref"
	// URI query parameter name that maps to clone depth.
	queryParamDepth = "depth"
)

// ParsedURI is the result of parsing a clone URI argument.
type ParsedURI struct {
	// URI is the clean remote URI (without git:: prefix and without query params).
	URI string
	// Branch is the parsed branch/ref from ?ref=, or empty.
	Branch string
	// Depth is the parsed clone depth from ?depth=, or 0 for full history.
	Depth int
	// RepoName is the last path component without .git suffix, used as the
	// clone directory name for ad hoc URI clones.
	RepoName string
}

// IsURI reports whether s looks like a Git URI:
//   - has git:: forcing prefix
//   - starts with https:// or http://
//   - starts with git@... (SCP-style)
//   - starts with ssh://
//   - starts with git://
func IsURI(s string) bool {
	if strings.HasPrefix(s, gitGetterPrefix) {
		return true
	}
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "ssh://") ||
		strings.HasPrefix(lower, "git://") ||
		isScpStyle(s)
}

// isScpStyle detects git@host:path SCP-style URIs.
func isScpStyle(s string) bool {
	atIdx := strings.Index(s, "@")
	colonIdx := strings.Index(s, ":")
	slashIdx := strings.Index(s, "/")
	// git@github.com:org/repo — at before colon, colon before first slash (or no slash).
	return atIdx > 0 && colonIdx > atIdx && (slashIdx < 0 || colonIdx < slashIdx)
}

// ParseCloneURI parses a clone URI argument, stripping the git:: prefix and
// extracting ?ref= and ?depth= query parameters (precedence: flags > query
// params > config). Unknown query parameters return an error.
func ParseCloneURI(raw string) (*ParsedURI, error) {
	stripped := strings.TrimPrefix(raw, gitGetterPrefix)

	// SCP-style URIs (git@host:path) cannot be parsed by net/url; no query
	// parameters are possible in that form.
	if isScpStyle(stripped) {
		return &ParsedURI{
			URI:      stripped,
			RepoName: extractRepoName(stripped),
		}, nil
	}

	u, err := url.Parse(stripped)
	if err != nil {
		return nil, fmt.Errorf("parsing git URI %q: %w", stripped, err)
	}

	parsed := &ParsedURI{
		RepoName: extractRepoName(u.Path),
	}

	q := u.Query()
	for key := range q {
		if key != queryParamRef && key != queryParamDepth {
			return nil, fmt.Errorf("%w: unknown query parameter %q in URI %q (only 'ref' and 'depth' are supported)", errUtils.ErrInvalidFlagValue, key, stripped)
		}
	}

	parsed.Branch = q.Get(queryParamRef)

	if depthStr := q.Get(queryParamDepth); depthStr != "" {
		var depth int
		if _, scanErr := fmt.Sscanf(depthStr, "%d", &depth); scanErr != nil || depth < 0 {
			return nil, fmt.Errorf("%w: 'depth' query parameter must be a non-negative integer, got %q", errUtils.ErrInvalidFlagValue, depthStr)
		}
		parsed.Depth = depth
	}

	// Reconstruct URI without query string.
	u.RawQuery = ""
	u.Fragment = ""
	parsed.URI = u.String()

	return parsed, nil
}

// extractRepoName derives the repository directory name from a URI path:
// the last path segment without a .git suffix.
func extractRepoName(rawPath string) string {
	base := path.Base(rawPath)
	return strings.TrimSuffix(base, ".git")
}
