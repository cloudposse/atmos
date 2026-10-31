package component

import (
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

const settingsSchema = `{
  "type": "object",
  "properties": {
    "enabled": {"type": "boolean"},
    "mode": {"const": "native"},
    "discovery": {"type": "object", "properties": {"enabled": {"type": "boolean"}}, "additionalProperties": false}
  },
  "required": ["enabled"],
  "additionalProperties": false
}`

func providerWithSchema(t *testing.T) *Provider {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("settings.json", strings.NewReader(settingsSchema)))
	compiled, err := compiler.Compile("settings.json")
	require.NoError(t, err)
	return &Provider{name: "terragrunt", schema: compiled}
}

func TestSettingsViolationsNameKeysAndHideValues(t *testing.T) {
	provider := providerWithSchema(t)
	require.NoError(t, provider.ValidateSettings(map[string]any{"enabled": true, "mode": "native"}))

	for name, test := range map[string]struct {
		settings map[string]any
		want     string
	}{
		"misspelled key": {map[string]any{"enabled": true, "discoverey": map[string]any{}}, "components.terraform.terragrunt.discoverey"},
		"nested unknown": {map[string]any{"enabled": true, "discovery": map[string]any{"included": "x"}}, "components.terraform.terragrunt.discovery.included"},
		"missing":        {map[string]any{}, "components.terraform.terragrunt.enabled"},
		"wrong type":     {map[string]any{"enabled": "hunter2"}, "`components.terraform.terragrunt.enabled` has the wrong type"},
		"bad const":      {map[string]any{"enabled": true, "mode": "hunter2"}, "`components.terraform.terragrunt.mode` has an unsupported value"},
	} {
		t.Run(name, func(t *testing.T) {
			err := provider.ValidateSettings(test.settings)
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			text := strings.Join(errUtils.AllDetails(err), "\n")
			assert.Contains(t, text, test.want)
			assert.NotContains(t, text, "hunter2", "schema messages may echo values and must stay out")
			assert.NotEmpty(t, errUtils.AllHints(err))
		})
	}
}

func TestNonSchemaSettingsErrorStillNamesTheSection(t *testing.T) {
	err := providerWithSchema(t).settingsError(assert.AnError)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	assert.Contains(t, strings.Join(errUtils.AllDetails(err), "\n"), "components.terraform.terragrunt")
}
