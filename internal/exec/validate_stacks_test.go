package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestValidateStacks(t *testing.T) {
	basePath := "../../tests/fixtures/scenarios/atmos-overrides-section/stacks"

	basePathAbs, err := filepath.Abs(basePath)
	assert.Nil(t, err)

	stackConfigFilesAbsolutePaths := []string{
		filepath.Join(basePathAbs, "deploy", "dev.yaml"),
		filepath.Join(basePathAbs, "deploy", "prod.yaml"),
		filepath.Join(basePathAbs, "deploy", "sandbox.yaml"),
		filepath.Join(basePathAbs, "deploy", "staging.yaml"),
		filepath.Join(basePathAbs, "deploy", "test.yaml"),
	}

	atmosConfig := schema.AtmosConfiguration{
		BasePath:                      basePath,
		StacksBaseAbsolutePath:        basePathAbs,
		StackConfigFilesAbsolutePaths: stackConfigFilesAbsolutePaths,
		Stacks: schema.Stacks{
			NamePattern: "{stage}",
		},
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

	err = ValidateStacks(atmosConfig)
	assert.Nil(t, err)
}
