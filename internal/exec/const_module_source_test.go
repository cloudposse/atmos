package exec

import (
	"testing"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// constModuleSourceFixture is a fixture whose `testme` component declares a static variable
// interpolated in a local module source, which is valid Terraform 1.15+ and OpenTofu 1.8+.
const constModuleSourceFixture = "../../tests/fixtures/scenarios/const-module-source"

// TestDescribeComponentWithInterpolatedModuleSource asserts that `describe component` succeeds for a
// component whose `module.source` interpolates a `const` variable.
//
// Before hashicorp/terraform-config-inspect#146, this failed with
// "failed to load terraform component ... Variables not allowed".
func TestDescribeComponentWithInterpolatedModuleSource(t *testing.T) {
	t.Chdir(constModuleSourceFixture)

	sections, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "testme",
		Stack:                "test",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
	})

	require.NoError(t, err, "an interpolated `module.source` must not fail component loading")
	require.NotNil(t, sections)

	componentInfo, ok := sections["component_info"].(map[string]any)
	require.True(t, ok, "component_info section should be present")

	terraformConfig := componentInfo[terraformConfigKey]
	require.NotNil(t, terraformConfig, "%s should not be nil for a loadable component", terraformConfigKey)

	module, ok := terraformConfig.(*tfconfig.Module)
	require.True(t, ok, "%s should be a parsed Terraform module", terraformConfigKey)
	assert.Contains(t, module.Variables, "org", "the `const` variable should be parsed")
	assert.Contains(t, module.Outputs, "greeting", "outputs should be parsed")
}

// TestDescribeComponentWithGenuineSyntaxErrorStillFails asserts that tolerating interpolated module
// sources does not make Atmos tolerant of real syntax errors in `variable` blocks.
func TestDescribeComponentWithGenuineSyntaxErrorStillFails(t *testing.T) {
	t.Chdir(constModuleSourceFixture)

	_, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "broken-variable",
		Stack:                "test",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
	})

	require.Error(t, err, "a genuine HCL syntax error must still fail")
	assert.ErrorIs(t, err, errUtils.ErrFailedToLoadTerraformComponent)
}
