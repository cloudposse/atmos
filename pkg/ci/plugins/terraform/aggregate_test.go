package terraform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	githubprovider "github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/ci/templates"
	"github.com/cloudposse/atmos/pkg/schema"
)

type nilOutputProvider struct {
	*mockProvider
}

func (p *nilOutputProvider) OutputWriter() provider.OutputWriter {
	return nil
}

type failingOutputProvider struct {
	*mockProvider
}

func (p *failingOutputProvider) OutputWriter() provider.OutputWriter {
	return failingOutputWriter{}
}

type failingOutputWriter struct{}

func (f failingOutputWriter) WriteSummary(string) error {
	return errors.New("summary failed")
}

func (f failingOutputWriter) WriteOutput(string, string) error {
	return errors.New("output failed")
}

var _ = schema.TerraformPlanCIResult{
	NodeID:     "",
	Stack:      "",
	Component:  "",
	Status:     "",
	Processed:  false,
	Changed:    false,
	ExitCode:   0,
	Output:     "",
	Error:      "",
	StartedAt:  time.Time{},
	FinishedAt: time.Time{},
	DurationMS: 0,
}

func TestOnAfterPlanAggregateRendersSummaryOutputsCommentAndChecks(t *testing.T) {
	p := &Plugin{}
	ctx := newAggregateHookContext()
	mp := ctx.Provider.(*mockProvider)

	err := p.onAfterPlanAggregate(ctx)
	require.NoError(t, err)

	require.Len(t, mp.writer.summaries, 1)
	rendered := mp.writer.summaries[0]
	assert.Contains(t, rendered, "## Terraform Plan Summary")
	assert.Contains(t, rendered, "5 components: 2 changed, 1 failed, 1 skipped")
	assert.Contains(t, rendered, "| Resource Action | Count |")
	assert.Contains(t, rendered, "| Group | Components |")
	assert.Contains(t, rendered, "| Failed | dev/app |")
	assert.Contains(t, rendered, "| Changed | dev/outputs, dev/vpc |")
	assert.Contains(t, rendered, "| No changes | dev/database |")
	assert.Contains(t, rendered, "| Skipped | dev/worker |")
	assert.Contains(t, rendered, "| dev | app | failed |")
	assert.Contains(t, rendered, "| dev | outputs | changed | Output values will change. No infrastructure changes.")
	assert.Contains(t, rendered, "### Failed Components")
	assert.Contains(t, rendered, "### Changed Components")

	outputs := mp.writer.outputs
	assert.Equal(t, "true", outputs["has_changes"])
	assert.Equal(t, "true", outputs["has_errors"])
	assert.Equal(t, "1", outputs["exit_code"])
	assert.Equal(t, "5", outputs["components_total"])
	assert.Equal(t, "3", outputs["components_succeeded"])
	assert.Equal(t, "1", outputs["components_failed"])
	assert.Equal(t, "2", outputs["components_changed"])
	assert.Equal(t, "1", outputs["components_no_changes"])
	assert.Equal(t, "1", outputs["components_skipped"])
	assert.Equal(t, "1", outputs["resources_to_create"])
	assert.Equal(t, "0", outputs["resources_to_change"])
	assert.Equal(t, "0", outputs["resources_to_replace"])
	assert.Equal(t, "0", outputs["resources_to_destroy"])
	assert.Equal(t, "plan", outputs["command"])
	assert.Equal(t, "dev", outputs["stack"])
	assert.Equal(t, "aggregate", outputs["component"])
	assert.Contains(t, outputs["summary"], "Terraform Plan Summary")

	require.Len(t, mp.commentCalls, 1)
	assert.Equal(t, "<!-- atmos:ci:plan:aggregate:dev -->", mp.commentCalls[0].Marker)
	assert.Contains(t, mp.commentCalls[0].Body, "<!-- atmos:ci:plan:aggregate:dev -->")

	require.Len(t, mp.updateRunCalls, 5)
	assert.Equal(t, "atmos/plan/dev/app", mp.updateRunCalls[0].Name)
	assert.Equal(t, "atmos/plan/dev/database", mp.updateRunCalls[1].Name)
	assert.Equal(t, "atmos/plan/dev/outputs", mp.updateRunCalls[2].Name)
	assert.Equal(t, "atmos/plan/dev/vpc", mp.updateRunCalls[3].Name)
	assert.Equal(t, "atmos/plan/dev/worker", mp.updateRunCalls[4].Name)
}

func TestBuildPlanAggregateExitCodeRules(t *testing.T) {
	p := &Plugin{}
	tests := []struct {
		name     string
		input    schema.TerraformPlanCIResultSet
		wantCode int
	}{
		{
			name: "no changes",
			input: schema.TerraformPlanCIResultSet{Results: []schema.TerraformPlanCIResult{
				{
					NodeID:    "vpc-dev",
					Stack:     "dev",
					Component: "vpc",
					Status:    "succeeded",
					Processed: true,
					Output:    "No changes. Your infrastructure matches the configuration.",
				},
			}},
			wantCode: 0,
		},
		{
			name: "changes",
			input: schema.TerraformPlanCIResultSet{Results: []schema.TerraformPlanCIResult{
				{
					NodeID:    "vpc-dev",
					Stack:     "dev",
					Component: "vpc",
					Status:    "succeeded",
					Processed: true,
					Changed:   true,
					ExitCode:  2,
					Output:    "Plan: 1 to add, 0 to change, 0 to destroy.",
				},
			}},
			wantCode: 2,
		},
		{
			name: "failed dominates changes",
			input: schema.TerraformPlanCIResultSet{Results: []schema.TerraformPlanCIResult{
				{
					NodeID:    "vpc-dev",
					Stack:     "dev",
					Component: "vpc",
					Status:    "failed",
					Processed: true,
					ExitCode:  1,
					Output:    "Error: invalid value",
					Error:     "terraform failed",
				},
				{
					NodeID:    "database-dev",
					Stack:     "dev",
					Component: "database",
					Status:    "succeeded",
					Processed: true,
					Changed:   true,
					ExitCode:  2,
					Output:    "Plan: 1 to add, 0 to change, 0 to destroy.",
				},
			}},
			wantCode: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.buildPlanAggregate(tt.input)
			assert.Equal(t, tt.wantCode, got.ExitCode)
		})
	}
}

func TestOnAfterPlanAggregateWritesGitHubOutputFiles(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "github-output")
	summaryFile := filepath.Join(tmpDir, "github-step-summary")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_OUTPUT", outputFile)
	t.Setenv("GITHUB_STEP_SUMMARY", summaryFile)

	p := &Plugin{}
	ctx := newAggregateHookContext()
	ctx.Config = &schema.AtmosConfiguration{
		CI: schema.CIConfig{
			Summary: schema.CISummaryConfig{Enabled: boolPtr(true)},
			Output:  schema.CIOutputConfig{Enabled: boolPtr(true)},
		},
	}
	ctx.Provider = githubprovider.NewProvider()

	err := p.onAfterPlanAggregate(ctx)
	require.NoError(t, err)

	outputData, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Contains(t, string(outputData), "has_changes=true")
	assert.Contains(t, string(outputData), "exit_code=1")
	assert.Contains(t, string(outputData), "summary<<EOF")

	summaryData, err := os.ReadFile(summaryFile)
	require.NoError(t, err)
	assert.Contains(t, string(summaryData), "## Terraform Plan Summary")
	assert.Contains(t, string(summaryData), "5 components: 2 changed, 1 failed, 1 skipped")
}

func newAggregateHookContext() *plugin.HookContext {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	return &plugin.HookContext{
		Event:          "after.terraform.plan.aggregate",
		Command:        "plan",
		EventPrefix:    "after",
		Config:         newAggregateTestConfig(),
		Provider:       newMockProvider(),
		TemplateLoader: templates.NewLoader(&schema.AtmosConfiguration{}),
		CICtx: &provider.Context{
			Provider:  "github-actions",
			RepoOwner: "cloudposse",
			RepoName:  "atmos",
			SHA:       "abc123",
			PullRequest: &provider.PRInfo{
				Number: 2467,
			},
		},
		Info: &schema.ConfigAndStacksInfo{
			Stack: "dev",
		},
		Aggregate: schema.TerraformPlanCIResultSet{Results: []schema.TerraformPlanCIResult{
			{
				NodeID:     "vpc-dev",
				Stack:      "dev",
				Component:  "vpc",
				Status:     "succeeded",
				Processed:  true,
				Changed:    true,
				ExitCode:   2,
				Output:     "Plan: 1 to add, 0 to change, 0 to destroy.",
				StartedAt:  now,
				FinishedAt: now.Add(2 * time.Second),
				DurationMS: 2000,
			},
			{
				NodeID:     "database-dev",
				Stack:      "dev",
				Component:  "database",
				Status:     "succeeded",
				Processed:  true,
				Output:     "No changes. Your infrastructure matches the configuration.",
				StartedAt:  now,
				FinishedAt: now.Add(time.Second),
				DurationMS: 1000,
			},
			{
				NodeID:    "outputs-dev",
				Stack:     "dev",
				Component: "outputs",
				Status:    "succeeded",
				Processed: true,
				Changed:   true,
				ExitCode:  2,
				Output: `Changes to Outputs:
  + endpoint = "https://example.test"

You can apply this plan to save these new output values to the Terraform state, without changing any real infrastructure.`,
			},
			{
				NodeID:    "app-dev",
				Stack:     "dev",
				Component: "app",
				Status:    "failed",
				Processed: true,
				ExitCode:  1,
				Output:    "Error: invalid reference",
				Error:     "terraform plan failed",
			},
			{
				NodeID:    "worker-dev",
				Stack:     "dev",
				Component: "worker",
				Status:    "skipped",
				Error:     "dependency app-dev failed",
			},
		}},
	}
}

func newAggregateTestConfig() *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		CI: schema.CIConfig{
			Summary: schema.CISummaryConfig{
				Enabled: boolPtr(true),
			},
			Output: schema.CIOutputConfig{
				Enabled: boolPtr(true),
			},
			Checks: schema.CIChecksConfig{
				Enabled: boolPtr(true),
			},
			Comments: schema.CICommentsConfig{
				Enabled:  boolPtr(true),
				Behavior: "update",
			},
		},
	}
}

func TestOnAfterPlanAggregateSkipsInvalidAggregate(t *testing.T) {
	p := &Plugin{}
	ctx := newAggregateHookContext()
	ctx.Aggregate = errors.New("not a result set")
	mp := ctx.Provider.(*mockProvider)

	err := p.onAfterPlanAggregate(ctx)
	require.NoError(t, err)
	assert.Empty(t, mp.writer.summaries)
	assert.Empty(t, mp.writer.outputs)
	assert.Empty(t, mp.commentCalls)
	assert.Empty(t, mp.updateRunCalls)
}

func TestOnAfterPlanAggregateWriterErrorsAreWarnOnly(t *testing.T) {
	p := &Plugin{}
	ctx := newAggregateHookContext()
	ctx.Provider = &failingOutputProvider{mockProvider: newMockProvider()}
	ctx.Config.CI.Checks.Enabled = boolPtr(false)
	ctx.Config.CI.Comments.Enabled = boolPtr(false)

	err := p.onAfterPlanAggregate(ctx)
	require.NoError(t, err)
}

func TestOnAfterPlanAggregateReturnsPlanfileUploadError(t *testing.T) {
	p := &Plugin{}
	ctx := newAggregateHookContext()
	planfilePath := filepath.Join(t.TempDir(), "plan.tfplan")
	require.NoError(t, os.WriteFile(planfilePath, []byte("plan"), 0o644))
	ctx.Info.PlanFile = planfilePath
	ctx.Config.CI.Summary.Enabled = boolPtr(false)
	ctx.Config.CI.Output.Enabled = boolPtr(false)
	ctx.Config.CI.Checks.Enabled = boolPtr(false)
	ctx.Config.CI.Comments.Enabled = boolPtr(false)
	ctx.Config.Components.Terraform.Planfiles.Default = "local"
	ctx.Config.Components.Terraform.Planfiles.Stores = map[string]schema.PlanfileStoreSpec{
		"local": {Type: "local/dir"},
	}
	ctx.CreatePlanfileStore = func() (any, error) {
		return nil, errors.New("store unavailable")
	}

	err := p.onAfterPlanAggregate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store unavailable")
}

func TestAggregateHelpersCoverFallbacks(t *testing.T) {
	p := &Plugin{}

	nilResultSet, ok := normalizeTerraformPlanAggregate((*schema.TerraformPlanCIResultSet)(nil))
	assert.False(t, ok)
	assert.Empty(t, nilResultSet.Results)

	resultSet := &schema.TerraformPlanCIResultSet{Results: []schema.TerraformPlanCIResult{
		{NodeID: "b", Stack: "dev", Component: "database", Status: "succeeded", Processed: true},
		{NodeID: "a", Stack: "dev", Component: "vpc", Status: "succeeded", Processed: true, Changed: true},
	}}
	normalized, ok := normalizeTerraformPlanAggregate(resultSet)
	require.True(t, ok)
	require.Len(t, normalized.Results, 2)

	aggregate := p.buildPlanAggregate(*resultSet)
	require.Len(t, aggregate.Components, 2)
	assert.Equal(t, "database", aggregate.Components[0].Result.Component, "components are sorted by stack/component/node")

	assert.Equal(t, "provider error", componentSummaryText(
		&schema.TerraformPlanCIResult{},
		&plugin.OutputResult{Errors: []string{"provider error"}},
		nil,
		aggregateStatusFailed,
	))
	assert.Equal(t, aggregateStatusFailed, componentSummaryText(&schema.TerraformPlanCIResult{}, nil, nil, aggregateStatusFailed))
	assert.Equal(t, aggregateStatusSkipped, componentSummaryText(&schema.TerraformPlanCIResult{}, nil, nil, aggregateStatusSkipped))
	assert.Equal(t, "Changes detected", componentSummaryText(
		&schema.TerraformPlanCIResult{},
		&plugin.OutputResult{HasChanges: true},
		nil,
		aggregateStatusChanged,
	))

	assert.Empty(t, truncateAggregateDetail(""))
	longValue := strings.Repeat("x", aggregateDetailOutputMaxBytes+4)
	assert.True(t, strings.HasPrefix(truncateAggregateDetail(longValue), "... output truncated ..."))
	line := `resource "google_cloud_run_v2_job" "run_job" {`
	lineAlignedValue := strings.Repeat(line+"\n", aggregateDetailOutputMaxBytes/len(line)+10)
	lineAlignedTail := strings.TrimPrefix(truncateAggregateDetail(lineAlignedValue), "... output truncated ...\n")
	firstLine, _, _ := strings.Cut(lineAlignedTail, "\n")
	assert.Equal(t, line, firstLine)

	assert.Equal(t, plugin.ResourceCounts{}, resourceCounts(nil))
	assert.Equal(t, plugin.ResourceCounts{Replace: 2}, resourceCounts(&plugin.TerraformOutputData{
		ReplacedResources: []string{"aws_instance.a", "aws_instance.b"},
	}))

	assert.Equal(t, "-", markdownTableCell(" \n "))
	assert.Equal(t, "a \\| b", markdownTableCell("a | b"))
	assert.Equal(t, "-", markdownInline("\n"))

	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, "1500ms", formatAggregateDuration(&schema.TerraformPlanCIResult{
		StartedAt:  now,
		FinishedAt: now.Add(1500 * time.Millisecond),
	}))
	assert.Equal(t, "-", formatAggregateDuration(&schema.TerraformPlanCIResult{}))

	assert.Equal(t, "all", aggregateStackValue(nil))
	assert.Equal(t, "all", aggregateStackValue(&schema.ConfigAndStacksInfo{}))
	assert.Equal(t, "<!-- atmos:ci:plan:aggregate:all -->", buildAggregateCommentMarker("plan", ""))
}

func TestAggregateSummaryAndOutputsHandleProviderWithoutWriter(t *testing.T) {
	p := &Plugin{}
	ctx := newAggregateHookContext()
	ctx.Provider = &nilOutputProvider{mockProvider: newMockProvider()}
	aggregate := p.buildPlanAggregate(ctx.Aggregate.(schema.TerraformPlanCIResultSet))

	require.NoError(t, p.writeAggregateSummary(ctx, aggregate.Markdown))
	require.NoError(t, p.writeAggregateOutputs(ctx, &aggregate))
}

func TestWriteAggregateOutputsFiltersVariables(t *testing.T) {
	p := &Plugin{}
	ctx := newAggregateHookContext()
	ctx.Config.CI.Output.Variables = []string{"has_changes", "exit_code"}
	mp := ctx.Provider.(*mockProvider)
	aggregate := p.buildPlanAggregate(ctx.Aggregate.(schema.TerraformPlanCIResultSet))

	require.NoError(t, p.writeAggregateOutputs(ctx, &aggregate))
	assert.Equal(t, map[string]string{
		"has_changes": "true",
		"exit_code":   "1",
	}, mp.writer.outputs)
}

func TestWriteAggregateOutputsReturnsJoinedWriterErrors(t *testing.T) {
	p := &Plugin{}
	ctx := newAggregateHookContext()
	ctx.Provider = &failingOutputProvider{mockProvider: newMockProvider()}
	ctx.Config.CI.Output.Variables = []string{"has_changes", "exit_code"}
	aggregate := p.buildPlanAggregate(ctx.Aggregate.(schema.TerraformPlanCIResultSet))

	err := p.writeAggregateOutputs(ctx, &aggregate)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `failed to write aggregate CI output "exit_code"`)
	assert.Contains(t, err.Error(), `failed to write aggregate CI output "has_changes"`)
}

func TestUploadAggregatePlanfilesSkipsFailedAndReturnsDelegateError(t *testing.T) {
	p := &Plugin{}
	ctx := newAggregateHookContext()
	planfilePath := filepath.Join(t.TempDir(), "plan.tfplan")
	require.NoError(t, os.WriteFile(planfilePath, []byte("plan"), 0o644))
	ctx.Info.PlanFile = planfilePath
	ctx.CreatePlanfileStore = func() (any, error) {
		return nil, errors.New("store unavailable")
	}

	aggregate := terraformPlanAggregate{
		Components: []terraformPlanAggregateComponent{
			{
				Result: schema.TerraformPlanCIResult{
					Stack:     "dev",
					Component: "skipped",
					Status:    "skipped",
				},
				Skipped: true,
			},
			{
				Result: schema.TerraformPlanCIResult{
					Stack:     "dev",
					Component: "failed",
					Status:    "failed",
				},
				HasErrors: true,
			},
			{
				Result: schema.TerraformPlanCIResult{
					Stack:     "dev",
					Component: "vpc",
					Status:    "changed",
				},
				HasChanges: true,
			},
		},
	}

	err := p.uploadAggregatePlanfiles(ctx, &aggregate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store unavailable")
}

func TestPostAggregateCommentSkipsAndReturnsErrors(t *testing.T) {
	p := &Plugin{}

	t.Run("skips without PR context", func(t *testing.T) {
		ctx := newAggregateHookContext()
		ctx.CICtx.PullRequest = nil
		mp := ctx.Provider.(*mockProvider)

		require.NoError(t, p.postAggregateComment(ctx, "summary"))
		assert.Empty(t, mp.commentCalls)
	})

	t.Run("invalid behavior returns error", func(t *testing.T) {
		ctx := newAggregateHookContext()
		ctx.Config.CI.Comments.Behavior = "garbage"

		err := p.postAggregateComment(ctx, "summary")
		require.Error(t, err)
	})

	t.Run("provider error is returned", func(t *testing.T) {
		ctx := newAggregateHookContext()
		mp := ctx.Provider.(*mockProvider)
		mp.commentErr = errors.New("api error")

		err := p.postAggregateComment(ctx, "summary")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "api error")
		assert.Len(t, mp.commentCalls, 1)
	})
}
