package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIncludeYAMLReleaseQueryCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		query   string
		want    any
	}{
		{"all map results", "- {first: A}\n- {second: B}\n", ".[]", map[string]any{"first": "A", "second": "B"}},
		{"all scalar results", "- first\n- second\n", ".[]", "first second"},
		{"empty query default", "foo: ORIGINAL\n", `.missing // ""`, nil},
		{"empty selected scalar", "foo: ''\n", `.foo`, nil},
		{"empty literal data", "foo: ''\n", `.`, map[string]any{"foo": ""}},
		{"missing selected scalar", "foo: ORIGINAL\n", `.missing`, nil},
		{"numeric string", "foo: '001234567890'\n", `.foo`, "001234567890"},
		{"quoted eval", "foo: ORIGINAL\n", `.foo | "eval"`, "eval"},
		{"quoted eval pipeline", "foo: ORIGINAL\n", `.foo | "eval" | length`, 4},
		{"spaced eval function", "foo: ORIGINAL\npath: .foo\n", `. | eval (.path)`, "ORIGINAL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "data.yaml", tc.content)
			manifest := writeFile(t, dir, "stack.yaml", "")
			result, err := UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{BasePath: dir},
				fmt.Sprintf("included: !include './data.yaml %s'\n", tc.query), manifest)
			require.NoError(t, err)
			assert.Equal(t, tc.want, result["included"])
		})
	}
}

func TestIncludeYAMLDataAcceptsForeignTags(t *testing.T) {
	for _, query := range []string{"", " ."} {
		t.Run("query="+query, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "data.yaml", "scalar: !vault placeholder\nmap: !Custom {first: A, second: B}\nlist: !Custom [first, second]\n")
			manifest := writeFile(t, dir, "stack.yaml", "")
			result, err := UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{BasePath: dir},
				"included: !include ./data.yaml"+query+"\n", manifest)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{
				"scalar": "placeholder",
				"map":    map[string]any{"first": "A", "second": "B"},
				"list":   []any{"first", "second"},
			}, result["included"])

			// Opting into execution must not silently accept an unsupported function.
			_, err = UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{BasePath: dir},
				"included: !include ./data.yaml"+query+" | eval\n", manifest)
			require.ErrorIs(t, err, errUtils.ErrUnsupportedYamlTag)
		})
	}
}

func TestIncludeYAMLQueryRejectsDuplicateMappingKeys(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "data.yaml", "- {value: first}\n- {value: second}\n")
	manifest := writeFile(t, dir, "stack.yaml", "")
	_, err := UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{BasePath: dir},
		"included: !include ./data.yaml .[]\n", manifest)
	// The release rejected ambiguous mappings; success with only the first value loses data.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already defined")
}
