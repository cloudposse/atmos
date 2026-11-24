package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard: a rename of the configuration field breaks the build, not only these tests.
var _ = schema.PluginPackage{InteractionTimeout: "30m"}

func TestPackageInteractionTimeoutRoundTripsAndParses(t *testing.T) {
	var parsed schema.PluginsConfig
	source := "packages:\n  one:\n    source: oci://r/a\n    version: ^1\n    timeout: 5m\n    interaction_timeout: 45m\n"
	require.NoError(t, yaml.Unmarshal([]byte(source), &parsed))
	require.NoError(t, Validate(&parsed))
	limit, err := PackageInteractionTimeout(parsed.Packages["one"])
	require.NoError(t, err)
	assert.Equal(t, 45*time.Minute, limit)
	raw, err := yaml.Marshal(parsed)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "interaction_timeout: 45m")

	// Negative: an unset key keeps the runtime default, which the zero value selects.
	none, err := PackageInteractionTimeout(schema.PluginPackage{Timeout: "5m"})
	require.NoError(t, err)
	assert.Zero(t, none, "an empty interaction_timeout means the default limit")
}

func TestPackageInteractionTimeoutAcceptsTheMaximum(t *testing.T) {
	limit, err := PackageInteractionTimeout(schema.PluginPackage{InteractionTimeout: MaxInteractionTimeout.String()})
	require.NoError(t, err)
	assert.Equal(t, MaxInteractionTimeout, limit)
}

func TestValidateRejectsInvalidInteractionTimeout(t *testing.T) {
	cases := []struct {
		name  string
		value string
		hint  string
	}{
		{"negative", "-1s", "positive Go duration"},
		{"zero", "0s", "positive Go duration"},
		{"unparseable", "soon", "positive Go duration"},
		{"above the maximum", (MaxInteractionTimeout + time.Second).String(), "at most 24h"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			entry := schema.PluginPackage{Source: "oci://r/a", Version: "^1", Timeout: "5m", InteractionTimeout: testCase.value}
			err := Validate(&schema.PluginsConfig{Packages: map[string]schema.PluginPackage{"corporate": entry}})
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			assert.Contains(t, err.Error(), "plugins.packages.corporate.interaction_timeout", "the error names the package and key")
			hints := errUtils.AllHints(err)
			require.NotEmpty(t, hints)
			assert.Contains(t, hints[0], "plugins.packages.corporate.interaction_timeout")
			assert.Contains(t, hints[0], testCase.hint)

			_, parseErr := PackageInteractionTimeout(entry)
			require.ErrorIs(t, parseErr, errUtils.ErrPluginInvalidConfig)
		})
	}
}

func TestValidateRequiresTimeoutBesideInteractionTimeout(t *testing.T) {
	entry := schema.PluginPackage{Source: "oci://r/a", Version: "^1", InteractionTimeout: "30m"}
	err := Validate(&schema.PluginsConfig{Packages: map[string]schema.PluginPackage{"corporate": entry}})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	assert.Contains(t, err.Error(), "plugins.packages.corporate.interaction_timeout")
	assert.Contains(t, err.Error(), "plugins.packages.corporate.timeout")

	// Negative: the same value is accepted once the package has a timeout.
	entry.Timeout = "5m"
	require.NoError(t, Validate(&schema.PluginsConfig{Packages: map[string]schema.PluginPackage{"corporate": entry}}))
}

func TestValidateIgnoresInteractionTimeoutOfDisabledPackage(t *testing.T) {
	disabled := false
	entry := schema.PluginPackage{Enabled: &disabled, InteractionTimeout: "soon"}
	require.NoError(t, Validate(&schema.PluginsConfig{Packages: map[string]schema.PluginPackage{"corporate": entry}}))
}

func TestAtmosConfigSchemaDescribesPackageInteractionTimeout(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "datafetcher", "schema", "atmos", "config", "1.0.json"))
	require.NoError(t, err)
	var document struct {
		Defs map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(raw, &document))
	property, found := document.Defs["PluginPackage"].Properties["interaction_timeout"]
	require.True(t, found, "plugins.packages.<alias>.interaction_timeout must be in the atmos.yaml schema")
	assert.Contains(t, string(property), `"string"`)
	assert.Contains(t, string(property), "15m", "the description states the default")
}
