package utils

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// upperRewriter is a test foreign tag: `!x-upper value` becomes `{Upper: VALUE}`.
func upperRewriter(node *yaml.Node) error {
	inner := *node
	inner.Tag = "!!str"
	inner.Value = stringsToUpper(inner.Value)
	key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "Upper"}
	*node = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{key, &inner}}
	return nil
}

func stringsToUpper(s string) string {
	out := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

func TestForeignTagRewriter_RewritesInPlaceAndWalksChildren(t *testing.T) {
	RegisterForeignTagRewriter("!x-upper", upperRewriter)
	RegisterForeignTagRewriter("!x-wrap", func(node *yaml.Node) error {
		inner := *node
		inner.Tag = ""
		key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "Wrapped"}
		*node = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{key, &inner}}
		return nil
	})

	manifest := `
value: !x-upper hello
nested: !x-wrap
  - !x-upper inner
  - !env SOME_VAR
`
	result, err := UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{}, manifest, "test.yaml")
	require.NoError(t, err)

	assert.Equal(t, map[string]any{"Upper": "HELLO"}, result["value"])

	nested, ok := result["nested"].(map[string]any)
	require.True(t, ok, "wrapper mapping")
	items, ok := nested["Wrapped"].([]any)
	require.True(t, ok, "wrapped sequence")
	require.Len(t, items, 2)
	assert.Equal(t, map[string]any{"Upper": "INNER"}, items[0], "a foreign tag nested inside a rewritten value is rewritten too")
	assert.Equal(t, "!env SOME_VAR", items[1], "an Atmos function nested inside a rewritten value is deferred, not dropped")
}

func TestForeignTagRewriter_ErrorIsWrapped(t *testing.T) {
	RegisterForeignTagRewriter("!x-fail", func(*yaml.Node) error { return errors.New("boom") })

	_, err := UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{}, "a: !x-fail x\n", "test.yaml")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnsupportedYamlTag)
	assert.Contains(t, err.Error(), "!x-fail")
	assert.Contains(t, err.Error(), "boom")
}

func TestForeignTagHint_UsedForRecognizedUnsupportedTag(t *testing.T) {
	RegisterForeignTagHint(func(tag string) []string {
		if tag == "!x-other" {
			return []string{"use !env instead", "see the docs"}
		}
		return nil
	})

	_, err := UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{}, "a: !x-other x\n", "test.yaml")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnsupportedYamlTag)
	assert.True(t, errUtils.HasHint(err, "use !env instead"), "the registered hint is attached")
	assert.True(t, errUtils.HasHint(err, "see the docs"), "every hint line is attached")
	assert.NotContains(t, err.Error(), "Supported tags are", "the generic list is replaced by the registered hint")

	// A tag no hint provider recognizes keeps the generic message.
	_, err = UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{}, "a: !x-unknown x\n", "test.yaml")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnsupportedYamlTag)
	assert.Contains(t, err.Error(), "Supported tags are")
}
