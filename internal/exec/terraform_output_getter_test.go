package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
)

func TestGetAllTerraformOutputs_PanicWhenNoExecutor(t *testing.T) {
	// Save and restore original executor.
	originalExecutor := tfoutput.GetDefaultExecutor()
	defer tfoutput.SetDefaultExecutor(originalExecutor)

	tfoutput.SetDefaultExecutor(nil)

	atmosConfig := &schema.AtmosConfiguration{}

	assert.PanicsWithValue(t,
		"output.SetDefaultExecutor must be called before GetComponentOutputs",
		func() {
			_, _ = GetAllTerraformOutputs(atmosConfig, "component", "stack", false, nil)
		},
	)
}

func TestGetTerraformOutput_PanicWhenNoExecutor(t *testing.T) {
	// Save and restore original executor.
	originalExecutor := tfoutput.GetDefaultExecutor()
	defer tfoutput.SetDefaultExecutor(originalExecutor)

	tfoutput.SetDefaultExecutor(nil)

	atmosConfig := &schema.AtmosConfiguration{}

	assert.PanicsWithValue(t,
		"output.SetDefaultExecutor must be called before GetOutput",
		func() {
			_, _, _ = GetTerraformOutput(atmosConfig, "stack", "component", "output", false, nil, nil)
		},
	)
}
