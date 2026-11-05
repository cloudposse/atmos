package naming

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpgradeTokensAndQuotedLiterals(t *testing.T) {
	for _, tc := range []struct {
		before, after string
		unknown       bool
	}{
		{"{namespace}-{tenant}-{environment}-{stage}-{region}", "{{ .vars.namespace }}-{{ .vars.tenant }}-{{ .vars.environment }}-{{ .vars.stage }}-{{ .vars.region }}", false},
		{"{component}-{base-component}-{attributes}", `{{ .atmos_component }}-{{ .component }}-{{ join "-" .vars.attributes }}`, false},
		{"{{ .vars.stage }}-{component}", "{{.vars.stage}}-{{ .atmos_component }}", false},
		{`{{ "{component}" }}`, `{{ "{component}" }}`, false},
		{"{env}-{name}-{stack}-{component-path}-{workspace}-{terraform_workspace}", "{env}-{name}-{stack}-{component-path}-{workspace}-{terraform_workspace}", true},
	} {
		t.Run(tc.before, func(t *testing.T) {
			after, _, unknown := Upgrade(tc.before)
			require.Equal(t, tc.after, after)
			require.Equal(t, tc.unknown, len(unknown) > 0)
		})
	}
}

func TestSafeNamingTemplates(t *testing.T) {
	for _, value := range []string{`{{ .vars.stage }}`, `{{ .atmos_component }}`, `{{ join "-" .vars.attributes }}`, `{{ "{component}" }}`} {
		require.True(t, SafeTemplate(value), value)
	}
	for _, value := range []string{`{{ env "TOKEN" }}`, `{{ readFile "secret" }}`, `{{ atmos.Component "vpc" }}`, `{{ index . .key }}`, `{{ . }}`, `{{ range .vars }}{{ . }}{{ end }}`, `{{ .vars.x`} {
		require.False(t, SafeTemplate(value), value)
	}
	require.False(t, Field([]string{"vars", "terraform_workspace_pattern"}))
	require.False(t, Field([]string{"components", "helmfile", "example", "metadata", "terraform_workspace_pattern"}))
}
