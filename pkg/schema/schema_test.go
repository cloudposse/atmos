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

func TestIsPagerEnabled(t *testing.T) {
	tests := []struct {
		name   string
		pager  string
		expect bool
	}{
		{"Empty string should enable pager", "", true},
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
		{"Random string should disable pager", "random", false},
		{"Capitalized 'ON' should disable pager (case sensitive)", "ON", false},
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
