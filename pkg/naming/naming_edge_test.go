package naming

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpgradeConditionalBranches(t *testing.T) {
	for _, tc := range []struct{ name, before, after string }{
		{"if", `{{ if .vars.stage }}{stage}{{ else }}{component}{{ end }}`, `{{if .vars.stage}}{{ .vars.stage }}{{else}}{{ .atmos_component }}{{end}}`},
		{"range", `{{ range .vars.attributes }}{stage}{{ else }}{component}{{ end }}`, `{{range .vars.attributes}}{{ .vars.stage }}{{else}}{{ .atmos_component }}{{end}}`},
		{"with", `{{ with .vars }}{stage}{{ else }}{component}{{ end }}`, `{{with .vars}}{{ .vars.stage }}{{else}}{{ .atmos_component }}{{end}}`},
		{"without else", `{{ if .vars.stage }}{stage}-{component}{{ end }}`, `{{if .vars.stage}}{{ .vars.stage }}-{{ .atmos_component }}{{end}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			after, tokens, unsupported := Upgrade(tc.before)
			require.Equal(t, tc.after, after)
			require.Equal(t, []string{"{stage}", "{component}"}, tokens)
			require.Empty(t, unsupported)
		})
	}
}

func TestUpgradeMalformedAndRepeatedTokens(t *testing.T) {
	malformed := `{{ if .vars.stage }{component}`
	after, tokens, unsupported := Upgrade(malformed)
	require.Equal(t, malformed, after)
	require.Empty(t, tokens)
	require.Empty(t, unsupported)

	after, tokens, unsupported = Upgrade(`{stage}-{stage}-{unknown}-{unknown}`)
	require.Equal(t, `{{ .vars.stage }}-{{ .vars.stage }}-{unknown}-{unknown}`, after)
	require.Equal(t, []string{"{stage}", "{unknown}"}, tokens)
	require.Equal(t, []string{"{unknown}"}, unsupported)
}

func TestNamingFieldsExcludeUserData(t *testing.T) {
	for _, path := range [][]string{
		{"stacks", "name_pattern"},
		{"stacks", "name_template"},
		{"metadata", "terraform_workspace_pattern"},
		{"metadata", "terraform_workspace_template"},
		{"components", "helmfile", "cluster_name_pattern"},
		{"components", "helmfile", "cluster_name_template"},
		{"components", "terraform", "app", "metadata", "terraform_workspace_pattern"},
		{"components", "terraform", "app", "metadata", "terraform_workspace_template"},
	} {
		require.True(t, Field(path), "%v", path)
	}
	for _, path := range [][]string{
		nil,
		{"name_pattern"},
		{"vars", "name_pattern"},
		{"stacks", "other"},
		{"components", "terraform", "cluster_name_pattern"},
		{"components", "terraform", "app", "terraform_workspace_pattern"},
		{"components", "terraform", "app", "vars", "terraform_workspace_pattern"},
		{"components", "terraform", "app", "metadata", "unrelated"},
	} {
		require.False(t, Field(path), "%v", path)
	}
	require.False(t, SafeTemplate(`{{ .secrets.token }}`), "unapproved roots must require manual review")
}
