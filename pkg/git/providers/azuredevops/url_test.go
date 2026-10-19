package azuredevops

import (
	"testing"

	"github.com/stretchr/testify/assert"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
)

// TestRepositoryFromURL verifies the Azure DevOps HTTPS and SSH URL forms resolve to their
// organization, project, and repository, and that other URLs are rejected.
func TestRepositoryFromURL(t *testing.T) {
	want := atmosgit.PullRequestRepository{Owner: "acme", Namespace: []string{"platform"}, Repository: "deployments"}
	tests := []struct {
		name string
		uri  string
		want atmosgit.PullRequestRepository
		ok   bool
	}{
		{"https", "https://dev.azure.com/acme/platform/_git/deployments", want, true},
		{"https with user", "https://acme@dev.azure.com/acme/platform/_git/deployments", want, true},
		{"https host is case-insensitive", "https://DEV.azure.com/acme/platform/_git/deployments", want, true},
		{
			"escaped project", "https://dev.azure.com/acme/Platform%20Team/_git/deployments",
			atmosgit.PullRequestRepository{Owner: "acme", Namespace: []string{"Platform Team"}, Repository: "deployments"},
			true,
		},
		{"ssh", "git@ssh.dev.azure.com:v3/acme/platform/deployments", want, true},
		{"ssh unknown host", "git@example.com:v3/acme/platform/deployments", atmosgit.PullRequestRepository{}, false},
		{"ssh missing v3", "git@ssh.dev.azure.com:acme/platform/deployments", atmosgit.PullRequestRepository{}, false},
		{"ssh without path", "git@ssh.dev.azure.com", atmosgit.PullRequestRepository{}, false},
		{"ssh empty segment", "git@ssh.dev.azure.com:v3/acme//deployments", atmosgit.PullRequestRepository{}, false},
		{"ssh bad escape", "git@ssh.dev.azure.com:v3/acme/%zz/deployments", atmosgit.PullRequestRepository{}, false},
		{"https bad escape", "https://dev.azure.com/acme/%zz/_git/deployments", atmosgit.PullRequestRepository{}, false},
		{"https missing _git", "https://dev.azure.com/acme/platform/deployments", atmosgit.PullRequestRepository{}, false},
		{"legacy visualstudio.com is not recognized", "https://acme.visualstudio.com/platform/_git/deployments", atmosgit.PullRequestRepository{}, false},
		{"unsupported scheme", "git://dev.azure.com/acme/platform/_git/deployments", atmosgit.PullRequestRepository{}, false},
		{"github https", "https://github.com/acme/deployments.git", atmosgit.PullRequestRepository{}, false},
		{"github ssh", "git@github.com:acme/deployments.git", atmosgit.PullRequestRepository{}, false},
		{"local path", "/tmp/origin.git", atmosgit.PullRequestRepository{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := New().RepositoryFromURL(tt.uri)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
