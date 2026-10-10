package azuredevops

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestParseRepositoryURL verifies the Azure DevOps HTTPS and SSH URL forms parse into their
// organization, project, and repository, and that other URLs are rejected.
func TestParseRepositoryURL(t *testing.T) {
	tests := []struct {
		name               string
		uri                string
		org, project, repo string
		ok                 bool
	}{
		{"dev.azure.com https", "https://dev.azure.com/acme/platform/_git/deployments", "acme", "platform", "deployments", true},
		{"dev.azure.com https with user", "https://acme@dev.azure.com/acme/platform/_git/deployments", "acme", "platform", "deployments", true},
		{"escaped project", "https://dev.azure.com/acme/Platform%20Team/_git/deployments", "acme", "Platform Team", "deployments", true},
		{"visualstudio.com", "https://acme.visualstudio.com/platform/_git/deployments", "acme", "platform", "deployments", true},
		{"visualstudio.com DefaultCollection", "https://acme.visualstudio.com/DefaultCollection/platform/_git/deployments", "acme", "platform", "deployments", true},
		{"ssh dev.azure.com", "git@ssh.dev.azure.com:v3/acme/platform/deployments", "acme", "platform", "deployments", true},
		{"ssh visualstudio.com", "acme@vs-ssh.visualstudio.com:v3/acme/platform/deployments", "acme", "platform", "deployments", true},
		{"ssh url form", "ssh://git@ssh.dev.azure.com/v3/acme/platform/deployments", "acme", "platform", "deployments", true},
		{"ssh unknown host", "git@example.com:v3/acme/platform/deployments", "", "", "", false},
		{"scp without user", "ssh.dev.azure.com:v3/acme/platform/deployments", "", "", "", false},
		{"scp without path", "git@ssh.dev.azure.com", "", "", "", false},
		{"ssh empty segment", "git@ssh.dev.azure.com:v3/acme//deployments", "", "", "", false},
		{"ssh bad escape", "git@ssh.dev.azure.com:v3/acme/%zz/deployments", "", "", "", false},
		{"https bad url", "https://dev.azure.com/acme/%zz/_git/deployments", "", "", "", false},
		{"visualstudio.com missing _git", "https://acme.visualstudio.com/platform/deployments", "", "", "", false},
		{"unsupported scheme", "git://dev.azure.com/acme/platform/_git/deployments", "", "", "", false},
		{"ssh missing v3", "git@ssh.dev.azure.com:acme/platform/deployments", "", "", "", false},
		{"github https", "https://github.com/acme/deployments.git", "", "", "", false},
		{"github ssh", "git@github.com:acme/deployments.git", "", "", "", false},
		{"local path", "/tmp/origin.git", "", "", "", false},
		{"dev.azure.com missing _git", "https://dev.azure.com/acme/platform/deployments", "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, ok := ParseRepositoryURL(tt.uri)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, Repository{Organization: tt.org, Project: tt.project, Name: tt.repo}, repo)
		})
	}
}
