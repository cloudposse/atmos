package cloudformation

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/ci/templates"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fakeProvider is a minimal provider.Provider stub used to exercise
// onAfterOperation without a real CI platform.
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

// fakeWriter is a provider.OutputWriter stub that captures the last summary
// written (or returns a configured error) without touching real CI output.
type fakeWriter struct {
	summary string
	err     error
}

func (f *fakeWriter) WriteOutput(string, string) error { return nil }
func (f *fakeWriter) WriteSummary(content string) error {
	f.summary = content
	return f.err
}

func TestPlugin_GetType(t *testing.T) {
	assert.Equal(t, "aws/cloudformation", (&Plugin{}).GetType())
}

func TestPlugin_GetHookBindings(t *testing.T) {
	bindings := (&Plugin{}).GetHookBindings()
	require.Len(t, bindings, 5)

	for _, event := range []string{
		"after.aws/cloudformation.diff",
		"after.aws/cloudformation.apply",
		"after.aws/cloudformation.delete",
		"after.aws/cloudformation.drift-detect",
		"after.aws/cloudformation.drift-describe",
	} {
		t.Run(event, func(t *testing.T) {
			binding := plugin.HookBindings(bindings).GetBindingForEvent(event)
			require.NotNil(t, binding)
			assert.NotNil(t, binding.Handler)
		})
	}
}

func TestPlugin_BuildTemplateContext(t *testing.T) {
	ctx := (&Plugin{}).buildTemplateContext(&plugin.HookContext{
		Command:      "diff",
		Output:       "cloudformation diff output\n",
		CommandError: errors.New("diff failed"),
		ExitCode:     1,
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "tenant1-ue2-dev",
		},
		Aggregate: &schema.CloudFormationCIResult{
			Stack:           "tenant1-ue2-dev",
			Component:       "vpc",
			Command:         "diff",
			StackName:       "tenant1-ue2-dev-vpc",
			ResourceChanges: 3,
		},
	})

	assert.Equal(t, "vpc", ctx.Component)
	assert.Equal(t, "tenant1-ue2-dev", ctx.Stack)
	assert.Equal(t, "diff", ctx.Command)
	assert.Equal(t, "cloudformation diff output", ctx.Output)
	assert.Equal(t, "tenant1-ue2-dev-vpc", ctx.StackName)
	assert.Equal(t, 3, ctx.ResourceChanges)
	require.True(t, ctx.Result.HasErrors)
	assert.Equal(t, []string{"diff failed"}, ctx.Result.Errors)
}

func TestPlugin_BuildTemplateContext_FallsBackToHookContextInfo(t *testing.T) {
	// No Aggregate provided (or the wrong type) — Component/Stack must still
	// come from ctx.Info rather than panicking or leaving them empty.
	ctx := (&Plugin{}).buildTemplateContext(&plugin.HookContext{
		Command: "apply",
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "tenant1-ue2-dev",
		},
	})

	assert.Equal(t, "vpc", ctx.Component)
	assert.Equal(t, "tenant1-ue2-dev", ctx.Stack)
	assert.Empty(t, ctx.StackName)
	assert.False(t, ctx.Result.HasErrors)
}

func TestPlugin_BuildTemplateContext_AggregateErrorFallback(t *testing.T) {
	// When there's no CommandError but the aggregate result carries its own
	// Error (e.g. a failed drift-detect API call surfaced by the operation
	// handler), that error must still populate Result.Errors.
	ctx := (&Plugin{}).buildTemplateContext(&plugin.HookContext{
		Command: "drift-detect",
		Aggregate: &schema.CloudFormationCIResult{
			StackName: "tenant1-ue2-dev-vpc",
			Error:     "AccessDenied: not authorized to perform cloudformation:DetectStackDrift",
		},
	})

	require.True(t, ctx.Result.HasErrors)
	assert.Equal(t, []string{"AccessDenied: not authorized to perform cloudformation:DetectStackDrift"}, ctx.Result.Errors)
}

func TestPlugin_BuildTemplateContext_DriftFields(t *testing.T) {
	ctx := (&Plugin{}).buildTemplateContext(&plugin.HookContext{
		Command: "drift-detect",
		Aggregate: &schema.CloudFormationCIResult{
			StackName:    "tenant1-ue2-dev-vpc",
			DriftStatus:  "DRIFTED",
			DriftedCount: 2,
		},
	})

	assert.Equal(t, "DRIFTED", ctx.DriftStatus)
	assert.Equal(t, 2, ctx.DriftedCount)
}

func TestAggregateResult_HandlesValuePointerAndMissing(t *testing.T) {
	result := aggregateResult(&plugin.HookContext{Aggregate: &schema.CloudFormationCIResult{StackName: "s1"}})
	assert.Equal(t, "s1", result.StackName)

	result = aggregateResult(&plugin.HookContext{Aggregate: schema.CloudFormationCIResult{StackName: "s2"}})
	assert.Equal(t, "s2", result.StackName)

	result = aggregateResult(&plugin.HookContext{Aggregate: nil})
	assert.Equal(t, &schema.CloudFormationCIResult{}, result)

	result = aggregateResult(&plugin.HookContext{Aggregate: "not-a-result"})
	assert.Equal(t, &schema.CloudFormationCIResult{}, result)
}

func TestIsSummaryEnabled(t *testing.T) {
	assert.True(t, isSummaryEnabled(nil))
	assert.True(t, isSummaryEnabled(&schema.AtmosConfiguration{}))

	enabled := true
	assert.True(t, isSummaryEnabled(&schema.AtmosConfiguration{CI: schema.CIConfig{Summary: schema.CISummaryConfig{Enabled: &enabled}}}))

	disabled := false
	assert.False(t, isSummaryEnabled(&schema.AtmosConfiguration{CI: schema.CIConfig{Summary: schema.CISummaryConfig{Enabled: &disabled}}}))
}

func TestTemplateRendering_Diff(t *testing.T) {
	ctx := (&Plugin{}).buildTemplateContext(&plugin.HookContext{
		Command: "diff",
		Output:  "3 resource change(s)",
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "tenant1-ue2-dev",
		},
		Aggregate: &schema.CloudFormationCIResult{
			StackName:       "tenant1-ue2-dev-vpc",
			ResourceChanges: 3,
		},
	})

	rendered, err := templates.NewLoader(nil).LoadAndRender("cloudformation", "diff", defaultTemplates, ctx)
	require.NoError(t, err)
	assert.Contains(t, rendered, "CloudFormation Diff Summary")
	assert.Contains(t, rendered, "atmos aws/cloudformation diff vpc -s tenant1-ue2-dev")
	assert.Contains(t, rendered, "Resource changes: **3**")
}

func TestTemplateRendering_DriftDetectFailed(t *testing.T) {
	ctx := (&Plugin{}).buildTemplateContext(&plugin.HookContext{
		Command:      "drift-detect",
		CommandError: errors.New("drift check failed"),
		ExitCode:     1,
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "tenant1-ue2-dev",
		},
		Aggregate: &schema.CloudFormationCIResult{
			StackName:    "tenant1-ue2-dev-vpc",
			DriftStatus:  "DRIFTED",
			DriftedCount: 2,
		},
	})

	rendered, err := templates.NewLoader(nil).LoadAndRender("cloudformation", "drift-detect", defaultTemplates, ctx)
	require.NoError(t, err)
	assert.Contains(t, rendered, "CloudFormation Drift Detect Failed")
	assert.Contains(t, rendered, "Drift status: **DRIFTED**")
	assert.Contains(t, rendered, "Drifted resources: **2**")
	assert.Contains(t, rendered, "drift check failed")
}

func TestPlugin_OnAfterOperation_SummaryDisabled(t *testing.T) {
	// When ci.summary.enabled is explicitly false, no template is rendered
	// and the writer is never invoked.
	disabled := false
	writer := &fakeWriter{}

	err := (&Plugin{}).onAfterOperation(&plugin.HookContext{
		Config:         &schema.AtmosConfiguration{CI: schema.CIConfig{Summary: schema.CISummaryConfig{Enabled: &disabled}}},
		Provider:       fakeProvider{writer: writer},
		TemplateLoader: templates.NewLoader(nil),
		Command:        "apply",
	})

	require.NoError(t, err)
	assert.Empty(t, writer.summary)
}

func TestPlugin_OnAfterOperation_NoOutputWriter(t *testing.T) {
	// A CI provider that doesn't support summaries (OutputWriter returns
	// nil) must be a silent no-op, not a nil-pointer panic.
	err := (&Plugin{}).onAfterOperation(&plugin.HookContext{
		Provider:       fakeProvider{},
		TemplateLoader: templates.NewLoader(nil),
		Command:        "apply",
	})

	require.NoError(t, err)
}

func TestPlugin_OnAfterOperation_EmptyTemplateName(t *testing.T) {
	// No command and no configured summary template resolves to an empty
	// template name, which must skip rendering entirely without an error.
	writer := &fakeWriter{}

	err := (&Plugin{}).onAfterOperation(&plugin.HookContext{
		Provider:       fakeProvider{writer: writer},
		TemplateLoader: templates.NewLoader(nil),
		Command:        "",
	})

	require.NoError(t, err)
	assert.Empty(t, writer.summary)
}

func TestPlugin_OnAfterOperation_RendersAndWritesSummary(t *testing.T) {
	// The success path: a real diff summary is rendered from the embedded
	// template and handed to the CI provider's writer verbatim.
	writer := &fakeWriter{}

	err := (&Plugin{}).onAfterOperation(&plugin.HookContext{
		Provider:       fakeProvider{writer: writer},
		TemplateLoader: templates.NewLoader(nil),
		Command:        "diff",
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "tenant1-ue2-dev",
		},
		Aggregate: &schema.CloudFormationCIResult{
			StackName:       "tenant1-ue2-dev-vpc",
			ResourceChanges: 3,
		},
	})

	require.NoError(t, err)
	assert.Contains(t, writer.summary, "CloudFormation Diff Summary")
	assert.Contains(t, writer.summary, "Resource changes: **3**")
}

func TestPlugin_OnAfterOperation_UsesConfiguredTemplateOverride(t *testing.T) {
	// ci.summary.template overrides the command-derived template name.
	writer := &fakeWriter{}

	err := (&Plugin{}).onAfterOperation(&plugin.HookContext{
		Config:         &schema.AtmosConfiguration{CI: schema.CIConfig{Summary: schema.CISummaryConfig{Template: "delete"}}},
		Provider:       fakeProvider{writer: writer},
		TemplateLoader: templates.NewLoader(nil),
		Command:        "diff",
	})

	require.NoError(t, err)
	assert.Contains(t, writer.summary, "CloudFormation Delete Summary")
}

func TestPlugin_OnAfterOperation_RenderErrorWrapsSentinel(t *testing.T) {
	// An unknown template name (no embedded template) must surface as
	// errUtils.ErrTemplateEvaluation, and the writer must never be called.
	writer := &fakeWriter{}

	err := (&Plugin{}).onAfterOperation(&plugin.HookContext{
		Provider:       fakeProvider{writer: writer},
		TemplateLoader: templates.NewLoader(nil),
		Command:        "does-not-exist",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	assert.Empty(t, writer.summary)
}

func TestPlugin_OnAfterOperation_WriteSummaryErrorWrapsSentinel(t *testing.T) {
	// A writer failure (e.g. CI platform API error) must surface as
	// errUtils.ErrCISummaryWriteFailed with the underlying cause preserved.
	sentinel := errors.New("write failed")

	err := (&Plugin{}).onAfterOperation(&plugin.HookContext{
		Provider:       fakeProvider{writer: &fakeWriter{err: sentinel}},
		TemplateLoader: templates.NewLoader(nil),
		Command:        "diff",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCISummaryWriteFailed)
	assert.ErrorIs(t, err, sentinel)
}
