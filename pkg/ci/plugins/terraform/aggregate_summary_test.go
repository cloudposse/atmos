package terraform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/templates"
	metricsprocess "github.com/cloudposse/atmos/pkg/metrics/process"
	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.ConfigAndStacksInfo{ExecMetadataRawMetrics: nil}

func TestAggregatePlanSummaryIncludesUnchangedComponent(t *testing.T) {
	ctx := newAggregateHookContext()
	ctx.Aggregate = schema.TerraformPlanCIResultSet{Results: []schema.TerraformPlanCIResult{
		{
			NodeID: "test-ue1-ncsadev", Stack: "ue1-ncsadev", Component: "test",
			Status: "succeeded", Processed: true,
			Output: "No changes. Your infrastructure matches the configuration.", DurationMS: 58473,
		},
	}}

	require.NoError(t, (&Plugin{}).onAfterTerraformAggregate(ctx))
	mp := ctx.Provider.(*mockProvider)
	require.Len(t, mp.writer.summaries, 1)
	summary := mp.writer.summaries[0]
	assert.Contains(t, summary, "## Terraform Plan Summary")
	assert.Contains(t, summary, "1 components: 0 changed, 0 failed, 0 skipped")
	assert.Contains(t, summary, "## No Changes for `test` in `ue1-ncsadev`")
	assert.Contains(t, summary, "atmos terraform plan test -s ue1-ncsadev")
	assert.Contains(t, summary, "NO_CHANGE")
	assert.Contains(t, summary, "| 58.5s |")
	assert.Equal(t, summary, mp.writer.outputs["summary"])
	require.Len(t, mp.commentCalls, 1)
	assert.Contains(t, mp.commentCalls[0].Body, summary)
}

func TestAggregatePlanSummaryMatchesStandaloneTemplates(t *testing.T) {
	p := &Plugin{}
	ctx := newAggregateHookContext()
	require.NoError(t, p.onAfterTerraformAggregate(ctx))
	mp := ctx.Provider.(*mockProvider)
	require.Len(t, mp.writer.summaries, 1)
	summary := mp.writer.summaries[0]
	assert.NotContains(t, summary, "### Failed Components")
	assert.NotContains(t, summary, "### Changed Components")
	assert.Contains(t, summary, "| dev | worker | skipped | dependency app-dev failed |")
	assert.NotContains(t, summary, "atmos terraform plan worker")

	lastIndex := -1
	for _, name := range []string{"app", "database", "outputs", "vpc"} {
		for _, result := range ctx.Aggregate.(schema.TerraformPlanCIResultSet).Results {
			if result.Component != name {
				continue
			}
			standalone := newAggregateHookContext()
			standalone.Info = &schema.ConfigAndStacksInfo{Stack: result.Stack, ComponentFromArg: result.Component}
			standalone.Output = result.Output
			standalone.ExitCode = result.ExitCode
			standalone.CommandError = aggregateCommandError(result.Error)
			rendered, err := p.writeSummary(standalone, p.parseOutputWithError(standalone))
			require.NoError(t, err)
			require.NotEmpty(t, rendered)
			assert.Equal(t, 1, strings.Count(summary, rendered), name)
			index := strings.Index(summary, rendered)
			assert.Greater(t, index, lastIndex, name)
			lastIndex = index
		}
	}
}

func TestAggregatePlanSummaryCustomTemplates(t *testing.T) {
	for _, mode := range []string{"explicit", "named", "convention"} {
		t.Run(mode, func(t *testing.T) {
			ctx := newAggregateHookContext()
			templateName := "plan"
			if mode == "named" {
				templateName = "custom"
				ctx.Config.CI.Summary.Template = templateName
			}
			dir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "terraform"), 0o755))
			path := filepath.Join(dir, "terraform", templateName+".md")
			require.NoError(t, os.WriteFile(path, []byte("CUSTOM {{.Stack}}/{{.Component}} {{.Command}} {{.Result.HasErrors}}"), 0o600))
			if mode == "convention" {
				ctx.Config.CI.Templates.BasePath = dir
			} else {
				ctx.Config.CI.Templates.Terraform = map[string]string{templateName: path}
			}
			ctx.TemplateLoader = templates.NewLoader(ctx.Config)
			require.NoError(t, (&Plugin{}).onAfterTerraformAggregate(ctx))
			mp := ctx.Provider.(*mockProvider)
			require.Len(t, mp.writer.summaries, 1)
			summary := mp.writer.summaries[0]
			assert.Contains(t, summary, "## Terraform Plan Summary")
			assert.Contains(t, summary, "CUSTOM dev/app plan true")
			assert.Contains(t, summary, "CUSTOM dev/database plan false")
			assert.Equal(t, 4, strings.Count(summary, "CUSTOM "))
			assert.NotContains(t, summary, "CUSTOM dev/worker")
		})
	}
}

func TestAggregatePlanSummaryTemplateFailurePreservesOtherComponents(t *testing.T) {
	ctx := newAggregateHookContext()
	setAggregatePlanTemplate(t, ctx, "{{if eq .Component `app`}}{{.MissingField}}{{else}}OK {{.Stack}}/{{.Component}}{{end}}")
	require.NoError(t, (&Plugin{}).onAfterTerraformAggregate(ctx))
	mp := ctx.Provider.(*mockProvider)
	require.Len(t, mp.writer.summaries, 1)
	summary := mp.writer.summaries[0]
	assert.Contains(t, summary, "### dev/app")
	assert.Contains(t, summary, "Status: failed — terraform plan failed")
	assert.Contains(t, summary, "Component summary template failed")
	assert.Contains(t, summary, "OK dev/database")
	assert.Contains(t, summary, "OK dev/vpc")
	assert.Equal(t, "1", mp.writer.outputs["exit_code"])
	require.Len(t, mp.updateRunCalls, 5)
}

func TestAggregatePlanSummaryDoesNotInheritParentMetrics(t *testing.T) {
	ctx := newAggregateHookContext()
	metrics := &metricsprocess.ProcessMetrics{WallTime: time.Second}
	ctx.Info.ExecMetadataRawMetrics = metrics
	require.NoError(t, (&Plugin{}).onAfterTerraformAggregate(ctx))
	mp := ctx.Provider.(*mockProvider)
	require.Len(t, mp.writer.summaries, 1)
	assert.NotContains(t, mp.writer.summaries[0], "Resource usage:")
	assert.Same(t, metrics, ctx.Info.ExecMetadataRawMetrics)

	// Single-component rendering must still show its own metrics.
	rendered, err := (&Plugin{}).writeSummary(ctx, (&Plugin{}).parseOutputWithError(ctx))
	require.NoError(t, err)
	assert.Contains(t, rendered, "Resource usage:")
}

func TestAggregatePlanSummaryRespectsFeatureSwitches(t *testing.T) {
	ctx := newAggregateHookContext()
	ctx.Config.CI.Summary.Enabled = boolPtr(false)
	require.NoError(t, (&Plugin{}).onAfterTerraformAggregate(ctx))
	mp := ctx.Provider.(*mockProvider)
	assert.Empty(t, mp.writer.summaries)
	assert.Contains(t, mp.writer.outputs["summary"], "## No Changes for `database`")
	require.Len(t, mp.commentCalls, 1)
	assert.Contains(t, mp.commentCalls[0].Body, "## No Changes for `database`")

	ctx = newAggregateHookContext()
	ctx.Config.CI.Output.Enabled = boolPtr(false)
	ctx.Config.CI.Comments.Enabled = boolPtr(false)
	require.NoError(t, (&Plugin{}).onAfterTerraformAggregate(ctx))
	mp = ctx.Provider.(*mockProvider)
	require.Len(t, mp.writer.summaries, 1)
	assert.Empty(t, mp.writer.outputs)
	assert.Empty(t, mp.commentCalls)
}

func TestAggregatePlanSummarySkipsWholeOversizedSections(t *testing.T) {
	ctx := newAggregateHookContext()
	setAggregatePlanTemplate(t, ctx, "BEGIN {{.Component}}\n{{.Output}}\nEND {{.Component}}")
	ctx.Aggregate = schema.TerraformPlanCIResultSet{Results: []schema.TerraformPlanCIResult{
		{Stack: "dev", Component: "large", Processed: true, Output: strings.Repeat("x", aggregateMarkdownMaxBytes)},
		{Stack: "dev", Component: "small", Processed: true, Output: "small output"},
	}}
	require.NoError(t, (&Plugin{}).onAfterTerraformAggregate(ctx))
	mp := ctx.Provider.(*mockProvider)
	require.Len(t, mp.writer.summaries, 1)
	summary := mp.writer.summaries[0]
	assert.LessOrEqual(t, len(summary), aggregateMarkdownMaxBytes)
	assert.Contains(t, summary, "| dev | large |")
	assert.Contains(t, summary, "BEGIN small\nsmall output\nEND small")
	assert.NotContains(t, summary, "BEGIN large")
	assert.NotContains(t, summary, "END large")
	assert.Contains(t, summary, "Omitted 1 component detail section(s)")
	assert.Contains(t, summary, "> [!WARNING]")
	assert.Contains(t, summary, "Consider reducing scope using `--tags` or `--labels`.")
}

func TestAggregatePlanSummarySortsByStackComponentAndNode(t *testing.T) {
	ctx := newAggregateHookContext()
	setAggregatePlanTemplate(t, ctx, "ORDER {{.Stack}}/{{.Component}} {{.Output}}")
	ctx.Aggregate = schema.TerraformPlanCIResultSet{Results: []schema.TerraformPlanCIResult{
		{Stack: "prod", Component: "app", NodeID: "a", Processed: true, Output: "last"},
		{Stack: "dev", Component: "db", NodeID: "a", Processed: true, Output: "third"},
		{Stack: "dev", Component: "app", NodeID: "b", Processed: true, Output: "second"},
		{Stack: "dev", Component: "app", NodeID: "a", Processed: true, Output: "first"},
	}}
	require.NoError(t, (&Plugin{}).onAfterTerraformAggregate(ctx))
	mp := ctx.Provider.(*mockProvider)
	require.Len(t, mp.writer.summaries, 1)
	assert.Contains(t, mp.writer.summaries[0], "ORDER dev/app first\n\n\nORDER dev/app second\n\n\nORDER dev/db third\n\n\nORDER prod/app last\n")
}

func TestAggregatePlanSummaryUsesSchedulerFailure(t *testing.T) {
	ctx := newAggregateHookContext()
	ctx.Aggregate = schema.TerraformPlanCIResultSet{Results: []schema.TerraformPlanCIResult{
		{Stack: "dev", Component: "app", Status: "failed", Processed: true},
	}}
	require.NoError(t, (&Plugin{}).onAfterTerraformAggregate(ctx))
	mp := ctx.Provider.(*mockProvider)
	require.Len(t, mp.writer.summaries, 1)
	assert.Contains(t, mp.writer.summaries[0], "## Plan Failed for `app` in `dev`")
	assert.NotContains(t, mp.writer.summaries[0], "## No Changes")
	assert.Contains(t, mp.writer.summaries[0], "\nfailed\n")
}

func setAggregatePlanTemplate(t *testing.T, ctx *plugin.HookContext, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plan.md")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	ctx.Config.CI.Templates.Terraform = map[string]string{"plan": path}
	ctx.TemplateLoader = templates.NewLoader(ctx.Config)
}
