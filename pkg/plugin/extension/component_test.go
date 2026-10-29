package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestComponentMetadataRequiresLocalSchemaAndFamily(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "config.json"), []byte(`{"type":"object"}`), 0o600))
	for name, metadata := range map[string]map[string]any{
		"missing type":      {"schema": "config.json"},
		"non-string type":   {"component_type": true, "schema": "config.json"},
		"empty type":        {"component_type": "", "schema": "config.json"},
		"padded type":       {"component_type": " terraform ", "schema": "config.json"},
		"missing schema":    {"component_type": "terraform"},
		"non-string schema": {"component_type": "terraform", "schema": true},
		"remote schema":     {"component_type": "terraform", "schema": "https://example.invalid/config.json"},
		"undeclared option": {"component_type": "terraform", "schema": "config.json", "execute": true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := ValidateComponent(Binding{Directory: directory, Metadata: metadata})
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
		})
	}
	require.NoError(t, ValidateComponent(Binding{Directory: directory, Metadata: map[string]any{"component_type": "terraform", "schema": "config.json"}}))
}

func TestComponentServicesAreIndependentOfExecution(t *testing.T) {
	t.Parallel()
	for _, capability := range []string{ComponentDiscovery, ComponentValidation, ComponentPreparation, ComponentState} {
		require.True(t, IsComponent(capability))
	}
	for _, capability := range []string{"", "command", "auth.provider", "component.execution"} {
		require.False(t, IsComponent(capability))
	}
}
