package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestValidateRejectsIncompletePackagesBeforeAnyNetworkAccess(t *testing.T) {
	disabled := false
	cases := []struct {
		name    string
		pkg     schema.PluginPackage
		message string
	}{
		{"empty source", schema.PluginPackage{Version: "^1"}, "plugins.packages.corporate.source"},
		{"empty version", schema.PluginPackage{Source: "oci://r/a"}, "plugins.packages.corporate.version"},
		{"negative timeout", schema.PluginPackage{Source: "oci://r/a", Version: "^1", Timeout: "-1s"}, "plugins.packages.corporate.timeout"},
		{"zero timeout", schema.PluginPackage{Source: "oci://r/a", Version: "^1", Timeout: "0s"}, "plugins.packages.corporate.timeout"},
		{"unparseable timeout", schema.PluginPackage{Source: "oci://r/a", Version: "^1", Timeout: "soon"}, "plugins.packages.corporate.timeout"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := Validate(&schema.PluginsConfig{Packages: map[string]schema.PluginPackage{"corporate": testCase.pkg}})
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			assert.Contains(t, err.Error(), testCase.message)
			assert.Contains(t, err.Error(), "corporate", "the error names the package")
		})
	}
	t.Run("disabled packages may be incomplete", func(t *testing.T) {
		require.NoError(t, Validate(&schema.PluginsConfig{Packages: map[string]schema.PluginPackage{"corporate": {Enabled: &disabled}}}))
	})
}

func TestPackageTimeoutRoundTripsAndParses(t *testing.T) {
	var parsed schema.PluginsConfig
	require.NoError(t, yaml.Unmarshal([]byte("packages:\n  one:\n    source: oci://r/a\n    version: ^1\n    timeout: 5m\n"), &parsed))
	require.NoError(t, Validate(&parsed))
	timeout, err := PackageTimeout(parsed.Packages["one"])
	require.NoError(t, err)
	assert.Equal(t, "5m0s", timeout.String())
	none, err := PackageTimeout(schema.PluginPackage{})
	require.NoError(t, err)
	assert.Zero(t, none, "an empty timeout means no deadline")
	raw, err := yaml.Marshal(parsed)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "timeout: 5m")
}

func TestAtmosConfigSchemaDescribesPackageTimeout(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "datafetcher", "schema", "atmos", "config", "1.0.json"))
	require.NoError(t, err)
	var document struct {
		Defs map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(raw, &document))
	timeout, found := document.Defs["PluginPackage"].Properties["timeout"]
	require.True(t, found, "plugins.packages.<alias>.timeout must be in the atmos.yaml schema")
	assert.Contains(t, string(timeout), `"string"`)
}
