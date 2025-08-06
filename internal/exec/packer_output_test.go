package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecutePackerOutput(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_BASE_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Info")

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            "nonprod",
		StackFile:        "",
		ComponentType:    "packer",
		ComponentFromArg: "aws/bastion",
		SubCommand:       "output",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	packerFlags := PackerFlags{}

	d, err := ExecutePackerOutput(&info, &packerFlags)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(d.(map[string]any)["builds"].([]any)))

	packerFlags.Query = ".builds[0].artifact_id | split(\":\")[1]"
	d, err = ExecutePackerOutput(&info, &packerFlags)
	assert.NoError(t, err)
	assert.Equal(t, "ami-0c2ca16b7fcac7529", d)

	packerFlags.Query = ".builds[0].artifact_id"
	d, err = ExecutePackerOutput(&info, &packerFlags)
	assert.NoError(t, err)
	assert.Equal(t, "us-east-2:ami-0c2ca16b7fcac7529", d)
}
