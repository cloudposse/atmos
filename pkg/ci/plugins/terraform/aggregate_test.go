package terraform

import (
	"errors"
	"os"
	"path/filepath"
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

	noChanges := p.buildPlanAggregate(schema.TerraformPlanCIResultSet{Results: []schema.TerraformPlanCIResult{
		{
			NodeID:    "vpc-dev",
			Stack:     "dev",
			Component: "vpc",
			Status:    "succeeded",
			Processed: true,
			Output:    "No changes. Your infrastructure matches the configuration.",
		},
	}})
	assert.Equal(t, 0, noChanges.ExitCode)

	changes := p.buildPlanAggregate(schema.TerraformPlanCIResultSet{Results: []schema.TerraformPlanCIResult{
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
	}})
	assert.Equal(t, 2, changes.ExitCode)

	failed := p.buildPlanAggregate(schema.TerraformPlanCIResultSet{Results: []schema.TerraformPlanCIResult{
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
	}})
	assert.Equal(t, 1, failed.ExitCode)
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
		EventPrefix:    "terraform",
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
