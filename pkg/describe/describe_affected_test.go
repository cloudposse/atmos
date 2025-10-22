package describe

import (
	"testing"

	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/tests"
)

func TestDescribeAffectedWithTargetRefClone(t *testing.T) {
	// Skip long tests in short mode (this test takes ~36 seconds due to Git cloning)
	tests.SkipIfShort(t)

	// Check for Git repository with valid remotes and GitHub access (for cloning)
	tests.RequireGitRemoteWithValidURL(t)
	tests.RequireGitHubAccess(t)
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	// We are using `atmos.yaml` from this dir. This `atmos.yaml` has set base_path: "../../tests/fixtures/scenarios/complete",
	// which will be wrong for the remote repo which is cloned into a temp dir.
	// Set the correct base path for the cloned remote repo
	atmosConfig.BasePath = "./tests/fixtures/scenarios/complete"

	// Git reference and commit SHA
	// Refer to https://git-scm.com/book/en/v2/Git-Internals-Git-References for more details
	ref := "refs/heads/main"
	sha := ""

	affected, _, _, _, err := e.ExecuteDescribeAffectedWithTargetRefClone(
		&atmosConfig,
		ref,
		sha,
		"",
		"",
		true,
		true,
		"",
		true,
		true,
		nil,
		false,
	)
	assert.Nil(t, err)

	affectedYaml, err := u.ConvertToYAML(affected)
	assert.Nil(t, err)

	t.Logf("\nAffected components and stacks:\n%v", affectedYaml)
}

func TestDescribeAffectedWithTargetRepoPath(t *testing.T) {
	// Check for Git repository with valid remotes precondition
	tests.RequireGitRemoteWithValidURL(t)

	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	// We are using `atmos.yaml` from this dir. This `atmos.yaml` has set base_path: "../../tests/fixtures/scenarios/complete",
	// which will be wrong for the remote repo which is cloned into a temp dir.
	// Set the correct base path for the cloned remote repo
	atmosConfig.BasePath = "./tests/fixtures/scenarios/complete"

	// Point to the same local repository
	// This will compare this local repository with itself as the remote target, which should result in an empty `affected` list
	repoPath := "../../"

	affected, _, _, _, err := e.ExecuteDescribeAffectedWithTargetRepoPath(
		&atmosConfig,
		repoPath,
		true,
		true,
		"",
		true,
		true,
		nil,
		false,
	)
	assert.Nil(t, err)

	affectedYaml, err := u.ConvertToYAML(affected)
	assert.Nil(t, err)

	t.Logf("\nAffected components and stacks:\n%v", affectedYaml)
}
