package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDeferredSkillVersion(t *testing.T) {
	for _, ref := range []string{"!version tool", `"!version tool"`, "main"} {
		t.Run(ref, func(t *testing.T) {
			var node yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte("ai:\n  skills:\n    source:\n      source: example/repo\n      ref: "+ref+"\n"), &node))
			v := viper.New()
			require.NoError(t, v.MergeConfigMap(map[string]any{"ai": map[string]any{"skills": map[string]any{"source": map[string]any{"source": "example/repo", "ref": "main"}}}}))
			require.NoError(t, processNode(&node, v, ""))
			// Mirror Viper's initial read for ordinary scalars.
			if ref != "!version tool" {
				var values map[string]any
				require.NoError(t, yaml.Unmarshal([]byte("ai:\n  skills:\n    source:\n      source: example/repo\n      ref: "+ref+"\n"), &values))
				require.NoError(t, v.MergeConfigMap(values))
			}
			var config schema.AtmosConfiguration
			require.NoError(t, v.Unmarshal(&config, atmosDecodeHook()))
			result := config.AI.Skills["source"].Ref
			if ref == "!version tool" {
				require.Equal(t, "tool", result.Dependency)
				require.Empty(t, result.Literal)
			} else {
				require.Empty(t, result.Dependency)
			}
		})
	}
}

func TestVersionOutsideSkillRefRejected(t *testing.T) {
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("base_path: !version tool"), &node))
	require.Error(t, processNode(&node, viper.New(), ""))
}

// TestLoadConfigRejectsNonScalarSkillVersion prevents malformed tags from being
// decoded as literal refs and bypassing deferred version resolution.
func TestLoadConfigRejectsNonScalarSkillVersion(t *testing.T) {
	for _, ref := range []string{
		"!version {literal: main}",
		"!version {dependency: tool}",
		"!version {}",
		"!version [tool]",
		"!version []",
	} {
		t.Run(ref, func(t *testing.T) {
			writeEditionTestConfig(t, "ai:\n  skills:\n    source:\n      source: example/repo\n      ref: "+ref+"\n")
			_, err := LoadConfig(&schema.ConfigAndStacksInfo{})
			require.ErrorIs(t, err, errUtils.ErrUnsupportedYamlTag)
			require.ErrorContains(t, err, "!version")
			require.ErrorContains(t, err, "ai.skills.source.ref")
		})
	}
}

// TestLoadConfigScalarSkillRefs preserves the distinction between a deferred tag
// and literal text, including a quoted string that looks like a tag.
func TestLoadConfigScalarSkillRefs(t *testing.T) {
	for _, tc := range []struct {
		ref  string
		want schema.SkillRef
	}{
		{ref: "!version tool", want: schema.SkillRef{Dependency: "tool"}},
		{ref: `"!version tool"`, want: schema.SkillRef{Literal: "!version tool"}},
		{ref: "main", want: schema.SkillRef{Literal: "main"}},
	} {
		t.Run(tc.ref, func(t *testing.T) {
			writeEditionTestConfig(t, "ai:\n  skills:\n    source:\n      source: example/repo\n      ref: "+tc.ref+"\n")
			config, err := LoadConfig(&schema.ConfigAndStacksInfo{})
			require.NoError(t, err)
			require.Equal(t, tc.want, config.AI.Skills["source"].Ref)
		})
	}
}

// TestLoadConfigRejectsVersionInSequences ensures sequence traversal cannot bypass
// the restriction of version tags to scalar skill-source refs.
func TestLoadConfigRejectsVersionInSequences(t *testing.T) {
	for _, tc := range []struct {
		value string
		path  string
	}{
		{value: "[!version {literal: main}]", path: "devcontainer.examples[0]"},
		{value: "[!version {}]", path: "devcontainer.examples[0]"},
		{value: "[!version [tool]]", path: "devcontainer.examples[0]"},
		{value: "[!version []]", path: "devcontainer.examples[0]"},
		{value: "[[!version {literal: main}]]", path: "devcontainer.examples[0][0]"},
		{value: "[[!version [tool]]]", path: "devcontainer.examples[0][0]"},
		{value: "[{nested: [!version {}]}]", path: "devcontainer.examples[0].nested[0]"},
		{value: "[!version tool]", path: "devcontainer.examples[0]"},
		{value: "[[!version tool]]", path: "devcontainer.examples[0][0]"},
		{value: "!append [!version {literal: main}]", path: "devcontainer.examples[0]"},
		{value: "!append [[!version []]]", path: "devcontainer.examples[0][0]"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			writeEditionTestConfig(t, "devcontainer:\n  examples: "+tc.value+"\n")
			_, err := LoadConfig(&schema.ConfigAndStacksInfo{})
			require.ErrorIs(t, err, errUtils.ErrUnsupportedYamlTag)
			require.ErrorContains(t, err, "!version")
			require.ErrorContains(t, err, tc.path)
		})
	}
}

// TestSequenceVersionValidationPreservesValues keeps ordinary collections,
// quoted tag-like strings, and supported scalar tags compatible.
func TestSequenceVersionValidationPreservesValues(t *testing.T) {
	t.Setenv("ATMOS_TEST_SEQUENCE_VALUE", "resolved")
	content := `items:
  - {literal: main}
  - ["!version tool", main]
  - !env ATMOS_TEST_SEQUENCE_VALUE
`
	v := viper.New()
	require.NoError(t, preprocessAtmosYamlFunc([]byte(content), v))
	require.Equal(t, []any{
		map[string]any{"literal": "main"},
		[]any{"!version tool", "main"},
		"resolved",
	}, v.Get("items"))
}

// TestLoadConfigRejectsAliasedVersionInSequences covers anchors declared under
// commands, which are decoded separately from the rest of the configuration.
func TestLoadConfigRejectsAliasedVersionInSequences(t *testing.T) {
	for _, list := range []string{"[*version]", "[[*version]]", "!append [*version]"} {
		t.Run(list, func(t *testing.T) {
			writeEditionTestConfig(t, "commands:\n  - name: example\n    env: &version !version {literal: main}\ndevcontainer:\n  examples: "+list+"\n")
			_, err := LoadConfig(&schema.ConfigAndStacksInfo{})
			require.ErrorIs(t, err, errUtils.ErrUnsupportedYamlTag)
			require.ErrorContains(t, err, "!version")
			require.ErrorContains(t, err, "devcontainer.examples[0]")
		})
	}
}

// TestSequenceVersionAliases validates parsed alias targets and terminates on
// cycles while still inspecting subsequent siblings.
func TestSequenceVersionAliases(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		path    string
	}{
		{name: "scalar", content: "anchor: &ref !version tool\nitems: [*ref]", path: "items[0]"},
		{name: "sequence", content: "anchor: &ref !version []\nitems: [*ref]", path: "items[0]"},
		{name: "nested alias", content: "anchor: &ref !version {}\ncontainer: &group {nested: *ref}\nitems: [*group]", path: "items[0].nested"},
		{name: "shared literal", content: "anchor: &ref {literal: \"!version tool\"}\nitems: [*ref, *ref]"},
		{name: "cycle", content: "anchor: &ref [*ref]\nitems: [*ref]"},
		{name: "cycle then invalid sibling", content: "anchor: &ref [*ref, !version {}]\nitems: [*ref]", path: "items[0][1]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var root yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte(tc.content), &root))
			mapping := root.Content[0]
			items := mapping.Content[len(mapping.Content)-1]
			err := rejectSequenceVersionTags(items, "items")
			if tc.path == "" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, errUtils.ErrUnsupportedYamlTag)
				require.ErrorContains(t, err, tc.path)
			}
		})
	}
}
