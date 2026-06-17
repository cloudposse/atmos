package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/templates"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPlugin_GetType(t *testing.T) {
	assert.Equal(t, "helm", (&Plugin{}).GetType())
}

func TestPlugin_GetHookBindings(t *testing.T) {
	bindings := (&Plugin{}).GetHookBindings()
	require.Len(t, bindings, 5)

	for _, event := range []string{
		"after.helm.template",
		"after.helm.diff",
		"after.helm.apply",
		"after.helm.deploy",
		"after.helm.delete",
	} {
		t.Run(event, func(t *testing.T) {
			binding := plugin.HookBindings(bindings).GetBindingForEvent(event)
			require.NotNil(t, binding)
			assert.NotNil(t, binding.Handler)
		})
	}
}

func TestHelmTemplateName(t *testing.T) {
	assert.Equal(t, "template", helmTemplateName("render"))
	assert.Equal(t, "diff", helmTemplateName("plan"))
	assert.Equal(t, "apply", helmTemplateName("deploy"))
	assert.Equal(t, "delete", helmTemplateName("destroy"))
	assert.Equal(t, "diff", helmTemplateName("diff"))
}

func TestPlugin_BuildTemplateContext(t *testing.T) {
	ctx := (&Plugin{}).buildTemplateContext(&plugin.HookContext{
		Command: "deploy",
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "nginx",
			Stack:            "plat-ue2-dev",
		},
		Aggregate: map[string]any{
			"chart":          "bitnami/nginx",
			"release_name":   "nginx",
			"namespace":      "apps",
			"target":         "deployment-repo",
			"object_count":   2,
			"object_kinds":   []any{"Service", "Deployment"},
			"manifest_bytes": 1234,
		},
	})

	assert.Equal(t, "nginx", ctx.Component)
	assert.Equal(t, "plat-ue2-dev", ctx.Stack)
	assert.Equal(t, "deploy", ctx.Command)
	assert.Equal(t, "bitnami/nginx", ctx.Chart)
	assert.Equal(t, "nginx", ctx.ReleaseName)
	assert.Equal(t, "apps", ctx.Namespace)
	assert.Equal(t, "deployment-repo", ctx.Target)
	assert.Equal(t, 2, ctx.ObjectCount)
	assert.Equal(t, 1234, ctx.ManifestBytes)
	assert.Equal(t, []string{"Deployment", "Service"}, ctx.ObjectKinds)
}

func TestTemplateRendering(t *testing.T) {
	ctx := (&Plugin{}).buildTemplateContext(&plugin.HookContext{
		Command: "apply",
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "nginx",
			Stack:            "plat-ue2-dev",
		},
		Aggregate: Summary{
			Chart:         "bitnami/nginx",
			ReleaseName:   "nginx",
			Namespace:     "apps",
			Target:        "kubernetes",
			ObjectCount:   2,
			ObjectKinds:   []string{"Deployment", "Service"},
			ManifestBytes: 1234,
		},
	})

	rendered, err := templates.NewLoader(nil).LoadAndRender("helm", "apply", defaultTemplates, ctx)
	require.NoError(t, err)
	assert.Contains(t, rendered, "Helm Apply Summary")
	assert.Contains(t, rendered, "bitnami/nginx")
	assert.Contains(t, rendered, "Deployment")
}
