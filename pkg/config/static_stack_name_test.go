package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/function/literalsource"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRenderStaticStackName(t *testing.T) {
	for _, tc := range []struct {
		name, source, want string
		wantErr            bool
	}{
		{"fields", "{{ .vars.stage }}", "dev", false},
		{"pure formatting", "{{ printf `%s-west` .vars.stage }}", "dev-west", false},
		{"literal custom delimiters", "[[ .vars.stage ]]", "[[ .vars.stage ]]", false},
		{"missing field", "{{ .vars.missing }}", "", true},
		{"external function", "{{ env.Getenv `SHOULD_NOT_READ` }}", "", true},
		{"invalid template", "{{", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RenderStaticStackName(tc.source, "file", map[string]any{"vars": map[string]any{"stage": "dev"}})
			if tc.wantErr {
				require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
				require.ErrorContains(t, err, "file")
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			}
		})
	}
}

func TestStaticStackManifestName(t *testing.T) {
	for _, tc := range []struct {
		name, source, want string
		delimiters         []string
		wantErr            bool
	}{
		{name: "plain", source: "dev", want: "dev"},
		{name: "literal", source: literalsource.Encode("{{ .vars.stage }}"), want: "{{ .vars.stage }}"},
		{name: "default template", source: "{{ .vars.stage }}", wantErr: true},
		{name: "custom template", source: "[[ .vars.stage ]]", delimiters: []string{"[[", "]]"}, wantErr: true},
		{name: "inactive delimiters", source: "{{ .vars.stage }}", delimiters: []string{"[[", "]]"}, want: "{{ .vars.stage }}"},
		{name: "native function", source: (yamlsource.Source{Tag: "!env", Argument: "STAGE"}).Encode(), wantErr: true},
		{name: "native starlark", source: (yamlsource.Source{Tag: "!starlark", Argument: "result = 'dev'"}).Encode(), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			config.Templates.Settings.Delimiters = tc.delimiters
			got, err := StaticStackManifestName(config, "manifest.yaml", tc.source)
			if tc.wantErr {
				require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
				require.ErrorContains(t, err, "manifest.yaml")
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			}
		})
	}
	t.Run("invalid delimiters", func(t *testing.T) {
		config := &schema.AtmosConfiguration{}
		config.Templates.Settings.Delimiters = []string{"[["}
		_, err := StaticStackManifestName(config, "manifest.yaml", "dev")
		require.ErrorIs(t, err, errUtils.ErrInvalidTemplateSettings)
	})
}

func TestRenderStaticStackNamePreservesLiteralSource(t *testing.T) {
	encoded := literalsource.Encode("dev")
	vars := map[string]any{"stage": encoded}
	got, err := RenderStaticStackName("{{ .vars.stage }}-west", "manifest.yaml", map[string]any{"vars": vars})
	require.NoError(t, err)
	require.Equal(t, "dev-west", got)
	require.Equal(t, encoded, vars["stage"])
}

func TestValidateStaticStackNamePattern(t *testing.T) {
	config := &schema.AtmosConfiguration{Stacks: schema.Stacks{NamePattern: "{namespace}-{stage}"}}
	dynamic := (yamlsource.Source{Tag: "!env", Argument: "STAGE"}).Encode()
	for _, value := range []string{dynamic, "{{ .vars.stage }}"} {
		err := ValidateStaticStackNamePattern(config, "manifest.yaml", map[string]any{"stage": value})
		require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	}
	require.NoError(t, ValidateStaticStackNamePattern(config, "manifest.yaml", map[string]any{
		"stage": literalsource.Encode("{{ external.stage }}"), "unused": dynamic,
	}))
}

func TestRenderStaticStackNameRejectsNativeSource(t *testing.T) {
	_, err := RenderStaticStackName("prefix-{{ .vars.stage }}", "manifest.yaml", map[string]any{
		"vars": map[string]any{"stage": (yamlsource.Source{Tag: "!env", Argument: "STAGE"}).Encode()},
	})
	require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	require.ErrorContains(t, err, "manifest.yaml")
}
