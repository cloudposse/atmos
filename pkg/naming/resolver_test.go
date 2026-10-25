package naming

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// A standard evaluator verifies shared resolution without the command execution package.
func literalTemplate(_ *schema.AtmosConfiguration, name, source string, data any, _ bool) (string, error) {
	tmpl, err := template.New(name).Option("missingkey=error").Parse(source)
	if err != nil {
		return "", err
	}
	var result bytes.Buffer
	err = tmpl.Execute(&result, data)
	return result.String(), err
}

func TestResolverStackContextPrecedence(t *testing.T) {
	resolver := Resolver{Template: literalTemplate, StackTemplate: literalTemplate}
	for _, tc := range []struct {
		name, explicit, template, pattern, want, failure string
	}{
		{name: "manifest wins", explicit: "explicit", template: "{{ .missing }}", pattern: "{tenant}", want: "explicit"},
		{name: "template wins", template: "{{ .vars.stage }}", pattern: "{tenant}", want: "dev"},
		{name: "pattern", pattern: "{stage}", want: "dev"},
		{name: "filename", want: "deploy/app"},
		{name: "template missing input", template: "{{ .missing }}", failure: "missing"},
		{name: "pattern missing input", pattern: "{tenant}", failure: "tenant"},
		{name: "computed manifest", explicit: "!starlark\n'prod'", failure: "!starlark"},
		{name: "computed template result", template: "!starlark\n'prod'", failure: "!starlark"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{Stacks: schema.Stacks{NameTemplate: tc.template, NamePattern: tc.pattern}}
			vars := map[string]any{"stage": "dev"}
			info := schema.ConfigAndStacksInfo{Stack: "deploy/app", ComponentFromArg: "alias/app", BaseComponentPath: "physical/app", ComponentVarsSection: vars, ComponentSection: map[string]any{"vars": vars}}
			err := resolver.ProcessStackContextPrefix(config, &info, "deploy/app", tc.explicit)
			if tc.failure != "" {
				require.ErrorContains(t, err, tc.failure)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, info.ContextPrefix)
			require.Equal(t, "alias/app", info.Context.Component)
			require.Equal(t, "physical/app", info.Context.BaseComponent)
			if tc.explicit != "" || tc.pattern != "" && tc.template == "" {
				require.Equal(t, "dev", info.Context.Stage)
			}
		})
	}
}

func TestResolverWorkspaceOverridesAndErrors(t *testing.T) {
	resolver := Resolver{Template: literalTemplate, StackTemplate: literalTemplate}
	for _, tc := range []struct {
		name, stackTemplate, want string
		metadata                  map[string]any
		fail                      bool
	}{
		{name: "template override", metadata: map[string]any{"terraform_workspace_template": "{{ .atmos_component }}", "terraform_workspace_pattern": "ignored", "terraform_workspace": "also-ignored"}, want: "alias-app"},
		{name: "literal override", metadata: map[string]any{"terraform_workspace": "custom/workspace"}, want: "custom-workspace"},
		{name: "aliased component default", want: "deploy-dev-alias-app"},
		{name: "invalid stack template", stackTemplate: "{{ .missing }}", fail: true},
		{name: "invalid workspace template", metadata: map[string]any{"terraform_workspace_template": "{{ .missing }}"}, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{Stacks: schema.Stacks{NameTemplate: tc.stackTemplate}}
			info := schema.ConfigAndStacksInfo{Stack: "deploy/dev", Context: schema.Context{Component: "alias/app", BaseComponent: "physical/app"}, ComponentSection: map[string]any{"atmos_component": "alias/app"}, ComponentMetadataSection: tc.metadata}
			workspace, err := resolver.BuildTerraformWorkspace(config, &info)
			if tc.fail {
				require.Error(t, err)
				require.Empty(t, workspace)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, workspace)
		})
	}
}

func TestResolverRejectsComputedContext(t *testing.T) {
	info := schema.ConfigAndStacksInfo{Stack: "dev", ComponentVarsSection: map[string]any{"stage": "!starlark\n'prod'"}}
	config := &schema.AtmosConfiguration{Stacks: schema.Stacks{NamePattern: "{stage}"}}
	err := (Resolver{}).ProcessStackContextPrefix(config, &info, "dev", "")
	require.ErrorIs(t, err, errUtils.ErrStarlarkStackIdentity)
}
