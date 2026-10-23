package exec

import (
	"fmt"
	"testing"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFieldTestSkipTaggedFunctions(t *testing.T) {
	for _, skip := range [][]string{{"env", "terraform.output", "ci.labels"}, {"!env", "!terraform.output", "!ci.labels"}} {
		input := map[string]any{"environment": "!env ATMOS_FIELD_SKIP", "state": "!terraform.output nonexistent output", "registry": "!ci.labels []"}
		t.Setenv("ATMOS_FIELD_SKIP", "must-not-resolve")
		got, err := ProcessCustomYamlTags(&schema.AtmosConfiguration{}, input, "dev", skip, nil)
		require.NoError(t, err)
		assert.Equal(t, input, got)
	}
}

func TestFieldTestWorkspacePattern(t *testing.T) {
	writeManifestLoadFixture(t, "dev.yaml", `vars:
  tenant: team
  environment: local
  stage: dev
components:
  terraform:
    app:
      metadata:
        terraform_workspace_pattern: "{tenant}-{environment}-{stage}"
`)
	got, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{Component: "app", Stack: "dev", ProcessTemplates: true, ProcessYamlFunctions: true})
	require.NoError(t, err)
	assert.Equal(t, "team-local-dev", got["workspace"])
}

func TestFieldTestStackDelimiters(t *testing.T) {
	for index, delimiters := range [][2]string{{"<<", ">>"}, {"'{{", "}}'"}, {"[[", "]]"}} {
		t.Run(fmt.Sprintf("pair-%d", index), func(t *testing.T) {
			writeManifestLoadFixture(t, "dev.yaml", fmt.Sprintf(`vars:
  stage: dev
components:
  terraform:
    app:
      settings:
        templates:
          settings:
            delimiters: [%q, %q]
            evaluations: 2
      vars:
        rendered: %q
        function_string: %q
`, delimiters[0], delimiters[1], delimiters[0]+" .vars.stage "+delimiters[1], "!env "+delimiters[0]+" .vars.stage "+delimiters[1]))
			component, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{Component: "app", Stack: "dev", ProcessTemplates: true, ProcessYamlFunctions: false})
			require.NoError(t, err)
			assert.Equal(t, "dev", component["vars"].(map[string]any)["rendered"])
			assert.Equal(t, "!env dev", component["vars"].(map[string]any)["function_string"])
			config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
			require.NoError(t, err)
			config.Templates.Settings.Evaluations = 2
			got, err := ExecuteDescribeStacks(&config, "dev", nil, nil, nil, false, true, false, false, nil, nil)
			require.NoError(t, err)
			stack := got["dev"].(map[string]any)
			app := stack["components"].(map[string]any)["terraform"].(map[string]any)["app"].(map[string]any)
			assert.Equal(t, "dev", app["vars"].(map[string]any)["rendered"])
			assert.Equal(t, "!env dev", app["vars"].(map[string]any)["function_string"])
			settings := app["settings"].(map[string]any)["templates"].(map[string]any)["settings"].(map[string]any)
			assert.Equal(t, []any{delimiters[0], delimiters[1]}, settings["delimiters"])
		})
	}
}

func TestFieldTestWorkspaceContextNamingStrategies(t *testing.T) {
	for _, strategy := range []string{"manifest", "template", "pattern", "filename"} {
		t.Run(strategy, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			manifest := ""
			switch strategy {
			case "manifest":
				manifest = "dev"
			case "template":
				config.Stacks.NameTemplate = "{{ .vars.stage }}"
			case "pattern":
				config.Stacks.NamePattern = "{stage}"
			}
			vars := map[string]any{"tenant": "team", "environment": "local", "stage": "dev"}
			info := schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "app", ComponentVarsSection: vars, ComponentSection: map[string]any{"vars": vars}, ComponentMetadataSection: map[string]any{"terraform_workspace_pattern": "{tenant}-{environment}-{stage}-{component}"}}
			require.NoError(t, processStackContextPrefix(config, &info, "dev", manifest))
			workspace, err := BuildTerraformWorkspace(config, info)
			require.NoError(t, err)
			assert.Equal(t, "team-local-dev-app", workspace)
		})
	}
}
