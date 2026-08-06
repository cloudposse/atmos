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
	require.Len(t, bindings, 7)

	for _, event := range []string{
		"after.helm.template",
		"after.helm.diff",
		"after.helm.plan.aggregate",
		"after.helm.apply",
		"after.helm.deploy",
		"after.helm.apply.aggregate",
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
			Stack:            "dev",
		},
		Aggregate: map[string]any{
			"chart":          "bitnami/nginx",
			"release_name":   "nginx",
			"namespace":      "apps",
			"target":         "deployment-repo",
			"object_count":   2,
			"object_kinds":   []any{"Service", "Deployment"},
			"manifest_bytes": 1234,
			"release": map[string]any{
				"operation": "upgrade",
				"wait":      map[string]any{"strategy": "watcher"},
			},
		},
	})

	assert.Equal(t, "nginx", ctx.Component)
	assert.Equal(t, "dev", ctx.Stack)
	assert.Equal(t, "deploy", ctx.Command)
	assert.Equal(t, "bitnami/nginx", ctx.Chart)
	assert.Equal(t, "nginx", ctx.ReleaseName)
	assert.Equal(t, "apps", ctx.Namespace)
	assert.Equal(t, "deployment-repo", ctx.Target)
	assert.Equal(t, 2, ctx.ObjectCount)
	assert.Equal(t, 1234, ctx.ManifestBytes)
	assert.Equal(t, []string{"Deployment", "Service"}, ctx.ObjectKinds)
	assert.Equal(t, "upgrade", ctx.Lifecycle["operation"])
	assert.Equal(t, "watcher", ctx.Lifecycle["wait"].(map[string]any)["strategy"])
}

func TestNormalizeSummary(t *testing.T) {
	assert.Equal(t, Summary{Component: "app"}, normalizeSummary(Summary{Component: "app"}))
	assert.Equal(t, Summary{Component: "app"}, normalizeSummary(&Summary{Component: "app"}))
	assert.Equal(t, Summary{}, normalizeSummary((*Summary)(nil)))
	assert.Equal(t, Summary{}, normalizeSummary("not-summary"))

	lifecycle := map[string]any{"operation": "install"}
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
		"release":        lifecycle,
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
	assert.Equal(t, map[string]any{"operation": "install"}, got.Lifecycle)

	lifecycle["operation"] = "upgrade"
	assert.Equal(t, "install", got.Lifecycle["operation"])
	got.Lifecycle["timeout"] = "30m0s"
	assert.NotContains(t, lifecycle, "timeout")
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
	assert.Nil(t, mapValue("not-a-map"))
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
	tests := []struct {
		name        string
		command     string
		lifecycle   map[string]any
		contains    []string
		notContains []string
	}{
		{
			name:    "cluster apply",
			command: "apply",
			lifecycle: map[string]any{
				"operation":          "upgrade",
				"wait":               map[string]any{"strategy": "watcher", "jobs": true},
				"timeout":            "30m0s",
				"chart_hooks":        true,
				"on_failure":         "rollback",
				"cleanup_on_failure": true,
				"history":            map[string]any{"max": 10},
			},
			contains: []string{
				"Helm Apply Summary", "bitnami/nginx", "Deployment", "\n### Release lifecycle\n",
				"\n| Operation | `upgrade` |\n",
				"\n| Wait strategy | `watcher` |\n",
				"| Timeout | `30m0s` |",
				"| Chart hooks enabled | `true` |",
				"| Wait for Jobs | `true` |",
				"| On failure | `rollback` |\n" +
					"| Cleanup on failure | `true` |\n" +
					"| Maximum history | `10` |",
			},
		},
		{
			name:    "cluster install",
			command: "apply",
			lifecycle: map[string]any{
				"operation":   "install",
				"wait":        map[string]any{"strategy": "hookOnly", "jobs": false},
				"timeout":     "5m0s",
				"chart_hooks": true,
				"on_failure":  "keep",
				"crds":        "create",
			},
			contains: []string{
				"Helm Apply Summary", "\n### Release lifecycle\n",
				"\n| Operation | `install` |\n",
				"| Wait strategy | `hookOnly` |",
				"| Timeout | `5m0s` |",
				"| Chart hooks enabled | `true` |",
				"| Wait for Jobs | `false` |",
				"| On failure | `keep` |\n" +
					"| Install CRDs | `create` |",
			},
			notContains: []string{"Maximum history"},
		},
		{
			name:    "external apply",
			command: "apply",
			lifecycle: map[string]any{
				"applied": false, "target_kind": "git", "reason": "external_target",
			},
			contains: []string{
				"Helm Apply Summary",
				"\n### Release lifecycle\n",
				"\n| Applied | `false` |\n",
				"| Target kind | `git` |",
				"| Reason | `external_target` |",
			},
			notContains: []string{"Wait strategy", "Timeout"},
		},
		{
			name:    "cluster delete",
			command: "delete",
			lifecycle: map[string]any{
				"operation":   "delete",
				"wait":        map[string]any{"strategy": "legacy"},
				"timeout":     "10m0s",
				"chart_hooks": false,
			},
			contains: []string{
				"Helm Delete Summary", "\n### Release lifecycle\n",
				"\n| Operation | `delete` |\n",
				"| Wait strategy | `legacy` |",
				"| Timeout | `10m0s` |",
				"| Chart hooks enabled | `false` |",
			},
		},
		{
			name:    "external delete",
			command: "delete",
			lifecycle: map[string]any{
				"deleted": false, "target_kind": "git", "reason": "external_target",
			},
			contains: []string{
				"Helm Delete Summary",
				"\n### Release lifecycle\n",
				"\n| Deleted | `false` |\n",
				"| Target kind | `git` |",
				"| Reason | `external_target` |",
			},
			notContains: []string{"Wait strategy", "Timeout"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := (&Plugin{}).buildTemplateContext(&plugin.HookContext{
				Command: tt.command,
				Info: &schema.ConfigAndStacksInfo{
					ComponentFromArg: "nginx",
					Stack:            "dev",
				},
				Aggregate: Summary{
					Chart:         "bitnami/nginx",
					ReleaseName:   "nginx",
					Namespace:     "apps",
					Target:        "kubernetes",
					ObjectCount:   2,
					ObjectKinds:   []string{"Deployment", "Service"},
					ManifestBytes: 1234,
					Lifecycle:     tt.lifecycle,
				},
			})

			rendered, err := templates.NewLoader(nil).LoadAndRender("helm", tt.command, defaultTemplates, ctx)
			require.NoError(t, err)
			for _, expected := range tt.contains {
				assert.Contains(t, rendered, expected)
			}
			for _, unexpected := range tt.notContains {
				assert.NotContains(t, rendered, unexpected)
			}
		})
	}
}
