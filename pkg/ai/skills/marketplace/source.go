package marketplace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ghtoken "github.com/cloudposse/atmos/pkg/github"
	"github.com/cloudposse/atmos/pkg/perf"
)

// githubComHost is the public GitHub host, used as the default when no GitHub Enterprise
// Server host is configured, and as the target of Format 0's bare owner/repo shorthand.
const githubComHost = "github.com"

// SourceInfo contains parsed source information.
type SourceInfo struct {
	Type     string // "github", "git", "local".
	Owner    string // GitHub owner.
	Repo     string // GitHub repo name.
	Ref      string // Tag, branch, or commit (optional).
	URL      string // Full URL for git clone.
	FullPath string // e.g., "github.com/user/repo".
	Name     string // Skill name (derived from repo).
}

// ParseSource parses various source formats into SourceInfo.
//
// Supported formats:
//   - user/repo (GitHub assumed)
//   - user/repo@v1.2.3
//   - github.com/user/repo
//   - github.com/user/repo@v1.2.3
//   - https://github.com/user/repo.git
//   - git@github.com:user/repo.git
//   - file:///absolute/path/to/skill (local filesystem)
//   - /absolute/or/relative/path/to/skill (local filesystem, if it exists on disk)
func ParseSource(source string) (*SourceInfo, error) {
	defer perf.Track(nil, "marketplace.ParseSource")()

	// Remove @ref suffix if present.
	ref := ""
	if idx := strings.LastIndex(source, "@"); idx > 0 && !strings.Contains(source, "://") {
		// Only treat @ as ref separator if not part of git@github.com.
		if !strings.HasPrefix(source, "git@") {
			ref = source[idx+1:]
			source = source[:idx]
		}
	}

	// GitHub Enterprise Server host configured via GITHUB_SERVER_URL, if any (equal to
	// githubComHost when unset). Formats 1-3 below accept this host in addition to
	// github.com, since a marketplace source URL is a user-supplied value naming wherever
	// their skill repos actually live. Format 0 (bare owner/repo shorthand) deliberately stays
	// github.com-only: an unqualified "owner/repo" can't disambiguate which host was meant.
	ghesHost := ghtoken.RepoEndpoints().Host

	// Format 0: Bare owner/repo shorthand (GitHub assumed).
	if isOwnerRepoShorthand(source) {
		return parseGitHubShorthand(githubComHost+"/"+source, ref, githubComHost)
	}

	// Format 1: GitHub (or GHES) shorthand (github.com/user/repo).
	if host, ok := matchGitHubSourceHost(source, ghesHost, "%s/"); ok {
		return parseGitHubShorthand(source, ref, host)
	}

	// Format 2: HTTPS URL (https://github.com/user/repo.git).
	if host, ok := matchGitHubSourceHost(source, ghesHost, "https://%s/"); ok {
		return parseGitHubHTTPS(source, ref, host)
	}

	// Format 3: SSH URL (git@github.com:user/repo.git).
	if host, ok := matchGitHubSourceHost(source, ghesHost, "git@%s:"); ok {
		return parseGitHubSSH(source, ref, host)
	}

	// Format 4: Local filesystem path (file:// URL, or a plain path that exists on
	// disk). Checked last so a typo'd GitHub shorthand still surfaces the
	// "unsupported source format" error below instead of a confusing
	// "no such file or directory" one.
	if info := parseLocalSource(source); info != nil {
		return info, nil
	}

	return nil, fmt.Errorf("%w: unsupported source format: %s", ErrInvalidSource, source)
}

// parseLocalSource recognizes a local filesystem source: an explicit file:// URL
// (whose path is trusted without an existence check, mirroring go-getter's file://
// scheme), or a plain path that exists on disk. Returns nil when source isn't a
// local path at all, so ParseSource can fall through to its "unsupported source
// format" error.
//
// FullPath is deliberately NOT the raw absolute path: on Windows that starts with a
// drive letter (e.g. "C:\Users\..."), and filepath.Join'ing it onto the skills
// install directory (see Installer.getInstallPath) would try to create a literal
// "C:" path segment, which is invalid. "local/<dir-name>" keeps the same
// owner/repo-style one-level nesting Git sources get, without embedding the host's
// filesystem layout.
func parseLocalSource(source string) *SourceInfo {
	raw := source
	isFileURL := strings.HasPrefix(source, "file://")
	if isFileURL {
		raw = strings.TrimPrefix(source, "file://")
	} else if _, err := os.Stat(raw); err != nil {
		return nil
	}

	abs, err := filepath.Abs(raw)
	if err != nil {
		return nil
	}

	name := filepath.Base(abs)
	return &SourceInfo{
		Type:     "local",
		URL:      abs,
		FullPath: fmt.Sprintf("local/%s", name), // Same "namespace/name" shape as github.com/owner/repo.
		Name:     name,
	}
}

// matchGitHubSourceHost reports whether source is prefixed with either githubComHost or
// ghesHost, using prefixTemplate as an fmt.Sprintf template with a single "%s" for the host
// (e.g. "%s/", "https://%s/", "git@%s:"). Returns the matched host and ok=true, or ok=false
// when neither matches. When ghesHost equals githubComHost (GHES not configured), it is not
// checked a second time.
func matchGitHubSourceHost(source, ghesHost, prefixTemplate string) (host string, ok bool) {
	if strings.HasPrefix(source, fmt.Sprintf(prefixTemplate, githubComHost)) {
		return githubComHost, true
	}
	if ghesHost != githubComHost && strings.HasPrefix(source, fmt.Sprintf(prefixTemplate, ghesHost)) {
		return ghesHost, true
	}
	return "", false
}

// isOwnerRepoShorthand returns true for bare "owner/repo" format
// (no dots, no colons, no slashes beyond the single separator).
func isOwnerRepoShorthand(source string) bool {
	if strings.Contains(source, ".") || strings.Contains(source, ":") || strings.Contains(source, "//") {
		return false
	}
	parts := strings.Split(source, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}

// githubCloneURL builds the HTTPS clone URL for owner/repo on host: github.com's default
// ServerURL when host is "github.com", or the resolved RepoEndpoints ServerURL otherwise (the
// two coincide unless GITHUB_SERVER_URL points at a GHES host while the source URL explicitly
// named plain github.com).
func githubCloneURL(host, owner, repo string) string {
	serverURL := "https://" + githubComHost
	if host != githubComHost {
		serverURL = ghtoken.RepoEndpoints().ServerURL
	}
	return fmt.Sprintf("%s/%s/%s.git", serverURL, owner, repo)
}

// parseGitHubShorthand parses "<host>/user/repo" format (host is "github.com" or the
// configured GHES host).
func parseGitHubShorthand(source, ref, host string) (*SourceInfo, error) {
	parts := strings.Split(strings.TrimPrefix(source, host+"/"), "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("%w: invalid GitHub shorthand format (expected %s/user/repo)", ErrInvalidSource, host)
	}

	owner := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")

	return &SourceInfo{
		Type:     "github",
		Owner:    owner,
		Repo:     repo,
		Ref:      ref,
		URL:      githubCloneURL(host, owner, repo),
		FullPath: fmt.Sprintf("%s/%s/%s", host, owner, repo),
		Name:     repo,
	}, nil
}

// parseGitHubHTTPS parses "https://<host>/user/repo.git" format.
func parseGitHubHTTPS(source, ref, host string) (*SourceInfo, error) {
	remainder := strings.TrimPrefix(source, "https://"+host+"/")
	parts := strings.Split(remainder, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("%w: invalid GitHub HTTPS URL format", ErrInvalidSource)
	}

	owner := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")

	return &SourceInfo{
		Type:     "github",
		Owner:    owner,
		Repo:     repo,
		Ref:      ref,
		URL:      githubCloneURL(host, owner, repo),
		FullPath: fmt.Sprintf("%s/%s/%s", host, owner, repo),
		Name:     repo,
	}, nil
}

// parseGitHubSSH parses "git@<host>:user/repo.git" format.
func parseGitHubSSH(source, ref, host string) (*SourceInfo, error) {
	remainder := strings.TrimPrefix(source, "git@"+host+":")
	parts := strings.Split(remainder, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("%w: invalid GitHub SSH URL format", ErrInvalidSource)
	}

	owner := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")

	return &SourceInfo{
		Type:     "github",
		Owner:    owner,
		Repo:     repo,
		Ref:      ref,
		URL:      githubCloneURL(host, owner, repo), // Use HTTPS for cloning.
		FullPath: fmt.Sprintf("%s/%s/%s", host, owner, repo),
		Name:     repo,
	}, nil
}
