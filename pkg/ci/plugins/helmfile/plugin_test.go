package helmfile

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/templates"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPlugin_GetType(t *testing.T) {
	assert.Equal(t, "helmfile", (&Plugin{}).GetType())
}

func TestPlugin_GetHookBindings(t *testing.T) {
	bindings := (&Plugin{}).GetHookBindings()
	require.Len(t, bindings, 6)

	for _, event := range []string{
		"after.helmfile.template",
		"after.helmfile.diff",
		"after.helmfile.apply",
		"after.helmfile.sync",
		"after.helmfile.deploy",
		"after.helmfile.destroy",
	} {
		t.Run(event, func(t *testing.T) {
			binding := plugin.HookBindings(bindings).GetBindingForEvent(event)
			require.NotNil(t, binding)
			assert.NotNil(t, binding.Handler)
		})
	}
}

func TestHelmfileTemplateName(t *testing.T) {
	assert.Equal(t, "apply", helmfileTemplateName("sync"))
	assert.Equal(t, "apply", helmfileTemplateName("deploy"))
	assert.Equal(t, "diff", helmfileTemplateName("diff"))
}

func TestPlugin_BuildTemplateContext(t *testing.T) {
	ctx := (&Plugin{}).buildTemplateContext(&plugin.HookContext{
		Command:      "diff",
		Output:       "helmfile diff output\n",
		CommandError: errors.New("diff failed"),
		ExitCode:     1,
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "echo-server",
			Stack:            "tenant1-ue2-dev",
		},
	})

	assert.Equal(t, "echo-server", ctx.Component)
	assert.Equal(t, "tenant1-ue2-dev", ctx.Stack)
	assert.Equal(t, "diff", ctx.Command)
	assert.Equal(t, "helmfile diff output", ctx.Output)
	require.True(t, ctx.Result.HasErrors)
	assert.Equal(t, []string{"diff failed"}, ctx.Result.Errors)
}

func TestTemplateRendering(t *testing.T) {
	ctx := (&Plugin{}).buildTemplateContext(&plugin.HookContext{
		Command: "apply",
		Output:  "UPDATED RELEASES:\nname: echo-server",
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "echo-server",
			Stack:            "tenant1-ue2-dev",
		},
	})

	rendered, err := templates.NewLoader(nil).LoadAndRender("helmfile", "apply", defaultTemplates, ctx)
	require.NoError(t, err)
	assert.Contains(t, rendered, "Helmfile Apply Summary")
	assert.Contains(t, rendered, "atmos helmfile apply echo-server -s tenant1-ue2-dev")
	assert.Contains(t, rendered, "UPDATED RELEASES")
}
