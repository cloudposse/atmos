package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestAtmosConfigurationWorksWithOpa(t *testing.T) {
	yamlString := `
schemas:
  opa:
    base_path: "some/random/path"
`
	atmosConfig := &AtmosConfiguration{}
	err := yaml.Unmarshal([]byte(yamlString), atmosConfig)
	assert.NoError(t, err)
	resourcePath := atmosConfig.GetResourcePath("opa")
	assert.Equal(t, "some/random/path", resourcePath.BasePath)
}

func TestAtmosConfigurationWithSchemas(t *testing.T) {
	yamlString := `
schemas:
  atmos:
    manifest: "some/random/path"
    matches:
      - hello
      - world
`
	atmosConfig := &AtmosConfiguration{}
	err := yaml.Unmarshal([]byte(yamlString), atmosConfig)
	assert.NoError(t, err)
	schemas := atmosConfig.GetSchemaRegistry("atmos")
	assert.Equal(t, "some/random/path", schemas.Manifest)
	assert.Equal(t, []string{"hello", "world"}, schemas.Matches)
}

func TestAtmosConfigurationWithTabWidthAndDescribeSettings(t *testing.T) {
	// Test direct struct creation for TabWidth
	tabWidth := 4
	terminal := Terminal{TabWidth: tabWidth}
	assert.Equal(t, tabWidth, terminal.TabWidth)

	// Test direct struct creation for IncludeEmpty (false)
	falseValue := false
	describeSettings := DescribeSettings{IncludeEmpty: &falseValue}
	assert.NotNil(t, describeSettings.IncludeEmpty)
	assert.False(t, *describeSettings.IncludeEmpty)

	// Test direct struct creation for IncludeEmpty (true)
	trueValue := true
	describeSettings = DescribeSettings{IncludeEmpty: &trueValue}
	assert.NotNil(t, describeSettings.IncludeEmpty)
	assert.True(t, *describeSettings.IncludeEmpty)

	// Test complete struct creation with all fields
	atmosConfig := AtmosConfiguration{
		Settings: AtmosSettings{
			Terminal: Terminal{TabWidth: tabWidth},
		},
		Describe: Describe{
			Settings: DescribeSettings{IncludeEmpty: &trueValue},
		},
	}

	// Verify fields are set correctly
	assert.Equal(t, tabWidth, atmosConfig.Settings.Terminal.TabWidth)
	assert.NotNil(t, atmosConfig.Describe.Settings.IncludeEmpty)
	assert.True(t, *atmosConfig.Describe.Settings.IncludeEmpty)
}
