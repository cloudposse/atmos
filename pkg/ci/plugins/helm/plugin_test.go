package helm

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
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

func TestNormalizeSummary(t *testing.T) {
	assert.Equal(t, Summary{Component: "app"}, normalizeSummary(Summary{Component: "app"}))
	assert.Equal(t, Summary{Component: "app"}, normalizeSummary(&Summary{Component: "app"}))
	assert.Equal(t, Summary{}, normalizeSummary((*Summary)(nil)))
	assert.Equal(t, Summary{}, normalizeSummary("not-summary"))

	got := normalizeSummary(map[string]any{
		"component":      "app",
		"stack":          "dev",
		"command":        "diff",
		"chart":          "bitnami/nginx",
		"release_name":   "nginx",
		"namespace":      "apps",
		"target":         "git",
		"object_count":   int64(2),
		"object_kinds":   []any{"Service", "", "Deployment"},
		"manifest_bytes": float64(123),
		"message":        42,
		"diff":           "diff text",
	})
	assert.Equal(t, "app", got.Component)
	assert.Equal(t, "dev", got.Stack)
	assert.Equal(t, "diff", got.Command)
	assert.Equal(t, "bitnami/nginx", got.Chart)
	assert.Equal(t, "nginx", got.ReleaseName)
	assert.Equal(t, "apps", got.Namespace)
	assert.Equal(t, "git", got.Target)
	assert.Equal(t, 2, got.ObjectCount)
	assert.Equal(t, []string{"Deployment", "Service"}, got.ObjectKinds)
	assert.Equal(t, 123, got.ManifestBytes)
	assert.Equal(t, "42", got.Message)
	assert.Equal(t, "diff text", got.Diff)
}

func TestPluginBuildTemplateContextFallbacksAndErrors(t *testing.T) {
	sentinel := errors.New("helm failed")
	ctx := (&Plugin{}).buildTemplateContext(&plugin.HookContext{
		Command:      "diff",
		CommandError: sentinel,
		ExitCode:     1,
		Info:         &schema.ConfigAndStacksInfo{ComponentFromArg: "nginx", Stack: "dev"},
		Aggregate:    map[string]any{"diff": "diff text"},
	})

	assert.Equal(t, "nginx", ctx.Component)
	assert.Equal(t, "dev", ctx.Stack)
	assert.Equal(t, "diff", ctx.Command)
	assert.True(t, ctx.Result.HasErrors)
	assert.Equal(t, []string{"helm failed"}, ctx.Result.Errors)
	assert.Equal(t, "Diff", ctx.CommandTitle)
	assert.Equal(t, "diff text", ctx.Diff)
}

func TestSummaryEnabledAndPrimitiveConversions(t *testing.T) {
	assert.True(t, isSummaryEnabled(nil))
	assert.True(t, isSummaryEnabled(&schema.AtmosConfiguration{}))

	disabled := false
	assert.False(t, isSummaryEnabled(&schema.AtmosConfiguration{
		CI: schema.CIConfig{Summary: schema.CISummaryConfig{Enabled: &disabled}},
	}))

	assert.Equal(t, "", stringValue(nil))
	assert.Equal(t, "value", stringValue("value"))
	assert.Equal(t, "123", stringValue(123))
	assert.Equal(t, 7, intValue(7))
	assert.Equal(t, 8, intValue(int64(8)))
	assert.Equal(t, 9, intValue(float64(9)))
	assert.Zero(t, intValue("9"))
	assert.Equal(t, []string{"b", "a"}, stringSliceValue([]string{"b", "a"}))
	assert.Nil(t, stringSliceValue(1))
	assert.Equal(t, "Helm", title(""))
}

type fakeProvider struct {
	writer provider.OutputWriter
}

func (f fakeProvider) Name() string                        { return "fake" }
func (f fakeProvider) Detect() bool                        { return true }
func (f fakeProvider) Context() (*provider.Context, error) { return &provider.Context{}, nil }
func (f fakeProvider) GetStatus(context.Context, provider.StatusOptions) (*provider.Status, error) {
	return nil, nil
}
func (f fakeProvider) CreateCheckRun(context.Context, *provider.CreateCheckRunOptions) (*provider.CheckRun, error) {
	return nil, nil
}
func (f fakeProvider) UpdateCheckRun(context.Context, *provider.UpdateCheckRunOptions) (*provider.CheckRun, error) {
	return nil, nil
}
func (f fakeProvider) PostComment(context.Context, *provider.PostCommentOptions) (*provider.Comment, error) {
	return nil, nil
}
func (f fakeProvider) OutputWriter() provider.OutputWriter            { return f.writer }
func (f fakeProvider) ResolveBase() (*provider.BaseResolution, error) { return nil, nil }

type fakeWriter struct {
	summary string
	err     error
}

func (f *fakeWriter) WriteOutput(string, string) error { return nil }
func (f *fakeWriter) WriteSummary(content string) error {
	f.summary = content
	return f.err
}

func TestPluginOnAfterOperation(t *testing.T) {
	p := &Plugin{}
	disabled := false
	writer := &fakeWriter{}

	err := p.onAfterOperation(&plugin.HookContext{
		Config:         &schema.AtmosConfiguration{CI: schema.CIConfig{Summary: schema.CISummaryConfig{Enabled: &disabled}}},
		Provider:       fakeProvider{writer: writer},
		TemplateLoader: templates.NewLoader(nil),
		Command:        "apply",
	})
	require.NoError(t, err)
	assert.Empty(t, writer.summary)

	err = p.onAfterOperation(&plugin.HookContext{
		Provider:       fakeProvider{},
		TemplateLoader: templates.NewLoader(nil),
		Command:        "apply",
	})
	require.NoError(t, err)

	writer = &fakeWriter{}
	err = p.onAfterOperation(&plugin.HookContext{
		Provider:       fakeProvider{writer: writer},
		TemplateLoader: templates.NewLoader(nil),
		Command:        "render",
		Aggregate:      Summary{Component: "nginx", Chart: "bitnami/nginx"},
	})
	require.NoError(t, err)
	assert.Contains(t, writer.summary, "Helm Template Summary")
	assert.Contains(t, writer.summary, "bitnami/nginx")

	sentinel := errors.New("write failed")
	err = p.onAfterOperation(&plugin.HookContext{
		Provider:       fakeProvider{writer: &fakeWriter{err: sentinel}},
		TemplateLoader: templates.NewLoader(nil),
		Command:        "apply",
	})
	require.ErrorIs(t, err, sentinel)
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
