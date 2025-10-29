package marketplace

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// SourceInfo contains parsed source information.
type SourceInfo struct {
	Type     string // "github", "git", "local".
	Owner    string // GitHub owner.
	Repo     string // GitHub repo name.
	Ref      string // Tag, branch, or commit (optional).
	URL      string // Full URL for git clone.
	FullPath string // e.g., "github.com/user/repo".
	Name     string // Agent name (derived from repo).
}

// ParseSource parses various source formats into SourceInfo.
//
// Supported formats:
//   - github.com/user/repo
//   - github.com/user/repo@v1.2.3
//   - https://github.com/user/repo.git
//   - git@github.com:user/repo.git
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

	// Format 1: GitHub shorthand (github.com/user/repo).
	if strings.HasPrefix(source, "github.com/") {
		return parseGitHubShorthand(source, ref)
	}

	// Format 2: HTTPS URL (https://github.com/user/repo.git).
	if strings.HasPrefix(source, "https://github.com/") {
		return parseGitHubHTTPS(source, ref)
	}

	// Format 3: SSH URL (git@github.com:user/repo.git).
	if strings.HasPrefix(source, "git@github.com:") {
		return parseGitHubSSH(source, ref)
	}

	return nil, fmt.Errorf("%w: unsupported source format: %s", ErrInvalidSource, source)
}

// parseGitHubShorthand parses github.com/user/repo format.
func parseGitHubShorthand(source, ref string) (*SourceInfo, error) {
	parts := strings.Split(strings.TrimPrefix(source, "github.com/"), "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("%w: invalid GitHub shorthand format (expected github.com/user/repo)", ErrInvalidSource)
	}

	owner := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")

	return &SourceInfo{
		Type:     "github",
		Owner:    owner,
		Repo:     repo,
		Ref:      ref,
		URL:      fmt.Sprintf("https://github.com/%s/%s.git", owner, repo),
		FullPath: fmt.Sprintf("github.com/%s/%s", owner, repo),
		Name:     repo,
	}, nil
}

// parseGitHubHTTPS parses https://github.com/user/repo.git format.
func parseGitHubHTTPS(source, ref string) (*SourceInfo, error) {
	// Remove https://github.com/ prefix.
	remainder := strings.TrimPrefix(source, "https://github.com/")
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
		URL:      fmt.Sprintf("https://github.com/%s/%s.git", owner, repo),
		FullPath: fmt.Sprintf("github.com/%s/%s", owner, repo),
		Name:     repo,
	}, nil
}

// parseGitHubSSH parses git@github.com:user/repo.git format.
func parseGitHubSSH(source, ref string) (*SourceInfo, error) {
	// Remove git@github.com: prefix.
	remainder := strings.TrimPrefix(source, "git@github.com:")
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
		URL:      fmt.Sprintf("https://github.com/%s/%s.git", owner, repo), // Use HTTPS for cloning.
		FullPath: fmt.Sprintf("github.com/%s/%s", owner, repo),
		Name:     repo,
	}, nil
}
