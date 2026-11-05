package azuredevops

import (
	"net/url"
	"strings"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
)

var _ atmosgit.PullRequestRepositoryResolver = (*Provider)(nil)

// RepositoryFromURL recognizes Azure DevOps cloud remotes and returns their organization
// (Owner), project (Namespace), and repository:
//
//	https://[user@]dev.azure.com/{org}/{project}/_git/{repo}
//	git@ssh.dev.azure.com:v3/{org}/{project}/{repo}
//
// Legacy {org}.visualstudio.com hosts and Azure DevOps Server are not recognized.
func (p *Provider) RepositoryFromURL(uri string) (atmosgit.PullRequestRepository, bool) {
	segments, ok := httpsSegments(uri)
	if !ok {
		segments, ok = sshSegments(uri)
	}
	if !ok {
		return atmosgit.PullRequestRepository{}, false
	}

	// Path-unescape each segment (project names may contain spaces) and reject empty ones.
	for i, raw := range segments {
		value, err := url.PathUnescape(raw)
		if err != nil || value == "" {
			return atmosgit.PullRequestRepository{}, false
		}
		segments[i] = value
	}
	return atmosgit.PullRequestRepository{Owner: segments[0], Namespace: []string{segments[1]}, Repository: segments[2]}, true
}

// httpsSegments returns {org, project, repo} from https://dev.azure.com/{org}/{project}/_git/{repo}.
func httpsSegments(uri string) ([]string, bool) {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "dev.azure.com") {
		return nil, false
	}
	segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(segments) != 4 || segments[2] != "_git" {
		return nil, false
	}
	return []string{segments[0], segments[1], segments[3]}, true
}

// sshSegments returns {org, project, repo} from <user>@ssh.dev.azure.com:v3/{org}/{project}/{repo}.
func sshSegments(uri string) ([]string, bool) {
	_, rest, found := strings.Cut(uri, "@")
	if !found {
		return nil, false
	}
	host, path, _ := strings.Cut(rest, ":")
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if !strings.EqualFold(host, "ssh.dev.azure.com") || len(segments) != 4 || segments[0] != "v3" {
		return nil, false
	}
	return segments[1:], true
}
