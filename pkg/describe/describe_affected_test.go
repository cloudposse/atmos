package describe

import (
	"fmt"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	"testing"
)

func TestDescribeAffected(t *testing.T) {
	configAndStacksInfo := cfg.ConfigAndStacksInfo{}

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	// We are using `atmos.yaml` from this dir. This `atmos.yaml` has set base_path: "../../examples/complete",
	// which will be wrong for the remote repo which is cloned into a temp dir.
	// Set the correct base path for the cloned remote repo
	cliConfig.BasePath = "./examples/complete"

	// Git reference and commit SHA
	// Refer to https://git-scm.com/book/en/v2/Git-Internals-Git-References for more details
	ref := "refs/heads/master"
	sha := ""

	affected, err := e.ExecuteDescribeAffected(cliConfig, ref, sha, "", "", true)
	assert.Nil(t, err)

	affectedYaml, err := yaml.Marshal(affected)
	assert.Nil(t, err)

	t.Log(fmt.Sprintf("\nAffected components and stacks:\n%v", string(affectedYaml)))
}
