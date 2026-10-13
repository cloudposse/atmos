package azuredevops

import (
	"net/url"
	"strings"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
)

// repository addresses an Azure DevOps Git repository by its three segments.
type repository struct {
	Organization string
	Project      string
	Name         string
}

// parseRepositoryURL extracts the organization, project, and repository from an Azure DevOps
// Git remote URL. It recognizes the cloud HTTPS and SSH forms:
//
//	https://[user@]dev.azure.com/{org}/{project}/_git/{repo}
//	https://{org}.visualstudio.com/[DefaultCollection/]{project}/_git/{repo}
//	git@ssh.dev.azure.com:v3/{org}/{project}/{repo}
//	{org}@vs-ssh.visualstudio.com:v3/{org}/{project}/{repo}
//	ssh://git@ssh.dev.azure.com/v3/{org}/{project}/{repo}
//
// ok is false for any other URL, including Azure DevOps Server on a custom host.
func parseRepositoryURL(uri string) (repository, bool) {
	if !strings.Contains(uri, "://") {
		return parseSCPURL(uri)
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		return repository{}, false
	}
	host := strings.ToLower(parsed.Hostname())
	segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	switch parsed.Scheme {
	case "https":
		if host == "dev.azure.com" {
			return parseDevAzurePath(segments)
		}
		if org, isLegacy := strings.CutSuffix(host, ".visualstudio.com"); isLegacy {
			return parseVisualStudioPath(org, segments)
		}
	case "ssh":
		return parseSSHPath(host, segments)
	}
	return repository{}, false
}

// parseSCPURL handles the scp-like SSH form: <user>@<host>:v3/{org}/{project}/{repo}.
func parseSCPURL(uri string) (repository, bool) {
	_, rest, found := strings.Cut(uri, "@")
	if !found {
		return repository{}, false
	}
	host, path, found := strings.Cut(rest, ":")
	if !found {
		return repository{}, false
	}
	return parseSSHPath(strings.ToLower(host), strings.Split(strings.Trim(path, "/"), "/"))
}

// parseSSHPath parses the SSH hosts' v3/{org}/{project}/{repo} path.
func parseSSHPath(host string, segments []string) (repository, bool) {
	if host != "ssh.dev.azure.com" && host != "vs-ssh.visualstudio.com" {
		return repository{}, false
	}
	if len(segments) != 4 || segments[0] != "v3" {
		return repository{}, false
	}
	return newRepository(segments[1], segments[2], segments[3])
}

// parseDevAzurePath parses dev.azure.com's {org}/{project}/_git/{repo} path.
func parseDevAzurePath(segments []string) (repository, bool) {
	if len(segments) != 4 || segments[2] != "_git" {
		return repository{}, false
	}
	return newRepository(segments[0], segments[1], segments[3])
}

// parseVisualStudioPath parses {org}.visualstudio.com's [DefaultCollection/]{project}/_git/{repo}
// path; the organization is the subdomain.
func parseVisualStudioPath(org string, segments []string) (repository, bool) {
	if len(segments) == 4 && strings.EqualFold(segments[0], "DefaultCollection") {
		segments = segments[1:]
	}
	if len(segments) != 3 || segments[1] != "_git" {
		return repository{}, false
	}
	return newRepository(org, segments[0], segments[2])
}

// newRepository path-unescapes each segment (project names may contain spaces) and rejects
// empty or malformed segments.
func newRepository(rawOrg, rawProject, rawName string) (repository, bool) {
	values := make([]string, 0, 3)
	for _, raw := range []string{rawOrg, rawProject, rawName} {
		value, err := url.PathUnescape(raw)
		if err != nil || value == "" {
			return repository{}, false
		}
		values = append(values, value)
	}
	return repository{Organization: values[0], Project: values[1], Name: values[2]}, true
}

var _ atmosgit.PullRequestRepositoryResolver = (*Provider)(nil)

// RepositoryFromURL recognizes Azure DevOps cloud remotes over HTTPS or SSH and returns their
// organization (Owner), project (Namespace), and repository.
func (p *Provider) RepositoryFromURL(uri string) (atmosgit.PullRequestRepository, bool) {
	repo, ok := parseRepositoryURL(uri)
	if !ok {
		return atmosgit.PullRequestRepository{}, false
	}
	return atmosgit.PullRequestRepository{Owner: repo.Organization, Namespace: []string{repo.Project}, Repository: repo.Name}, true
}
