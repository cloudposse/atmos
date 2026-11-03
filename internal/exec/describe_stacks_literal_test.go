package exec

import (
	"encoding/json"
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/function/literalsource"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDescribeStacksLiteralSources(t *testing.T) {
	for _, templates := range []bool{false, true} {
		for _, functions := range []bool{false, true} {
			t.Run(fmtBoolPair(templates, functions), func(t *testing.T) {
				writeManifestLoadFixture(t, "dev.yaml", `vars:
  stage: dev
components:
  terraform:
    app:
      vars:
        scalar: !literal "{{external.email}}"
        nested:
          values: [!literal "!env EXTERNAL", plain]
`)
				config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
				require.NoError(t, err)
				before := literalSourceCacheSnapshot(t, &config)
				typeSection := literalComponentWithSources(t, &config)
				sourceBefore, err := json.Marshal(typeSection)
				require.NoError(t, err)
				for range 2 {
					processor := newDescribeStacksProcessor(&config, "dev", nil, nil, nil, templates, functions, false, nil, nil)
					require.NoError(t, processor.processComponentTypeSection("deploy/dev", "", cfg.TerraformSectionName, typeSection, processComponentTypeOpts{}))
					got := processor.finalStacksMap
					app := got["dev"].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)["app"].(map[string]any)
					sources, ok := app[sourcesSectionName].(schema.ConfigSources)
					require.True(t, ok, "expected typed provenance, got %T", app[sourcesSectionName])
					assert.Equal(t, "{{external.email}}", sources["vars"]["scalar"].FinalValue)
					wantNested := map[string]any{"values": []any{"!env EXTERNAL", "plain"}}
					assert.Equal(t, wantNested, sources["vars"]["nested"].FinalValue)
					dependencies := sources["vars"]["nested"].StackDependencies
					require.NotEmpty(t, dependencies)
					for _, dependency := range dependencies {
						assert.Equal(t, wantNested, dependency.VariableValue)
					}
					output, err := json.Marshal(got)
					require.NoError(t, err)
					assert.NotContains(t, string(output), "ATMOS_LITERAL_V1:")
					// Returned provenance must be independent of both cache maps and slices.
					sources["vars"]["nested"].FinalValue.(map[string]any)["values"] = "changed final"
					dependencies[0].VariableValue.(map[string]any)["values"] = "changed dependency"
					dependencies[0].StackFile = "changed file"
					sourceAfter, err := json.Marshal(typeSection)
					require.NoError(t, err)
					assert.JSONEq(t, string(sourceBefore), string(sourceAfter))
					assert.JSONEq(t, before, literalSourceCacheSnapshot(t, &config))
				}
			})
		}
	}
}

// literalComponentWithSources attaches the same typed provenance produced by
// describe component to a copied stack component, leaving the source cache intact.
func literalComponentWithSources(t *testing.T, config *schema.AtmosConfiguration) map[string]any {
	t.Helper()
	stacks, raw, _, err := FindStacksMap(config, false)
	require.NoError(t, err)
	component := maps.Clone(stacks["deploy/dev"].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)["app"].(map[string]any))
	info := buildConfigAndStacksInfo("app", "deploy/dev", "", extractDescribeComponentSections(component))
	info.StackFile = "deploy/dev"
	info.ComponentType = cfg.TerraformComponentType
	sources, err := ProcessConfigSources(info, raw)
	require.NoError(t, err)
	component[sourcesSectionName] = sources
	return map[string]any{"app": component}
}

func literalSourceCacheSnapshot(t *testing.T, config *schema.AtmosConfiguration) string {
	t.Helper()
	stacks, raw, _, err := FindStacksMap(config, false)
	require.NoError(t, err)
	data, err := json.Marshal([]any{stacks, raw})
	require.NoError(t, err)
	return string(data)
}

func TestDescribeStacksLiteralNamePattern(t *testing.T) {
	for _, templates := range []bool{false, true} {
		for _, functions := range []bool{false, true} {
			t.Run(fmtBoolPair(templates, functions), func(t *testing.T) {
				writeManifestLoadFixture(t, "dev.yaml", `vars:
  stage: !literal dev
  environment: !literal ue2
components:
  terraform:
    app:
      vars:
        value: !literal "{{external.email}}"
`)
				config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
				require.NoError(t, err)
				config.Stacks.NameTemplate = ""
				config.Stacks.NamePattern = "{environment}-{stage}"
				before := literalSourceCacheSnapshot(t, &config)
				for range 2 {
					got, err := ExecuteDescribeStacks(&config, "ue2-dev", nil, nil, nil, false, templates, functions, false, nil, nil)
					require.NoError(t, err)
					require.Contains(t, got, "ue2-dev", "logical stack selector must match decoded literal values")
					app := got["ue2-dev"].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)["app"].(map[string]any)
					assert.Equal(t, "ue2-dev", app["workspace"])
					assert.Equal(t, "dev", app["vars"].(map[string]any)["stage"])
					assert.Equal(t, "ue2", app["vars"].(map[string]any)["environment"])
					assert.JSONEq(t, before, literalSourceCacheSnapshot(t, &config))
				}
			})
		}
	}
}

func TestResolveStackNameLiteralContext(t *testing.T) {
	for _, tc := range []struct {
		name, pattern, want string
	}{
		{name: "pattern", pattern: "{environment}-{stage}", want: "ue2-dev"},
		{name: "missing tenant fallback", pattern: "{tenant}-{environment}-{stage}", want: "deploy/dev"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{Stacks: schema.Stacks{NamePattern: tc.pattern}}
			vars := map[string]any{
				"stage": literalsource.Encode("dev"), "environment": literalsource.Encode("ue2"),
				"attributes": []any{literalsource.Encode("{{external.attribute}}")},
			}
			before, err := json.Marshal(vars)
			require.NoError(t, err)
			name, context, err := ResolveStackName(config, "deploy/dev", "", schema.ConfigAndStacksInfo{}, vars)
			require.NoError(t, err)
			assert.Equal(t, tc.want, name)
			assert.Equal(t, "dev", context.Stage)
			assert.Equal(t, "ue2", context.Environment)
			assert.Equal(t, []any{"{{external.attribute}}"}, context.Attributes)
			context.Attributes[0] = "changed output"
			after, err := json.Marshal(vars)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after), "name resolution must not modify protected source values")
		})
	}
}
