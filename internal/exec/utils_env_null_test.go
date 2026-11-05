package exec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/function/literalsource"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProcessStacksRestoredEnvironmentOmitsNulls(t *testing.T) {
	for _, templates := range []bool{false, true} {
		for _, functions := range []bool{false, true} {
			t.Run(fmtBoolPair(templates, functions), func(t *testing.T) {
				writeManifestLoadFixture(t, "dev.yaml", `vars:
  stage: dev
components:
  terraform:
    app:
      env:
        TOKEN: null
        EMPTY: ""
        DISABLED: false
        ZERO: 0
        LITERAL: !literal "!env ATMOS_ENV_LITERAL_VALUE"
`)
				t.Setenv("ATMOS_ENV_LITERAL_VALUE", "must-not-resolve")
				config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
				require.NoError(t, err)
				before := literalSourceCacheSnapshot(t, &config)
				for range 2 {
					info, err := ProcessStacks(&config, schema.ConfigAndStacksInfo{
						ComponentFromArg: "app", ComponentType: cfg.TerraformComponentType,
						Stack: "dev", StackFromArg: "dev",
					}, true, templates, functions, nil, nil)
					require.NoError(t, err)
					assert.NotContains(t, info.ComponentEnvSection, "TOKEN")
					assert.Equal(t, "", info.ComponentEnvSection["EMPTY"])
					assert.Equal(t, false, info.ComponentEnvSection["DISABLED"])
					assert.EqualValues(t, 0, info.ComponentEnvSection["ZERO"])
					assert.Equal(t, "!env ATMOS_ENV_LITERAL_VALUE", info.ComponentEnvSection["LITERAL"])
					assert.NotContains(t, info.ComponentEnvList, "TOKEN=<nil>")
					// Execution paths rebuild this list from the refreshed map.
					info.ComponentEnvList = nil
					ConvertComponentEnvSectionToList(&info)
					assert.ElementsMatch(t, []string{"EMPTY=", "DISABLED=false", "ZERO=0", "LITERAL=!env ATMOS_ENV_LITERAL_VALUE"}, info.ComponentEnvList)
					info.ComponentEnvSection["LITERAL"] = "mutated output"
					sourceEnv := info.ComponentSection[cfg.EnvSectionName].(map[string]any)
					require.Contains(t, sourceEnv, "TOKEN")
					assert.Nil(t, sourceEnv["TOKEN"])
					assert.Equal(t, "!env ATMOS_ENV_LITERAL_VALUE", sourceEnv["LITERAL"])
					assert.JSONEq(t, before, literalSourceCacheSnapshot(t, &config))
				}
			})
		}
	}
}

func TestPostProcessEnvironmentNullFilteringIsolation(t *testing.T) {
	original := map[string]any{"TOKEN": nil, "EMPTY": "", "DISABLED": false, "ZERO": 0, "LITERAL": literalsource.Encode("{{external.value}}")}
	before, err := json.Marshal(original)
	require.NoError(t, err)
	info := schema.ConfigAndStacksInfo{ComponentSection: map[string]any{cfg.EnvSectionName: original}}
	// Match the final refresh boundary after literal restoration.
	info.ComponentSection = literalsource.RestoreMap(info.ComponentSection)
	for range 2 {
		postProcessTemplatesAndYamlFunctions(&info)
		assert.Equal(t, map[string]any{"EMPTY": "", "DISABLED": false, "ZERO": 0, "LITERAL": "{{external.value}}"}, info.ComponentEnvSection)
	}
	source := info.ComponentSection[cfg.EnvSectionName].(map[string]any)
	require.Contains(t, source, "TOKEN", "filtering the execution view must not remove source values")
	info.ComponentEnvSection["EMPTY"] = "changed result"
	assert.Equal(t, "", source["EMPTY"])
	source["ZERO"] = 42
	assert.Equal(t, 0, info.ComponentEnvSection["ZERO"])
	after, err := json.Marshal(original)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after), "the protected input must remain unchanged")
}
