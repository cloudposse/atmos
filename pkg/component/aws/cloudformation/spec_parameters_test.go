package cloudformation

import (
	"os"
	"path/filepath"
	"testing"

	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// paramPairs flattens parameters into key -> value for order-independent
// assertions; a nil ParameterValue (UsePreviousValue) maps to "<previous>".
func paramPairs(params []cfntypes.Parameter) map[string]string {
	pairs := make(map[string]string, len(params))
	for _, p := range params {
		if p.ParameterValue == nil {
			pairs[*p.ParameterKey] = "<previous>"
			continue
		}
		pairs[*p.ParameterKey] = *p.ParameterValue
	}
	return pairs
}

// The AWS CLI / Rain list form must produce the same API parameters as the
// native map form. Before the fix every non-map value was silently dropped and
// apply deployed the template defaults with exit code 0.
func TestNormalizeParameters_ListForm(t *testing.T) {
	params, err := normalizeParameters([]any{
		map[string]any{"ParameterKey": "Env", "ParameterValue": "dev"},
		map[string]any{"ParameterKey": "Count", "ParameterValue": 3},
		map[string]any{"ParameterKey": "AZs", "ParameterValue": []any{"a", "b"}},
		map[string]any{"ParameterKey": "Empty"},
	})
	require.NoError(t, err)
	require.Len(t, params, 4)

	// Declared order is preserved, and the first and last entries are exact.
	assert.Equal(t, "Env", *params[0].ParameterKey)
	assert.Equal(t, "dev", *params[0].ParameterValue)
	assert.Equal(t, "Empty", *params[3].ParameterKey)
	assert.Equal(t, "", *params[3].ParameterValue)
	assert.Equal(t, map[string]string{"Env": "dev", "Count": "3", "AZs": "a,b", "Empty": ""}, paramPairs(params))
}

func TestNormalizeParameters_ListFormUsePreviousValue(t *testing.T) {
	params, err := normalizeParameters([]any{
		map[string]any{"ParameterKey": "Keep", "UsePreviousValue": true},
		map[string]any{"ParameterKey": "Set", "ParameterValue": "x", "UsePreviousValue": false},
	})
	require.NoError(t, err)
	require.Len(t, params, 2)

	require.NotNil(t, params[0].UsePreviousValue)
	assert.True(t, *params[0].UsePreviousValue)
	assert.Nil(t, params[0].ParameterValue)
	assert.Equal(t, "x", *params[1].ParameterValue)
}

func TestNormalizeParameters_Invalid(t *testing.T) {
	tests := []struct {
		name        string
		raw         any
		wantContain []string
	}{
		{"string names its type", "not-a-map", []string{"string"}},
		{"number names its type", 42, []string{"int"}},
		{"bool names its type", true, []string{"bool"}},
		{"entry that is not a map names the index", []any{map[string]any{"ParameterKey": "A", "ParameterValue": "1"}, "oops"}, []string{"parameters[1]", "string"}},
		{"missing ParameterKey names the index", []any{map[string]any{"ParameterValue": "1"}}, []string{"parameters[0]", "ParameterKey"}},
		{"empty ParameterKey names the index", []any{map[string]any{"ParameterKey": "", "ParameterValue": "1"}}, []string{"parameters[0]", "ParameterKey"}},
		{"non-string ParameterKey names the index", []any{map[string]any{"ParameterKey": 7}}, []string{"parameters[0]", "ParameterKey"}},
		{"unknown field names the index and field", []any{map[string]any{"ParameterKey": "A", "Value": "1"}}, []string{"parameters[0]", "Value"}},
		{"non-boolean UsePreviousValue", []any{map[string]any{"ParameterKey": "A", "UsePreviousValue": "yes"}}, []string{"parameters[0]", "UsePreviousValue"}},
		{"UsePreviousValue with a value", []any{map[string]any{"ParameterKey": "A", "ParameterValue": "1", "UsePreviousValue": true}}, []string{"parameters[0]", "UsePreviousValue", "ParameterValue"}},
		{"duplicate key names both indexes", []any{
			map[string]any{"ParameterKey": "A", "ParameterValue": "1"},
			map[string]any{"ParameterKey": "A", "ParameterValue": "2"},
		}, []string{"parameters[1]", "parameters[0]", `"A"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := normalizeParameters(tt.raw)
			require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationParameters)
			assert.Nil(t, params)
			for _, want := range tt.wantContain {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

func TestNormalizeParameters_ListEntryValueError(t *testing.T) {
	_, err := normalizeParameters([]any{
		map[string]any{"ParameterKey": "A", "ParameterValue": map[string]any{"nested": "x"}},
	})
	require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
	assert.Contains(t, err.Error(), "parameters[0]")
}

// buildStackSpec must surface the parameter error, so apply, diff and
// --dry-run all fail instead of deploying the template defaults.
func TestBuildStackSpec_PropagatesInvalidParametersType(t *testing.T) {
	_, err := buildStackSpec(map[string]any{
		"stack_name": "vpc",
		"template":   "Resources: {}\n",
		"parameters": "not-a-map",
	})
	require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationParameters)
}

// parametersFromInclude loads a stack manifest whose `parameters:` uses the
// given include expression, through the real !include implementation, and
// returns the component section fed to buildStackSpec.
func parametersFromInclude(t *testing.T, fileName, fileBody, expr string) map[string]any {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, fileName), []byte(fileBody), 0o600))
	manifest := filepath.Join(dir, "stack.yaml")
	input := "stack_name: vpc\ntemplate: \"Resources: {}\\n\"\nparameters: !include " + fileName + expr + "\n"
	require.NoError(t, os.WriteFile(manifest, []byte(input), 0o600))
	t.Chdir(dir)

	section, err := u.UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{BasePath: "."}, input, manifest)
	require.NoError(t, err)
	return section
}

// `!include` of a JSON array (the AWS CLI parameter-file shape) must reach the
// API intact.
func TestBuildStackSpec_IncludeJSONParameterList(t *testing.T) {
	section := parametersFromInclude(t, "params.json", `[
  {"ParameterKey": "Env", "ParameterValue": "dev"},
  {"ParameterKey": "Name", "ParameterValue": "web"}
]`, "")

	spec, err := buildStackSpec(section)
	require.NoError(t, err)
	require.Len(t, spec.Parameters, 2)
	assert.Equal(t, "Env", *spec.Parameters[0].ParameterKey)
	assert.Equal(t, "web", *spec.Parameters[1].ParameterValue)
}

// A Rain config file is `{Parameters: {...}, Tags: {...}}`, not a parameter
// list. The documented way to use it is a YQ expression that selects the map.
func TestBuildStackSpec_IncludeRainConfigParametersViaYQ(t *testing.T) {
	section := parametersFromInclude(t, "rain.json", `{
  "Parameters": {"Env": "dev", "Name": "web"},
  "Tags": {"Team": "platform"}
}`, " .Parameters")

	spec, err := buildStackSpec(section)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"Env": "dev", "Name": "web"}, paramPairs(spec.Parameters))
}

// The YAML form of a Rain config file, as shown in the migration guide, works
// the same way for both `.Parameters` and `.Tags`.
func TestBuildStackSpec_IncludeRainConfigYAMLParametersAndTags(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rain-config.yaml"), []byte("Parameters:\n  CidrBlock: 10.0.0.0/16\n  Environment: dev\nTags:\n  Team: platform\n"), 0o600))
	manifest := filepath.Join(dir, "stack.yaml")
	input := "stack_name: vpc\ntemplate: \"Resources: {}\\n\"\nparameters: !include rain-config.yaml .Parameters\ntags: !include rain-config.yaml .Tags\n"
	require.NoError(t, os.WriteFile(manifest, []byte(input), 0o600))
	t.Chdir(dir)

	section, err := u.UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{BasePath: "."}, input, manifest)
	require.NoError(t, err)

	spec, err := buildStackSpec(section)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"CidrBlock": "10.0.0.0/16", "Environment": "dev"}, paramPairs(spec.Parameters))
	require.Len(t, spec.Tags, 1)
	assert.Equal(t, "Team", *spec.Tags[0].Key)
	assert.Equal(t, "platform", *spec.Tags[0].Value)
}

// Including the whole Rain config as `parameters:` would deploy Parameters and
// Tags as parameter names; they are nested maps, so they must be rejected, not
// silently ignored.
func TestBuildStackSpec_IncludeWholeRainConfigIsRejected(t *testing.T) {
	section := parametersFromInclude(t, "rain.json", `{
  "Parameters": {"Env": "dev"},
  "Tags": {"Team": "platform"}
}`, "")

	_, err := buildStackSpec(section)
	require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
}
