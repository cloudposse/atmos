package describe

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestDescribeAffectedWithTargetRefClone(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	// We are using `atmos.yaml` from this dir. This `atmos.yaml` has set base_path: "../../examples/tests",
	// which will be wrong for the remote repo which is cloned into a temp dir.
	// Set the correct base path for the cloned remote repo
	atmosConfig.BasePath = "./examples/tests"

	// Git reference and commit SHA
	// Refer to https://git-scm.com/book/en/v2/Git-Internals-Git-References for more details
	ref := "refs/heads/main"
	sha := ""

	affected, _, _, _, err := e.ExecuteDescribeAffectedWithTargetRefClone(
		atmosConfig,
		ref,
		sha,
		"",
		"",
		true,
		true,
		true,
		"",
	)
	assert.Nil(t, err)

	affectedYaml, err := u.ConvertToYAML(affected)
	assert.Nil(t, err)

	t.Log(fmt.Sprintf("\nAffected components and stacks:\n%v", affectedYaml))
}

func TestDescribeAffectedWithTargetRepoPath(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	// We are using `atmos.yaml` from this dir. This `atmos.yaml` has set base_path: "../../examples/tests",
	// which will be wrong for the remote repo which is cloned into a temp dir.
	// Set the correct base path for the cloned remote repo
	atmosConfig.BasePath = "./examples/tests"

	// Point to the same local repository
	// This will compare this local repository with itself as the remote target, which should result in an empty `affected` list
	repoPath := "../../"

	affected, _, _, _, err := e.ExecuteDescribeAffectedWithTargetRepoPath(
		atmosConfig,
		repoPath,
		true,
		true,
		true,
		"",
	)
	assert.Nil(t, err)

	affectedYaml, err := u.ConvertToYAML(affected)
	assert.Nil(t, err)

	t.Log(fmt.Sprintf("\nAffected components and stacks:\n%v", affectedYaml))
}
