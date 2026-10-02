package cloudformation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestPrepareSourceComponentConfig checks source management can normalize isolation
// without a template path while preserving inherited maps and disabled behavior.
func TestPrepareSourceComponentConfig(t *testing.T) {
	for _, test := range []struct {
		name     string
		enabled  bool
		override string
		disable  bool
		wantErr  bool
	}{
		{name: "implicit workdir", enabled: true},
		{name: "generation disabled"},
		{name: "metadata override", enabled: true, override: "metadata", wantErr: true},
		{name: "settings override", enabled: true, override: "settings", wantErr: true},
		{name: "explicit workdir disable", enabled: true, disable: true, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			config.Components.CloudFormation.AutoGenerateFiles = test.enabled
			working := map[string]any{"sentinel": "unchanged"}
			if test.disable {
				working["enabled"] = false
			}
			section := map[string]any{"generate": map[string]any{"template.yaml": "Resources: {}"}, "provision": map[string]any{"workdir": working}}
			if test.override != "" {
				section[test.override] = map[string]any{"working_directory": "shared"}
			}
			result, err := PrepareSourceComponentConfig(config, section)
			if test.wantErr {
				require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
				return
			}
			require.NoError(t, err)
			assert.NotContains(t, working, "enabled", "normalization must not mutate inherited workdir map")
			if test.enabled {
				assert.Equal(t, true, result["provision"].(map[string]any)["workdir"].(map[string]any)["enabled"])
				info := &schema.ConfigAndStacksInfo{ComponentSection: section}
				require.ErrorIs(t, prepareGeneration(config, info), errUtils.ErrMissingAwsCloudFormationTemplate)
			} else {
				assert.Equal(t, section, result)
			}
		})
	}
}

// TestGenerationRejectsSharedDestinationBeforeProvision prevents a rejected
// configuration from downloading or copying source into its working_directory.
func TestGenerationRejectsSharedDestinationBeforeProvision(t *testing.T) {
	for _, key := range []string{"metadata", "settings"} {
		t.Run(key, func(t *testing.T) {
			root := t.TempDir()
			config := &schema.AtmosConfiguration{BasePath: root}
			config.Components.CloudFormation.AutoGenerateFiles = true
			info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{
				"path":     "template.yaml",
				"generate": map[string]any{"template.yaml": "Resources: {}"},
				"source":   map[string]any{"uri": "https://example.invalid/template.yaml"},
				key:        map[string]any{"working_directory": filepath.Join(root, "shared")},
			}}
			_, err := prepareComponentFiles(t.Context(), config, info)
			require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
			files, err := os.ReadDir(root)
			require.NoError(t, err)
			assert.Empty(t, files)
		})
	}
}
