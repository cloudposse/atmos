package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestEnsureLiteralStackIdentity(t *testing.T) {
	computed := starlarkTestSource(`return "prod"`)
	tests := []struct {
		name      string
		stackName string
		section   map[string]any
		wantErr   bool
		wantField string
	}{
		{name: "literal name", stackName: "plat-ue2-prod", section: map[string]any{"vars": map[string]any{"stage": "prod"}}},
		{name: "empty name", stackName: "", section: nil},
		{name: "a quoted !starlark string is data", stackName: "!starlark return 1", section: nil},
		{
			name:      "whole name is a computed value",
			stackName: computed,
			section:   map[string]any{"vars": map[string]any{"stage": computed}},
			wantErr:   true, wantField: "vars.stage",
		},
		{
			name:      "computed value embedded in a longer name",
			stackName: "plat-" + computed,
			section:   map[string]any{"vars": map[string]any{"environment": "plat", "stage": computed}},
			wantErr:   true, wantField: "vars.stage",
		},
		{
			name:      "an unrelated computed field is not blamed",
			stackName: "plat-" + computed,
			section: map[string]any{"vars": map[string]any{
				"a_other": starlarkTestSource(`return "other"`),
				"stage":   computed,
			}},
			wantErr: true, wantField: "vars.stage",
		},
		{
			name:      "nested and list values are searched",
			stackName: computed,
			section:   map[string]any{"settings": map[string]any{"ctx": map[string]any{"tags": []any{"x", computed}}}},
			wantErr:   true, wantField: "settings.ctx.tags.1",
		},
		{name: "computed name that no field explains", stackName: computed, section: map[string]any{}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ensureLiteralStackIdentity("deploy/prod", tc.stackName, tc.section)
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrStarlarkStackIdentity)
			assert.Contains(t, err.Error(), "stack identity cannot be computed")
			manifest, _ := errUtils.GetContext(err, "manifest")
			assert.Equal(t, "deploy/prod", manifest)
			field, hasField := errUtils.GetContext(err, "field")
			assert.Equal(t, tc.wantField != "", hasField)
			assert.Equal(t, tc.wantField, field)
		})
	}
}

func TestResolveSpaceliftContextPrefixRejectsComputedIdentity(t *testing.T) {
	computed := starlarkTestSource(`return "prod"`)
	config := &schema.AtmosConfiguration{}
	_, err := ResolveSpaceliftContextPrefix(config, "deploy/prod", &schema.Context{}, map[string]any{"stage": computed}, SpaceliftStackNaming{NameTemplate: "{{ .vars.stage }}"})
	require.ErrorIs(t, err, errUtils.ErrStarlarkStackIdentity)

	prefix, err := ResolveSpaceliftContextPrefix(config, "deploy/prod", &schema.Context{}, map[string]any{"stage": "prod"}, SpaceliftStackNaming{NameTemplate: "{{ .vars.stage }}"})
	require.NoError(t, err)
	assert.Equal(t, "prod", prefix)
}

func TestStackNameTemplatesRejectComputedInputsBeforeTransformation(t *testing.T) {
	t.Parallel()
	computed := starlarkTestSource(`return "prod"`)
	section := map[string]any{"vars": map[string]any{"stage": computed, "environment": "dev"}}
	for _, source := range []string{
		`{{ printf "%.4s" .vars.stage }}`,
		`{{ .vars.stage | sha256sum }}`,
		`{{ slice .vars.stage 0 4 }}`,
		`{{ printf "%.4s" (index .vars "stage") }}`,
		`{{ get .vars "stage" | sha256sum }}`,
		`{{ dig "stage" "fallback" .vars | sha256sum }}`,
		`{{ range $key, $value := .vars }}{{ if eq $key "stage" }}{{ printf "%.4s" $value }}{{ end }}{{ end }}`,
		`{{ pluck "stage" .vars | printf "%.4s" }}`,
		`{{ $v := .vars }}{{ printf "%.4s" $v.stage }}`,
		`{{ if .vars.stage }}prod{{ else }}dev{{ end }}`,
		`{{ printf "%.4s" .vars }}`,
		`{{ .vars | printf "%.4s" }}`,
		`{{ with .vars }}{{ .stage | sha256sum }}{{ end }}`,
		`{{ define "name" }}{{ printf "%.4s" .stage }}{{ end }}{{ template "name" .vars }}`,
	} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			_, err := processStackNameTemplate(&schema.AtmosConfiguration{}, "deploy/prod", source, section, false)
			require.ErrorIs(t, err, errUtils.ErrStarlarkStackIdentity)
			field, ok := errUtils.GetContext(err, "field")
			require.True(t, ok)
			assert.Equal(t, "vars.stage", field)
		})
	}
}

func TestStackNameTemplatesAllowUnrelatedComputedInputs(t *testing.T) {
	t.Parallel()
	section := map[string]any{"vars": map[string]any{
		"stage": starlarkTestSource(`return "prod"`), "environment": "dev",
	}}
	for _, source := range []string{
		`{{ .vars.environment }}`,
		`{{ index .vars "environment" }}`,
		`{{ get .vars "environment" }}`,
		`{{ default .vars "dev" }}`,
		`{{ coalesce "dev" .vars }}`,
		`{{ and .vars "dev" }}`,
		`{{ or "dev" .vars }}`,
		`{{ if empty .vars }}prod{{ else }}dev{{ end }}`,
		`{{ range $key, $value := .vars }}{{ if eq $key "environment" }}{{ $value }}{{ end }}{{ end }}`,
		`{{ $v := .vars }}{{ $v.environment }}`,
		`{{ with .vars }}{{ .environment }}{{ end }}`,
		`{{ if false }}{{ .vars.stage }}{{ else }}dev{{ end }}`,
		`{{ if and false .vars.stage }}prod{{ else }}dev{{ end }}`,
	} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			got, err := processStackNameTemplate(&schema.AtmosConfiguration{}, "deploy/dev", source, section, false)
			require.NoError(t, err)
			assert.Equal(t, "dev", got)
		})
	}
	// Non-naming templates still preserve computed source for the later Starlark pass.
	got, err := ProcessTmpl(&schema.AtmosConfiguration{}, "component-template", `{{ .vars.stage }}`, section, false)
	require.NoError(t, err)
	assert.Equal(t, section["vars"].(map[string]any)["stage"], got)
}
