package exec

import (
	"testing"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// gcpServiceIDDescription is the Terraform `description` from the fixture component. It contains
// `Go` template delimiters that are meaningful to GCP, not to Atmos, and must survive verbatim.
const gcpServiceIDDescription = "Unique Identifier for the created service with format " +
	"projects/{{project}}/locations/{{location}}/services/{{name}}"

// TestProcessStacksDoesNotTemplateTerraformSource is a regression test for
// cloudposse/atmos#2145: a Terraform `description` containing `{{project}}` made
// `atmos terraform plan` fail with
// `template: templates-all-atmos-sections:NNN: function "project" not defined`.
//
// The Terraform source is parsed by `terraform-config-inspect` into
// `component_info.terraform_config`, which is part of the component section that Atmos renders
// as a `Go` template. Terraform source code is not an Atmos stack manifest, so it must never be
// rendered.
//
// The test reproduces the production call sequence: `ProcessStacks` is first invoked without
// template processing (hooks and auth config resolution do this), which stores `component_info`
// in the cached stacks map, and then invoked again with template processing enabled.
func TestProcessStacksDoesNotTemplateTerraformSource(t *testing.T) {
	// Clear env vars that would otherwise point Atmos at a different config.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
	t.Setenv("ATMOS_BASE_PATH", "")

	t.Chdir("../../tests/fixtures/scenarios/terraform-source-go-templates")

	// The stacks map is cached across `ProcessStacks` calls; start from a clean cache so this
	// test is independent of whatever other tests in the package loaded before it.
	ClearFindStacksMapCache()
	t.Cleanup(ClearFindStacksMapCache)

	newInfo := func() schema.ConfigAndStacksInfo {
		return schema.ConfigAndStacksInfo{
			ComponentFromArg: "gcp-service",
			Stack:            "dev",
			ComponentType:    cfg.TerraformComponentType,
		}
	}

	atmosConfig, err := cfg.InitCliConfig(newInfo(), true)
	require.NoError(t, err)
	require.True(t, atmosConfig.Templates.Settings.Enabled, "the fixture must enable templating for this regression to be meaningful")

	// First pass without templates: this is what populates `component_info` in the shared stacks map.
	_, err = ProcessStacks(&atmosConfig, newInfo(), true, false, false, nil, nil)
	require.NoError(t, err)

	// Second pass with templates: this is where #2145 failed.
	result, err := ProcessStacks(&atmosConfig, newInfo(), true, true, false, nil, nil)
	require.NoError(t, err, "Terraform source code must not be processed as a Go template")

	// Stack manifest templates must still be rendered.
	assert.Equal(t, "dev-ok", result.ComponentSettingsSection["rendered"])

	// The Terraform `description` must survive verbatim, delimiters included.
	componentInfo, ok := result.ComponentSection[componentInfoKey].(map[string]any)
	require.True(t, ok, "component_info should be a map")

	terraformConfig, ok := componentInfo[terraformConfigKey]
	require.True(t, ok, "component_info should contain terraform_config")

	description := terraformOutputDescription(t, terraformConfig, "service_id")
	assert.Equal(t, gcpServiceIDDescription, description)
}

// terraformOutputDescription digs the description of a Terraform output out of the
// `terraform_config` section, which is a `*tfconfig.Module` value.
func terraformOutputDescription(t *testing.T, terraformConfig any, outputName string) string {
	t.Helper()

	module, ok := terraformConfig.(*tfconfig.Module)
	require.True(t, ok, "terraform_config should be a *tfconfig.Module, got %T", terraformConfig)

	output, ok := module.Outputs[outputName]
	require.True(t, ok, "terraform_config should describe the %q output", outputName)

	return output.Description
}

func TestSplitNonTemplatedSections(t *testing.T) {
	t.Run("removes the non-templated sections without touching the input", func(t *testing.T) {
		componentInfo := map[string]any{"component_type": "terraform"}
		componentSection := map[string]any{
			"vars":           map[string]any{"stage": "dev"},
			componentInfoKey: componentInfo,
		}

		filtered, excluded := splitNonTemplatedSections(componentSection)

		assert.NotContains(t, filtered, componentInfoKey)
		assert.Contains(t, filtered, "vars")
		assert.Equal(t, map[string]any{componentInfoKey: componentInfo}, excluded)

		// The input must be left intact: callers still build the template context from it.
		assert.Contains(t, componentSection, componentInfoKey)

		// Mutating the result must not affect the input (result -> src isolation).
		filtered["vars"] = "mutated"
		assert.Equal(t, map[string]any{"stage": "dev"}, componentSection["vars"])

		// Mutating the input must not affect the result (src -> result isolation).
		componentSection["added"] = true
		assert.NotContains(t, filtered, "added")
	})

	t.Run("returns the section unchanged when there is nothing to exclude", func(t *testing.T) {
		componentSection := map[string]any{"vars": map[string]any{"stage": "dev"}}

		filtered, excluded := splitNonTemplatedSections(componentSection)

		assert.Nil(t, excluded)
		assert.Equal(t, componentSection, filtered)
	})
}

func TestRestoreNonTemplatedSections(t *testing.T) {
	t.Run("puts the excluded sections back", func(t *testing.T) {
		componentInfo := map[string]any{"component_type": "terraform"}
		processed := map[string]any{"vars": map[string]any{"stage": "dev"}}

		restoreNonTemplatedSections(processed, map[string]any{componentInfoKey: componentInfo})

		assert.Equal(t, componentInfo, processed[componentInfoKey])
	})

	t.Run("is a no-op when nothing was excluded", func(t *testing.T) {
		processed := map[string]any{"vars": map[string]any{"stage": "dev"}}

		restoreNonTemplatedSections(processed, nil)

		assert.Equal(t, map[string]any{"vars": map[string]any{"stage": "dev"}}, processed)
	})
}
