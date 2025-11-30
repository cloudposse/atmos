package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v3"
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

func TestIsColorEnabled(t *testing.T) {
	tests := []struct {
		name    string
		color   bool
		noColor bool
		isTTY   bool
		expect  bool
	}{
		{"Color=true, NoColor=false should force enable", true, false, false, true},
		{"Color=true, NoColor=false should force enable (even non-TTY)", true, false, false, true},
		{"Color=false, NoColor=false, isTTY=true should use TTY default", false, false, true, true},
		{"Color=false, NoColor=false, isTTY=false should use TTY default", false, false, false, false},
		{"NoColor=true should force disable (NoColor takes precedence)", true, true, true, false},
		{"NoColor=true should force disable even if Color=true", true, true, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := &Terminal{Color: tt.color, NoColor: tt.noColor}
			result := term.IsColorEnabled(tt.isTTY)
			if result != tt.expect {
				t.Errorf("IsColorEnabled(%v) for Color=%v, NoColor=%v: expected %v, got %v", tt.isTTY, tt.color, tt.noColor, tt.expect, result)
			}
		})
	}
}

func TestIsPagerEnabled(t *testing.T) {
	tests := []struct {
		name   string
		pager  string
		expect bool
	}{
		{"Empty string should disable pager (new default)", "", false},
		{"'on' should enable pager", "on", true},
		{"'less' should enable pager", "less", true},
		{"'true' should enable pager", "true", true},
		{"'yes' should enable pager", "yes", true},
		{"'y' should enable pager", "y", true},
		{"'1' should enable pager", "1", true},
		{"'off' should disable pager", "off", false},
		{"'false' should disable pager", "false", false},
		{"'no' should disable pager", "no", false},
		{"'n' should disable pager", "n", false},
		{"'0' should disable pager", "0", false},
		{"'more' should enable pager (pager command)", "more", true},
		{"'cat' should enable pager (any command)", "cat", true},
		{"Capitalized 'ON' should enable pager", "ON", true},
		{"Capitalized 'TRUE' should enable pager", "TRUE", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := &Terminal{Pager: tt.pager}
			result := term.IsPagerEnabled()
			if result != tt.expect {
				t.Errorf("IsPagerEnabled() for Pager=%q: expected %v, got %v", tt.pager, tt.expect, result)
			}
		})
	}
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
