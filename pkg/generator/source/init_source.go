package source

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-getter"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/vendor"
)

// InitSource identifies a directory and its default initialization behavior.
type InitSource struct {
	Source string
	Name   string
	Copy   bool
}

// NormalizeInitSource expands official example aliases and GitHub directory URLs.
// GitHub browser URLs use one path segment for the revision; refs containing
// slashes should use the canonical //directory syntax and --ref instead.
func NormalizeInitSource(value, ref string) (InitSource, error) {
	defer perf.Track(nil, "source.NormalizeInitSource")()

	if vendor.HasLocalPathPrefix(value) {
		return InitSource{Source: value, Name: directorySourceName(value)}, nil
	}
	alias := strings.HasPrefix(value, "examples/")
	if alias {
		value = "github.com/cloudposse/atmos//" + value
	}
	result := InitSource{Source: value}
	u, err := parseDirectoryURL(value)
	if err != nil {
		return result, fmt.Errorf("%w: invalid source: %w", errUtils.ErrInvalidFormat, err)
	}
	if isGitHubGitURL(u) {
		result, err = normalizeGitHubDirectory(u)
		if err != nil {
			return result, err
		}
	}
	// Apply --ref before the alias's default so callers can override main.
	result.Source = WithRef(result.Source, url.QueryEscape(ref))
	if alias {
		result.Source = WithRef(result.Source, "main")
	}
	if result.Name == "" {
		result.Name = directorySourceName(result.Source)
	}
	return result, nil
}

func isGitHubGitURL(u *url.URL) bool {
	if !strings.EqualFold(u.Hostname(), "github.com") {
		return false
	}
	switch u.Scheme {
	case "http", "https", "ssh", "git":
		return true
	default:
		return false
	}
}

// parseDirectoryURL recognizes SCP-style Git addresses without changing the
// fetch syntax for sources outside GitHub. Explicit local paths bypass it.
func parseDirectoryURL(value string) (*url.URL, error) {
	value = strings.TrimPrefix(value, "git::")
	if strings.HasPrefix(value, "github.com/") {
		value = "https://" + value
	}
	if !strings.Contains(value, "://") {
		authority, repository, found := strings.Cut(value, ":")
		if found && strings.Contains(authority, "@") {
			value = "ssh://" + authority + "/" + repository
		}
	}
	return url.Parse(value)
}

func normalizeGitHubDirectory(u *url.URL) (InitSource, error) {
	result := InitSource{}
	parts := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return result, fmt.Errorf("%w: GitHub sources require an owner and repository", errUtils.ErrInvalidFormat)
	}
	repo := strings.TrimSuffix(parts[1], ".git")
	subdir := ""
	query := u.Query()
	if len(parts) == 3 {
		var err error
		subdir, err = gitHubSubdirectory(parts[2], query)
		if err != nil {
			return result, err
		}
	}
	if err := validateInitSubdirectory(subdir); err != nil {
		return result, err
	}
	u.Path = "/" + parts[0] + "/" + repo + ".git"
	if subdir != "" {
		subdir = path.Clean(subdir)
		u.Path += "//" + subdir
	}
	u.RawPath = ""
	result.Copy = isOfficialExample(parts[0], repo, subdir)
	applyExampleCloneDepth(query, result.Copy)
	u.RawQuery = query.Encode()
	u.Fragment = ""
	result.Source = "git::" + u.String()
	result.Name = repo
	if subdir != "" && subdir != "." {
		result.Name = path.Base(subdir)
	}
	return result, nil
}

func applyExampleCloneDepth(query url.Values, officialExample bool) {
	// Official examples need only one revision, not the repository's history.
	if officialExample && !query.Has("depth") {
		query.Set("depth", "1")
	}
}

func validateInitSubdirectory(subdir string) error {
	for _, segment := range strings.Split(subdir, "/") {
		if segment == ".." || strings.Contains(segment, "\\") {
			return fmt.Errorf("%w: invalid source subdirectory", errUtils.ErrPathTraversal)
		}
	}
	return nil
}

func isOfficialExample(owner, repo, subdir string) bool {
	return strings.EqualFold(owner, "cloudposse") && strings.EqualFold(repo, "atmos") &&
		(subdir == "examples" || strings.HasPrefix(subdir, "examples/"))
}

func gitHubSubdirectory(value string, query url.Values) (string, error) {
	subdir := strings.TrimPrefix(value, "/")
	if strings.HasPrefix(value, "/") || !strings.HasPrefix(subdir, "tree/") {
		return subdir, nil
	}
	browser := strings.SplitN(strings.TrimPrefix(subdir, "tree/"), "/", 2)
	if browser[0] == "" {
		return "", fmt.Errorf("%w: GitHub tree URL requires a revision", errUtils.ErrInvalidFormat)
	}
	if !query.Has("ref") {
		query.Set("ref", browser[0])
	}
	if len(browser) == 2 {
		return browser[1], nil
	}
	return "", nil
}

func directorySourceName(src string) string {
	if vendor.IsOCIURI(src) {
		image, _, _ := strings.Cut(strings.TrimPrefix(src, "oci://"), "@")
		name, _, _ := strings.Cut(path.Base(image), ":")
		return name
	}
	if vendor.IsLocalPath(src) && !vendor.IsFileURI(src) {
		if absolute, err := filepath.Abs(src); err == nil {
			return filepath.Base(absolute)
		}
	}
	root, subdir := getter.SourceDirSubdir(src)
	if subdir != "" && subdir != "." {
		return path.Base(subdir)
	}
	root = strings.TrimPrefix(root, "git::")
	if u, err := parseDirectoryURL(root); err == nil {
		root = u.Path
	}
	name := strings.TrimSuffix(path.Base(strings.TrimRight(root, "/")), ".git")
	for _, suffix := range []string{".tar.gz", ".tar.bz2", ".tar.xz", ".tgz", ".zip"} {
		name = strings.TrimSuffix(name, suffix)
	}
	return name
}
