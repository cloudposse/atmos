package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	fntag "github.com/cloudposse/atmos/pkg/function/tag"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestExplicitYAMLTagsEdition(t *testing.T) {
	t.Setenv("ATMOS_TAG_TEST", "resolved")
	for _, pin := range []string{"", "2026-10-19", "2026-10-20", "2026-10-21", "2026-09", "2026-10", "2026"} {
		t.Run(pin, func(t *testing.T) {
			config := &schema.AtmosConfiguration{Edition: pin}
			strict, err := u.ExplicitYAMLTags(config)
			require.NoError(t, err)
			require.Equal(t, pin != "2026-10-19" && pin != "2026-09", strict)
			input, _, err := u.UnmarshalYAMLFromFileWithPositions[map[string]any](config, `
real: &real !env "ATMOS_TAG_TEST"
alias: *real
single: '!env ATMOS_TAG_TEST'
double: "!env ATMOS_TAG_TEST"
typed: !!str "!env ATMOS_TAG_TEST"
block: |-
  !env ATMOS_TAG_TEST
nested:
  keep: "!unset"
  remove: !unset
list: ["!unset", !env "ATMOS_TAG_TEST"]
`, "same-file.yaml")
			require.NoError(t, err)
			result, err := ProcessCustomYamlTags(config, input, "", nil, nil)
			require.NoError(t, err)
			require.Equal(t, "resolved", result["real"])
			require.Equal(t, "resolved", result["alias"])
			want := "resolved"
			if strict {
				want = "!env ATMOS_TAG_TEST"
			}
			for _, key := range []string{"single", "double", "typed", "block"} {
				require.Equal(t, want, result[key], key)
			}
			nested := result["nested"].(map[string]any)
			require.NotContains(t, nested, "remove")
			if strict {
				require.Equal(t, "!unset", nested["keep"])
				require.Equal(t, []any{"!unset", "resolved"}, result["list"])
			} else {
				require.Empty(t, nested)
				require.Equal(t, []any{"resolved"}, result["list"])
			}
		})
	}
}

func TestExplicitYAMLTagsNoExecutionFromStrings(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "executed")
	config := &schema.AtmosConfiguration{}
	input, err := u.UnmarshalYAMLFromFile[map[string]any](config, fmt.Sprintf("value: '!exec touch %s'\n", marker), "test.yaml")
	require.NoError(t, err)
	result, err := ProcessCustomYamlTags(config, input, "", nil, nil)
	require.NoError(t, err)
	require.Equal(t, input, result)
	_, err = os.Stat(marker)
	require.True(t, os.IsNotExist(err))
	t.Setenv("ATMOS_TAG_RETURN", "!exec touch "+marker)
	input, err = u.UnmarshalYAMLFromFile[map[string]any](config, "value: !env ATMOS_TAG_RETURN\n", "test.yaml")
	require.NoError(t, err)
	result, err = ProcessCustomYamlTags(config, input, "", nil, nil)
	require.NoError(t, err)
	_, err = ProcessCustomYamlTags(config, result, "", nil, nil)
	require.NoError(t, err)
	_, err = os.Stat(marker)
	require.True(t, os.IsNotExist(err))
}

func TestExplicitYAMLTagsMergeAndSerialization(t *testing.T) {
	t.Setenv("ATMOS_TAG_TEST", "resolved")
	config := &schema.AtmosConfiguration{}
	input, err := u.UnmarshalYAMLFromFile[map[string]any](config, "value: !env ATMOS_TAG_TEST\nliteral: '!env ATMOS_TAG_TEST'\n", "source.yaml")
	require.NoError(t, err)
	source, tagged := yamlsource.Decode(input["value"].(string))
	require.True(t, tagged)
	require.Equal(t, "source.yaml", source.File)
	require.Equal(t, 1, source.Line)
	text, err := u.ConvertToYAML(input)
	require.NoError(t, err)
	require.Contains(t, text, "value: !env")
	require.NotContains(t, text, "atmos-yaml-source:")
	roundtrip, err := u.UnmarshalYAML[map[string]any](text)
	require.NoError(t, err)
	result, err := ProcessCustomYamlTags(config, roundtrip, "", nil, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"value": "resolved", "literal": "!env ATMOS_TAG_TEST"}, result)
	merged, deferred, err := merge.MergeWithDeferred(config, []map[string]any{input, {"value": "!env ATMOS_TAG_TEST"}})
	require.NoError(t, err)
	require.NoError(t, merge.ApplyDeferredMerges(deferred, merged, config, NewComponentYAMLProcessor(config, "", nil, nil, nil)))
	require.Equal(t, "!env ATMOS_TAG_TEST", merged["value"])
}

// explicitYAMLFunctionsForTest models the loader boundary for unit fixtures
// constructed directly as Go maps instead of parsed from a YAML document.
func explicitYAMLFunctionsForTest(input map[string]any) map[string]any {
	return explicitYAMLValueForTest(input).(map[string]any)
}

func explicitYAMLValueForTest(value any) any {
	switch v := value.(type) {
	case string:
		if fntag.MatchesStringForm(v) {
			return taggedFunctionForTest(v)
		}
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, item := range v {
			result[key] = explicitYAMLValueForTest(item)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = explicitYAMLValueForTest(item)
		}
		return result
	}
	return value
}

func taggedFunctionForTest(expression string) string {
	tag, argument, _ := strings.Cut(expression, " ")
	if !strings.Contains(expression, " ") {
		for _, name := range fntag.StringFormYAML() {
			if fntag.MatchesTag(expression, name) {
				tag, argument = name, strings.TrimPrefix(expression, name)
				break
			}
		}
	}
	return (yamlsource.Source{Tag: tag, Argument: argument}).Encode()
}

func TestExplicitYAMLTagsWholeUnsetSection(t *testing.T) {
	for _, strict := range []bool{false, true} {
		require.Equal(t, !strict, isUnsetSection("!unset", strict))
		require.True(t, isUnsetSection(taggedFunctionForTest("!unset"), strict))
	}
	config := &schema.AtmosConfiguration{}
	for _, value := range []string{`"!unset"`, `!unset`} {
		input, err := u.UnmarshalYAMLFromFile[map[string]any](config, "vars: "+value, "unset.yaml")
		require.NoError(t, err)
		opts := &ComponentProcessorOptions{AtmosConfig: config, ComponentMap: input}
		result := &ComponentProcessorResult{}
		err = extractComponentSections(opts, result)
		if value == `"!unset"` {
			require.Error(t, err, "literal text cannot delete a map section")
			require.Empty(t, result.ComponentUnsetSections)
		} else {
			require.NoError(t, err)
			require.Equal(t, []string{"vars"}, result.ComponentUnsetSections)
		}
	}
}

func TestExplicitYAMLTagsAllQuotedFunctionsAreLiteral(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	for _, tag := range fntag.AllYAML() {
		t.Run(tag, func(t *testing.T) {
			expression := tag + " arbitrary arguments"
			input, err := u.UnmarshalYAMLFromFile[map[string]any](config, fmt.Sprintf("value: %q\n", expression), "literal.yaml")
			require.NoError(t, err)
			result, err := ProcessCustomYamlTags(config, input, "", nil, nil)
			require.NoError(t, err)
			require.Equal(t, expression, result["value"])
		})
	}
}

func TestExplicitYAMLTagsLegacyFixtureAndCacheIsolation(t *testing.T) {
	fixture, err := filepath.Abs("../../tests/fixtures/scenarios/yaml-functions-legacy-strings")
	require.NoError(t, err)
	t.Chdir(fixture)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", fixture)
	t.Setenv("ATMOS_EXPLICIT_TAG_TEST", "resolved")
	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	// Reuse caches across editions, including a return to the compatibility pin.
	for _, pin := range []string{"2026-10-19", "2026-10-20", "", "2026-10-19"} {
		config.Edition = pin
		stacks, err := ExecuteDescribeStacks(&config, "", nil, nil, nil, false, true, true, false, nil, nil)
		require.NoError(t, err)
		component := stacks["dev"].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)["mock"].(map[string]any)
		vars := component["vars"].(map[string]any)
		require.Equal(t, "resolved", vars["explicit"])
		if pin == "2026-10-19" {
			require.Equal(t, "resolved", vars["legacy"])
			require.NotContains(t, vars, "legacy_unset")
		} else {
			require.Equal(t, "!env ATMOS_EXPLICIT_TAG_TEST", vars["legacy"])
			require.Equal(t, "!unset", vars["legacy_unset"])
		}
	}
}
