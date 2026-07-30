package configschema

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Generation walks and parses all Go source under pkg/ to extract doc comments,
// so tests share one generated document instead of paying that cost per test.
var (
	generateOnce   sync.Once
	generatedBytes []byte
	generateErr    error
)

// generatedSchema returns the freshly generated schema document, generating it
// at most once per test binary.
func generatedSchema(t *testing.T) []byte {
	t.Helper()
	generateOnce.Do(func() {
		generatedBytes, generateErr = Generate()
	})
	require.NoError(t, generateErr)
	return generatedBytes
}

// generatedSchemaDocument returns the generated schema parsed into a generic map.
func generatedSchemaDocument(t *testing.T) map[string]any {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(generatedSchema(t), &doc))
	return doc
}

// rootProperties returns the top-level `properties` of the generated schema.
func rootProperties(t *testing.T) map[string]any {
	t.Helper()
	props, ok := generatedSchemaDocument(t)["properties"].(map[string]any)
	require.True(t, ok, "generated schema must have a root `properties` object")
	return props
}

// definitions returns the `$defs` of the generated schema.
func definitions(t *testing.T) map[string]any {
	t.Helper()
	defs, ok := generatedSchemaDocument(t)["$defs"].(map[string]any)
	require.True(t, ok, "generated schema must have a `$defs` object")
	return defs
}

func TestGenerateProducesRootMetadata(t *testing.T) {
	doc := generatedSchemaDocument(t)

	assert.Equal(t, SchemaID, doc["$id"])
	assert.Equal(t, "https://json-schema.org/draft/2020-12/schema", doc["$schema"])
	assert.Equal(t, schemaTitle, doc["title"])
	assert.Equal(t, "object", doc["type"])
}

func TestGenerateModelsAuthoredSections(t *testing.T) {
	props := rootProperties(t)

	// Representative authored sections spanning the first and last fields of
	// AtmosConfiguration, so dropped or corrupted contents fail loudly.
	for _, section := range []string{
		"base_path", "components", "stacks", "logs", "commands", "aliases",
		"integrations", "schemas", "templates", "settings", "vendor",
		"auth", "toolchain", "git", "profiles", "import", "env", "docs",
		"ai", "aws", "mcp", "lsp",
	} {
		assert.Contains(t, props, section, "authored atmos.yaml section %q must be modeled", section)
	}
}

func TestGenerateMarksDeprecatedConfigurationFields(t *testing.T) {
	defs := definitions(t)
	for _, test := range []struct {
		definition  string
		property    string
		replacement string
	}{
		{"Stacks", "name_pattern", "stacks.name_template"},
		{"Helmfile", "helm_aws_profile_pattern", "--identity"},
		{"Helmfile", "cluster_name_pattern", "components.helmfile.cluster_name_template"},
		{"Terminal", "no_color", "settings.terminal.color (invert the value)"},
		{"AtmosSettings", "docs", "docs"},
		{"Docs", "max-width", "settings.terminal.max-width"},
		{"Docs", "pagination", "settings.terminal.pagination"},
		{"Provider", "provider_type", "driver"},
		{"VersionProvider", "type", "kind"},
	} {
		t.Run(test.definition+"/"+test.property, func(t *testing.T) {
			definition := defs[test.definition].(map[string]any)
			properties := definition["properties"].(map[string]any)
			field := properties[test.property].(map[string]any)
			variants := field["anyOf"].([]any)
			metadata := variants[0].(map[string]any)
			assert.Equal(t, true, metadata["deprecated"])
			assert.Equal(t, test.replacement, metadata["x-atmos-replacement"])
		})
	}
}

func TestGenerateExcludesRuntimeFields(t *testing.T) {
	props := rootProperties(t)

	for _, field := range excludedRootFields {
		assert.NotContains(t, props, field, "runtime-computed field %q must not appear in the authored-config schema", field)
	}
}

func TestGenerateExtractsDocCommentsAsDescriptions(t *testing.T) {
	defs := definitions(t)

	profilesCfg, ok := defs["ProfilesConfig"].(map[string]any)
	require.True(t, ok, "ProfilesConfig definition must exist")
	assert.Equal(t,
		"ProfilesConfig defines configuration for the profiles system.",
		profilesCfg["description"],
		"Go doc comments must flow into schema descriptions")
}

func TestNormalizeCommentMapPaths(t *testing.T) {
	comments := map[string]string{
		"github.com/cloudposse/atmos/pkg\\schema.ProfilesConfig":          "type comment",
		"github.com/cloudposse/atmos/pkg\\schema.ProfilesConfig.BasePath": "field comment",
	}

	normalizeCommentMapPaths(comments)

	assert.Equal(t, "type comment", comments["github.com/cloudposse/atmos/pkg/schema.ProfilesConfig"])
	assert.Equal(t, "field comment", comments["github.com/cloudposse/atmos/pkg/schema.ProfilesConfig.BasePath"])
	assert.NotContains(t, comments, "github.com/cloudposse/atmos/pkg\\schema.ProfilesConfig")
	assert.NotContains(t, comments, "github.com/cloudposse/atmos/pkg\\schema.ProfilesConfig.BasePath")
}

func TestGenerateModelsSchemasSectionPolymorphism(t *testing.T) {
	defs := definitions(t)
	assert.Contains(t, defs, "SchemaRegistry")
	assert.Contains(t, defs, "ResourcePath")

	raw, err := json.Marshal(rootProperties(t)["schemas"])
	require.NoError(t, err)
	schemas := string(raw)
	assert.Contains(t, schemas, "#/$defs/ResourcePath")
	assert.Contains(t, schemas, "#/$defs/SchemaRegistry")
}

func TestGenerateAllowsYamlFunctions(t *testing.T) {
	defs := definitions(t)

	yamlFunc, ok := defs[yamlFunctionDef].(map[string]any)
	require.True(t, ok, "the %s definition must exist", yamlFunctionDef)
	pattern, ok := yamlFunc["pattern"].(string)
	require.True(t, ok, "the %s definition must constrain values by pattern", yamlFunctionDef)
	assert.Contains(t, pattern, "!include")
	assert.Contains(t, pattern, "!env")

	// Object-typed sections must accept the authored function form, e.g.
	// `logs: !include shared.yaml`.
	raw, err := json.Marshal(rootProperties(t)["logs"])
	require.NoError(t, err)
	assert.Contains(t, string(raw), "#/$defs/"+yamlFunctionDef,
		"object-typed sections must carry a yamlFunction anyOf alternative")
}

func TestGenerateHasNoRequiredFields(t *testing.T) {
	// Partial configs (atmos.d fragments, profile files, imports) must validate
	// on their own, so nothing anywhere in the schema may use the `required`
	// keyword. Properties literally named "required" (e.g. CommandArgument's
	// `required` flag) are fine — the keyword form is an array.
	assertNoRequiredKeyword(t, generatedSchemaDocument(t), "$")
}

func assertNoRequiredKeyword(t *testing.T, node any, path string) {
	t.Helper()
	switch typed := node.(type) {
	case map[string]any:
		for key, value := range typed {
			if key == "required" {
				_, isKeyword := value.([]any)
				assert.False(t, isKeyword,
					"the atmos.yaml schema must not mark fields required (found at %s): partial configs (atmos.d, profiles, imports) must validate standalone", path)
			}
			assertNoRequiredKeyword(t, value, path+"."+key)
		}
	case []any:
		for i, value := range typed {
			assertNoRequiredKeyword(t, value, fmt.Sprintf("%s[%d]", path, i))
		}
	}
}
