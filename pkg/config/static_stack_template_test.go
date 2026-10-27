package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/function/literalsource"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStaticStackTemplateRejectsUnresolvedReads(t *testing.T) {
	dynamic := (yamlsource.Source{Tag: "!env", Argument: "STAGE"}).Encode()
	for _, source := range []string{
		"{{ if eq .vars.stage `dev` }}dev{{ else }}other{{ end }}",
		"{{ printf `%.3s` .vars.stage }}",
		"{{ if eq $.vars.stage `dev` }}dev{{ else }}other{{ end }}",
		"{{ $stage := .vars.stage }}{{ if $stage }}dev{{ end }}",
		"{{ with .vars }}{{ if .stage }}dev{{ end }}{{ end }}",
		"{{ range .items }}{{ . }}{{ end }}",
		"{{ index .vars `stage` }}",
		"{{ (.vars).stage }}",
		"{{ define `name` }}{{ .stage }}{{ end }}{{ template `name` .vars }}",
		"{{ printf `%v` . }}",
		"{{ with .known }}{{ $.vars.stage }}{{ end }}",
		"{{ range .knownItems }}{{ $.vars.stage }}{{ end }}",
		"{{ with .empty }}static{{ else }}{{ .vars.stage }}{{ end }}",
		"{{ range .emptyItems }}static{{ else }}{{ .vars.stage }}{{ end }}",
	} {
		t.Run(source, func(t *testing.T) {
			_, err := RenderStaticStackName(&schema.AtmosConfiguration{}, source, "manifest.yaml", map[string]any{
				"vars": map[string]any{"stage": dynamic}, "items": []any{dynamic},
				"known": "dev", "knownItems": []any{"dev"}, "empty": "", "emptyItems": []any{},
			})
			require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
			require.ErrorContains(t, err, "manifest.yaml")
			require.NotContains(t, err.Error(), dynamic)
		})
	}
}

func TestStaticStackTemplatePreservesKnownReads(t *testing.T) {
	dynamic := (yamlsource.Source{Tag: "!env", Argument: "UNUSED"}).Encode()
	for _, source := range []string{
		"{{ if eq .vars.stage `dev` }}dev{{ else }}other{{ end }}",
		"{{ printf `%.3s` .vars.stage }}",
		"{{ $stage := .vars.stage }}{{ $stage }}",
		"{{ $.vars.stage }}",
	} {
		t.Run(source, func(t *testing.T) {
			for _, stage := range []string{"dev", literalsource.Encode("dev")} {
				section := map[string]any{"vars": map[string]any{"stage": stage, "unused": dynamic}}
				got, err := RenderStaticStackName(&schema.AtmosConfiguration{}, source, "manifest.yaml", section)
				require.NoError(t, err)
				require.Equal(t, "dev", got)
				require.Equal(t, stage, section["vars"].(map[string]any)["stage"])
			}
		})
	}
}

func TestStaticStackTemplateExpressionEditions(t *testing.T) {
	for _, tc := range []struct {
		name, value, edition string
		delims               []string
		wantError            bool
	}{
		{name: "default expression", value: "{{ .vars.stage }}", wantError: true},
		{name: "custom expression", value: "[[ .vars.stage ]]", delims: []string{"[[", "]]"}, wantError: true},
		{name: "inactive expression", value: "{{ .vars.stage }}", delims: []string{"[[", "]]"}},
		{name: "legacy function", value: "!env STAGE", edition: "2026-01-01", wantError: true},
		{name: "explicit edition string", value: "!env STAGE"},
		{name: "literal expression", value: literalsource.Encode("{{ external.stage }}")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{Edition: tc.edition}
			config.Templates.Settings.Delimiters = tc.delims
			got, err := RenderStaticStackName(config, "{{ .vars.stage }}", "manifest.yaml", map[string]any{"vars": map[string]any{"stage": tc.value}})
			if tc.wantError {
				require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
			} else {
				require.NoError(t, err)
				want, _ := literalsource.Decode(tc.value)
				require.Equal(t, want, got)
			}
		})
	}
}

func TestStaticStackTemplateKnownNestedContexts(t *testing.T) {
	for _, source := range []string{
		"{{ with .vars }}{{ .stage }}{{ end }}",
		"{{ range .items }}{{ . }}{{ end }}",
		"{{ index .vars `stage` }}",
		"{{ (.vars).stage }}",
		"{{ define `name` }}{{ .stage }}{{ end }}{{ template `name` .vars }}",
		"{{ define `name` }}{{ . }}{{ end }}{{ template `name` .vars.stage }}",
	} {
		t.Run(source, func(t *testing.T) {
			got, err := RenderStaticStackName(&schema.AtmosConfiguration{}, source, "manifest.yaml", map[string]any{
				"vars": map[string]any{"stage": "dev"}, "items": []any{"dev"},
				"unused": (yamlsource.Source{Tag: "!env", Argument: "UNUSED"}).Encode(),
			})
			require.NoError(t, err)
			require.Equal(t, "dev", got)
		})
	}
}
