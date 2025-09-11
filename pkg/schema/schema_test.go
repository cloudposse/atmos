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

// --- Additional comprehensive tests (using testing + testify/assert) ---

// Covers full matrix of accepted/declined pager values.
func TestTerminal_IsPagerEnabled_Matrix(t *testing.T) {
	tests := []struct {
		name   string
		pager  string
		expect bool
	}{
		{"empty string treated as enabled", "", true},
		{"explicit on", "on", true},
		{"less", "less", true},
		{"true", "true", true},
		{"yes", "yes", true},
		{"y", "y", true},
		{"1", "1", true},
		{"OFF uppercase", "OFF", false},
		{"no", "no", false},
		{"0", "0", false},
		{"random", "random", false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			term := &Terminal{Pager: tt.pager}
			assert.Equal(t, tt.expect, term.IsPagerEnabled(), "Pager=%q", tt.pager)
		})
	}
}

// Exercises custom YAML unmarshalling paths (string, ResourcePath, SchemaRegistry, raw yaml.Node) and getters.
func TestAtmosConfiguration_UnmarshalYAML_Comprehensive(t *testing.T) {
	yamlDoc := `
base_path: /root
schemas:
  cue:
    base_path: /schemas/cue
  opa:
    base_path: /schemas/opa
  jsonschema:
    base_path: /schemas/json
  manifests:
    manifest: "k8s"
    schema: "./schemas/k8s.cue"
    matches: ["**/*.yaml", "**/*.yml"]
  simple: "hello"
  rawnode:
    - not: "a mapping"
`
	var cfg AtmosConfiguration
	err := yaml.Unmarshal([]byte(yamlDoc), &cfg)
	assert.NoError(t, err)
	assert.Equal(t, "/root", cfg.BasePath)

	// ResourcePath keys
	assert.Equal(t, ResourcePath{BasePath: "/schemas/cue"}, cfg.GetResourcePath("cue"))
	assert.Equal(t, ResourcePath{BasePath: "/schemas/opa"}, cfg.GetResourcePath("opa"))
	assert.Equal(t, ResourcePath{BasePath: "/schemas/json"}, cfg.GetResourcePath("jsonschema"))

	// SchemaRegistry key
	man := cfg.GetSchemaRegistry("manifests")
	assert.Equal(t, "k8s", man.Manifest)
	assert.Equal(t, "./schemas/k8s.cue", man.Schema)
	assert.ElementsMatch(t, []string{"**/*.yaml", "**/*.yml"}, man.Matches)

	// String remains string; getters return zero values
	assert.Equal(t, "hello", cfg.Schemas["simple"])
	assert.Equal(t, (ResourcePath{}), cfg.GetResourcePath("simple"))
	assert.Equal(t, (SchemaRegistry{}), cfg.GetSchemaRegistry("simple"))

	// Fallback retains raw yaml.Node
	_, isNode := cfg.Schemas["rawnode"].(yaml.Node)
	assert.True(t, isNode, "rawnode should remain a yaml.Node fallback")

	// Missing keys -> zero values
	assert.Equal(t, (ResourcePath{}), cfg.GetResourcePath("missing"))
	assert.Equal(t, (SchemaRegistry{}), cfg.GetSchemaRegistry("missing"))
}

// Validates that ProcessSchemas converts generic maps to concrete types for both ResourcePath and SchemaRegistry.
func TestAtmosConfiguration_ProcessSchemas_Transforms(t *testing.T) {
	cfg := &AtmosConfiguration{
		Schemas: map[string]interface{}{
			"cue": map[string]interface{}{"base_path": "/x/cue"},
			"manifests": map[string]interface{}{
				"manifest": "helm",
				"schema":   "./schemas/helm.json",
				"matches":  []string{"charts/**"},
			},
			"simple": "value",
		},
	}

	cfg.ProcessSchemas()

	// Concrete types should be stored after processing
	if v, ok := cfg.Schemas["cue"].(ResourcePath); assert.True(t, ok, "cue should be ResourcePath") {
		assert.Equal(t, "/x/cue", v.BasePath)
	}
	if v, ok := cfg.Schemas["manifests"].(SchemaRegistry); assert.True(t, ok, "manifests should be SchemaRegistry") {
		assert.Equal(t, "helm", v.Manifest)
		assert.Equal(t, "./schemas/helm.json", v.Schema)
		assert.Equal(t, []string{"charts/**"}, v.Matches)
	}

	// Getters continue to return zero for unrelated entries
	assert.Equal(t, (ResourcePath{}), cfg.GetResourcePath("simple"))
	assert.Equal(t, (SchemaRegistry{}), cfg.GetSchemaRegistry("simple"))
}

// Ensures marshal errors in ProcessSchemas do not panic and leave values unchanged.
func TestAtmosConfiguration_ProcessSchemas_ErrorValuePreserved(t *testing.T) {
	ch := make(chan int) // json.Marshal cannot handle channels
	cfg := &AtmosConfiguration{
		Schemas: map[string]interface{}{
			"cue": map[string]interface{}{"base_path": "/ok"},
			"bad": ch,
		},
	}

	cfg.ProcessSchemas()

	assert.Same(t, ch, cfg.Schemas["bad"], "'bad' key should remain unchanged on marshal error")
	assert.Equal(t, "/ok", cfg.GetResourcePath("cue").BasePath)
}

// Getters should return zero-values when the stored type does not match expected.
func TestAtmosConfiguration_Getters_TypeMismatch_ReturnZeroValues(t *testing.T) {
	cfg := &AtmosConfiguration{
		Schemas: map[string]interface{}{
			"cue":        "should-be-ResourcePath-but-is-string",
			"manifests":  map[string]interface{}{"unexpected": "shape"},
			"jsonschema": ResourcePath{BasePath: "/ok"},
		},
	}

	assert.Equal(t, (ResourcePath{}), cfg.GetResourcePath("cue"))
	assert.Equal(t, "/ok", cfg.GetResourcePath("jsonschema").BasePath)
	assert.Equal(t, (SchemaRegistry{}), cfg.GetSchemaRegistry("manifests"))
}

// When `schemas` field is not present in YAML, UnmarshalYAML should still initialize Schemas map.
func TestAtmosConfiguration_UnmarshalYAML_NoSchemas_InitializesMap(t *testing.T) {
	yml := `base_path: /x`
	var cfg AtmosConfiguration
	err := yaml.Unmarshal([]byte(yml), &cfg)
	assert.NoError(t, err)
	if assert.NotNil(t, cfg.Schemas) {
		assert.Equal(t, 0, len(cfg.Schemas))
	}
}

// Explicit check of fallback to raw yaml.Node for values that cannot be decoded
// as string, ResourcePath (non cue/opa/jsonschema), or SchemaRegistry.
func TestAtmosConfiguration_UnmarshalYAML_FallbackNode_RawYamlNode(t *testing.T) {
	yml := `
schemas:
  weird:
    - 1
    - 2
    - three
`
	var cfg AtmosConfiguration
	err := yaml.Unmarshal([]byte(yml), &cfg)
	assert.NoError(t, err)

	val, ok := cfg.Schemas["weird"]
	assert.True(t, ok, "expected 'weird' key present")
	_, isNode := val.(yaml.Node)
	assert.True(t, isNode, "expected yaml.Node fallback for 'weird'")
}
