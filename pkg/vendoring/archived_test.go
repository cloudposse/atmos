package vendoring

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGithubArchivedChecker_NonGitHubSource_NotArchivedNoNetworkCall proves the production
// ArchivedChecker treats a non-GitHub Git URI as "not archived, no error" purely from
// ghclient.ParseOwnerRepo's host check, without ever reaching the network (GitHub's archived flag
// only applies to github.com repositories).
func TestGithubArchivedChecker_NonGitHubSource_NotArchivedNoNetworkCall(t *testing.T) {
	checker := &githubArchivedChecker{}

	archived, err := checker.IsArchived(context.Background(), "https://gitlab.com/cloudposse/terraform-aws-vpc.git")

	require.NoError(t, err)
	assert.False(t, archived)
}
