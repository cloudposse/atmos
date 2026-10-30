package manager

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSetInheritedEntryCreatesTrackOverride(t *testing.T) {
	file := crudSandbox(t)
	source := `version:
  track: dev
  dependencies:
    widget:
      ecosystem: github
      package: example/widget
      desired: "1.0.0"
  tracks:
    dev: {}
    prod: {}
`
	require.NoError(t, os.WriteFile(file, []byte(source), 0o600))
	var initial struct {
		Version schema.Version `yaml:"version"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(source), &initial))
	config := &schema.AtmosConfiguration{Version: initial.Version}
	_, err := SetEntryFields(config, "dev", "widget", map[string]any{"desired": "2.0.0"})
	require.NoError(t, err)
	changed, err := os.ReadFile(file)
	require.NoError(t, err)
	var after struct {
		Version schema.Version `yaml:"version"`
	}
	require.NoError(t, yaml.Unmarshal(changed, &after))
	updated := &schema.AtmosConfiguration{Version: after.Version}
	dev, err := EffectiveEntries(updated, "dev")
	require.NoError(t, err)
	prod, err := EffectiveEntries(updated, "prod")
	require.NoError(t, err)
	require.Equal(t, "2.0.0", dev["widget"].Desired)
	require.Equal(t, "1.0.0", prod["widget"].Desired)
	require.Equal(t, "1.0.0", updated.Version.Dependencies["widget"].Desired)
	require.Empty(t, updated.Version.Tracks["dev"].Dependencies["widget"].Package, "only requested fields should be pinned")
	_, err = SetEntryFields(updated, "dev", "missing", map[string]any{"desired": "2.0.0"})
	require.ErrorIs(t, err, ErrEntryNotFound)
}
