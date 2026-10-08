package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

// These tests pin the tag-preserving !include path: a YAML file pulled in with
// !include keeps its tags, so nested Atmos functions are deferred for the
// later evaluation phase (not decoded to bare strings) and foreign tags reach
// their registered rewriter. Before this, `ZipFile: !include handler.py` inside
// an included CloudFormation template became the literal string "handler.py".

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestIncludeYAML_PreservesNestedAtmosFunctions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "template.yaml", `
Resources:
  Marker:
    Properties:
      Value: !env DEPLOY_OWNER
      Code: !include handler.py
`)
	writeFile(t, dir, "handler.py", "def handler(event, context):\n    return 1\n")
	manifestPath := writeFile(t, dir, "stack.yaml", "x: 1\n")

	t.Run("with | eval", func(t *testing.T) {
		result, err := UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{BasePath: dir}, "template: !include ./template.yaml | eval\n", manifestPath)
		require.NoError(t, err)

		props := result["template"].(map[string]any)["Resources"].(map[string]any)["Marker"].(map[string]any)["Properties"].(map[string]any)
		assert.Equal(t, "!env DEPLOY_OWNER", props["Value"], "nested !env is deferred for the evaluation phase")
		assert.Equal(t, "def handler(event, context):\n    return 1\n", props["Code"], "nested !include of a non-YAML file resolves to its text")
	})

	t.Run("without | eval the documented data contract holds", func(t *testing.T) {
		result, err := UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{BasePath: dir}, "template: !include ./template.yaml\n", manifestPath)
		require.NoError(t, err)

		props := result["template"].(map[string]any)["Resources"].(map[string]any)["Marker"].(map[string]any)["Properties"].(map[string]any)
		assert.Equal(t, "DEPLOY_OWNER", props["Value"], "tag dropped, argument kept as a string")
		assert.Equal(t, "handler.py", props["Code"])
	})
}

func TestIncludeYAML_ForeignTagReachesRewriter(t *testing.T) {
	RegisterForeignTagRewriter("!x-ref", func(node *yaml.Node) error {
		inner := *node
		inner.Tag = "!!str"
		key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "Ref"}
		*node = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{key, &inner}}
		return nil
	})
	dir := t.TempDir()
	writeFile(t, dir, "template.yml", "Outputs:\n  Id:\n    Value: !x-ref Bucket\n")
	manifestPath := writeFile(t, dir, "stack.yaml", "x: 1\n")

	for _, manifest := range []string{"template: !include ./template.yml\n", "template: !include ./template.yml | eval\n"} {
		result, err := UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{BasePath: dir}, manifest, manifestPath)
		require.NoError(t, err)
		value := result["template"].(map[string]any)["Outputs"].(map[string]any)["Id"].(map[string]any)["Value"]
		assert.Equal(t, map[string]any{"Ref": "Bucket"}, value, "foreign tags are rewritten with or without | eval: %s", manifest)
	}
}

func TestIncludeYAML_QueryKeepsTags(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.yaml", "Parameters:\n  Owner: !env OWNER\n  Stage: dev\nTags:\n  Team: platform\n")
	manifestPath := writeFile(t, dir, "stack.yaml", "x: 1\n")

	result, err := UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{BasePath: dir}, "parameters: !include ./config.yaml .Parameters | eval\n", manifestPath)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"Owner": "!env OWNER", "Stage": "dev"}, result["parameters"])

	result, err = UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{BasePath: dir}, "parameters: !include ./config.yaml .Parameters\n", manifestPath)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"Owner": "OWNER", "Stage": "dev"}, result["parameters"], "without | eval the tag is dropped as documented")
}

func TestIncludeYAML_QueryScalarAndEmptyResults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.yaml", "name: \"068007702576\"\nnote: '# not a comment'\n")
	writeFile(t, dir, "empty.yaml", "")
	manifestPath := writeFile(t, dir, "stack.yaml", "x: 1\n")

	manifest := "name: !include ./config.yaml .name\nnote: !include ./config.yaml .note\nnothing: !include ./empty.yaml\n"
	result, err := UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{BasePath: dir}, manifest, manifestPath)
	require.NoError(t, err)
	assert.Equal(t, "068007702576", result["name"], "a quoted numeric-looking string stays a string")
	assert.Equal(t, "# not a comment", result["note"])
	assert.Nil(t, result["nothing"], "an empty YAML file includes as null, as before")
}

func TestIncludeRaw_DoesNotSpliceYAML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "template.yaml", "Value: !env X\n")
	manifestPath := writeFile(t, dir, "stack.yaml", "x: 1\n")

	result, err := UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{BasePath: dir}, "raw: !include.raw ./template.yaml\n", manifestPath)
	require.NoError(t, err)
	assert.Equal(t, "Value: !env X\n", result["raw"], "!include.raw still returns the file text verbatim")
}
