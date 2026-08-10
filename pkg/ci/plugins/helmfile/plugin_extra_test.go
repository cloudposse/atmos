package helmfile

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

	// Summaries disabled -> no-op, nothing written.
	disabled := false
	writer := &fakeWriter{}
	require.NoError(t, p.onAfterOperation(&plugin.HookContext{
		Config:         &schema.AtmosConfiguration{CI: schema.CIConfig{Summary: schema.CISummaryConfig{Enabled: &disabled}}},
		Provider:       fakeProvider{writer: writer},
		TemplateLoader: templates.NewLoader(nil),
		Command:        "apply",
	}))
	assert.Empty(t, writer.summary)

	// Nil output writer -> no-op.
	require.NoError(t, p.onAfterOperation(&plugin.HookContext{
		Provider:       fakeProvider{},
		TemplateLoader: templates.NewLoader(nil),
		Command:        "apply",
	}))

	// An empty command resolves to an empty template name -> no-op.
	writer = &fakeWriter{}
	require.NoError(t, p.onAfterOperation(&plugin.HookContext{
		Provider:       fakeProvider{writer: writer},
		TemplateLoader: templates.NewLoader(nil),
		Command:        "",
	}))
	assert.Empty(t, writer.summary)

	// Enabled with a known command renders and writes the summary.
	writer = &fakeWriter{}
	require.NoError(t, p.onAfterOperation(&plugin.HookContext{
		Provider:       fakeProvider{writer: writer},
		TemplateLoader: templates.NewLoader(nil),
		Command:        "apply",
		Output:         "UPDATED RELEASES:\nname: echo-server",
		Info:           &schema.ConfigAndStacksInfo{ComponentFromArg: "echo-server", Stack: "dev"},
	}))
	assert.Contains(t, writer.summary, "Helmfile Apply Summary")

	// An explicit summary template name overrides the command default.
	writer = &fakeWriter{}
	require.NoError(t, p.onAfterOperation(&plugin.HookContext{
		Config:         &schema.AtmosConfiguration{CI: schema.CIConfig{Summary: schema.CISummaryConfig{Template: "diff"}}},
		Provider:       fakeProvider{writer: writer},
		TemplateLoader: templates.NewLoader(nil),
		Command:        "apply",
		Info:           &schema.ConfigAndStacksInfo{ComponentFromArg: "echo-server", Stack: "dev"},
	}))
	assert.NotEmpty(t, writer.summary)

	// A write failure propagates.
	sentinel := errors.New("write failed")
	require.ErrorIs(t, p.onAfterOperation(&plugin.HookContext{
		Provider:       fakeProvider{writer: &fakeWriter{err: sentinel}},
		TemplateLoader: templates.NewLoader(nil),
		Command:        "apply",
	}), sentinel)
}

func TestIsSummaryEnabledAndTitle(t *testing.T) {
	assert.True(t, isSummaryEnabled(nil))
	assert.True(t, isSummaryEnabled(&schema.AtmosConfiguration{}))

	disabled := false
	assert.False(t, isSummaryEnabled(&schema.AtmosConfiguration{
		CI: schema.CIConfig{Summary: schema.CISummaryConfig{Enabled: &disabled}},
	}))

	assert.Equal(t, "Helmfile", title(""))
	assert.Equal(t, "Apply", title("apply"))
}
