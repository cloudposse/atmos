package stack

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestMultiFormatStackProcessor tests processing of YAML, JSON, and HCL stack files.
func TestMultiFormatStackProcessor(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/multi-format-stacks/stacks"
	terraformComponentsBasePath := "../../tests/fixtures/multi-format-stacks/components/terraform"
	helmfileComponentsBasePath := ""
	packerComponentsBasePath := ""

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
			},
		},
	}

	t.Run("process YAML stack file", func(t *testing.T) {
		filePaths := []string{
			"../../tests/fixtures/multi-format-stacks/stacks/deploy/dev.yaml",
		}

		listResult, mapResult, _, err := ProcessYAMLConfigFiles(
			&atmosConfig,
			stacksBasePath,
			terraformComponentsBasePath,
			helmfileComponentsBasePath,
			packerComponentsBasePath,
			filePaths,
			false,
			false,
			false,
		)

		require.NoError(t, err)
		assert.Equal(t, 1, len(listResult))
		assert.Equal(t, 1, len(mapResult))
	})

	t.Run("process JSON stack file", func(t *testing.T) {
		filePaths := []string{
			"../../tests/fixtures/multi-format-stacks/stacks/deploy/staging.json",
		}

		listResult, mapResult, _, err := ProcessYAMLConfigFiles(
			&atmosConfig,
			stacksBasePath,
			terraformComponentsBasePath,
			helmfileComponentsBasePath,
			packerComponentsBasePath,
			filePaths,
			false,
			false,
			false,
		)

		require.NoError(t, err)
		assert.Equal(t, 1, len(listResult))
		assert.Equal(t, 1, len(mapResult))
	})

	t.Run("process HCL stack file", func(t *testing.T) {
		filePaths := []string{
			"../../tests/fixtures/multi-format-stacks/stacks/deploy/prod.hcl",
		}

		listResult, mapResult, _, err := ProcessYAMLConfigFiles(
			&atmosConfig,
			stacksBasePath,
			terraformComponentsBasePath,
			helmfileComponentsBasePath,
			packerComponentsBasePath,
			filePaths,
			false,
			false,
			false,
		)

		require.NoError(t, err)
		assert.Equal(t, 1, len(listResult))
		assert.Equal(t, 1, len(mapResult))
	})

	t.Run("process all formats together", func(t *testing.T) {
		filePaths := []string{
			"../../tests/fixtures/multi-format-stacks/stacks/deploy/dev.yaml",
			"../../tests/fixtures/multi-format-stacks/stacks/deploy/staging.json",
			"../../tests/fixtures/multi-format-stacks/stacks/deploy/prod.hcl",
		}

		listResult, mapResult, _, err := ProcessYAMLConfigFiles(
			&atmosConfig,
			stacksBasePath,
			terraformComponentsBasePath,
			helmfileComponentsBasePath,
			packerComponentsBasePath,
			filePaths,
			false,
			false,
			false,
		)

		require.NoError(t, err)
		assert.Equal(t, 3, len(listResult))
		assert.Equal(t, 3, len(mapResult))
	})
}
