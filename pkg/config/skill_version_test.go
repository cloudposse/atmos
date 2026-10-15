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
