package terraform

import (
	"fmt"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	aggregateComponentName               = "aggregate"
	aggregateCommandPlan                 = "plan"
	aggregateStackAll                    = "all"
	aggregateDetailOutputMaxBytes        = 12 * 1024
	aggregateDetailLineBacktrackMaxBytes = 4 * 1024
	aggregateStatusChanged               = "changed"
	aggregateStatusFailed                = "failed"
	aggregateStatusNoChanges             = "no changes"
	aggregateStatusSkipped               = "skipped"
)

// terraformPlanAggregate is the complete rendered model for one graph-backed plan run.
type terraformPlanAggregate struct {
	Components []terraformPlanAggregateComponent
	Counts     terraformPlanAggregateCounts
	Summary    string
	ExitCode   int
	Markdown   string
}

// terraformPlanAggregateCounts tracks aggregate component and resource totals.
type terraformPlanAggregateCounts struct {
	Total              int
	Succeeded          int
	Failed             int
	Changed            int
	NoChanges          int
	Skipped            int
	ResourcesToCreate  int
	ResourcesToChange  int
	ResourcesToReplace int
	ResourcesToDestroy int
}

// terraformPlanAggregateComponent is the rendered CI model for one component.
type terraformPlanAggregateComponent struct {
	Result        schema.TerraformPlanCIResult
	Parsed        *plugin.OutputResult
	Data          *plugin.TerraformOutputData
	Status        string
	Summary       string
	CleanOutput   string
	HasChanges    bool
	HasErrors     bool
	Skipped       bool
	DurationLabel string
}

// onAfterPlanAggregate handles the after.terraform.plan.aggregate event for
// graph-backed multi-component plan runs. It writes GitHub/GitLab CI artifacts
// once after the scheduler completes, avoiding concurrent writes from worker
// goroutines.
func (p *Plugin) onAfterPlanAggregate(ctx *plugin.HookContext) error {
	defer perf.Track(ctx.Config, "terraform.Plugin.onAfterPlanAggregate")()

	resultSet, ok := normalizeTerraformPlanAggregate(ctx.Aggregate)
	if !ok || len(resultSet.Results) == 0 {
		log.Debug("Skipping aggregate Terraform plan CI hook: no results")
		return nil
	}

	aggregate := p.buildPlanAggregate(resultSet)

	if err := p.writeAggregateArtifacts(ctx, &aggregate); err != nil {
		return err
	}

	return nil
}

// writeAggregateArtifacts serializes all CI provider side effects for an aggregate plan.
func (p *Plugin) writeAggregateArtifacts(ctx *plugin.HookContext, aggregate *terraformPlanAggregate) error {
	if isSummaryEnabled(ctx.Config) {
		if err := p.writeAggregateSummary(ctx, aggregate.Markdown); err != nil {
			log.Warn("CI aggregate summary failed", "error", err)
		}
	}

	if isOutputEnabled(ctx.Config) {
		if err := p.writeAggregateOutputs(ctx, aggregate); err != nil {
			log.Warn("CI aggregate output failed", "error", err)
		}
	}

	if isPlanfileStorageEnabled(ctx.Config) {
		if err := p.uploadAggregatePlanfiles(ctx, aggregate); err != nil {
			return err
		}
	}

	if isCheckEnabled(ctx.Config) {
		p.updateAggregateCheckRuns(ctx, aggregate)
	}

	if isCommentsEnabled(ctx.Config) {
		if err := p.postAggregateComment(ctx, aggregate.Markdown); err != nil {
			logCommentError("CI aggregate PR comment skipped", err)
		}
	}

	return nil
}

// normalizeTerraformPlanAggregate extracts a Terraform plan result set from hook payload data.
func normalizeTerraformPlanAggregate(value any) (schema.TerraformPlanCIResultSet, bool) {
	switch v := value.(type) {
	case schema.TerraformPlanCIResultSet:
		return v, true
	case *schema.TerraformPlanCIResultSet:
		if v == nil {
			return schema.TerraformPlanCIResultSet{}, false
		}
		return *v, true
	default:
		return schema.TerraformPlanCIResultSet{}, false
	}
}

// buildPlanAggregate sorts scheduler results and calculates aggregate plan totals.
func (p *Plugin) buildPlanAggregate(resultSet schema.TerraformPlanCIResultSet) terraformPlanAggregate {
	results := append([]schema.TerraformPlanCIResult(nil), resultSet.Results...)
	sort.SliceStable(results, func(i, j int) bool {
		left := &results[i]
		right := &results[j]
		if left.Stack != right.Stack {
			return left.Stack < right.Stack
		}
		if left.Component != right.Component {
			return left.Component < right.Component
		}
		return left.NodeID < right.NodeID
	})

	aggregate := terraformPlanAggregate{
		Components: make([]terraformPlanAggregateComponent, 0, len(results)),
	}

	for i := range results {
		component := p.buildPlanAggregateComponent(&results[i])
		aggregate.addComponent(&component)
	}

	aggregate.ExitCode = aggregateExitCode(&aggregate.Counts)
	aggregate.Summary = aggregateSummaryText(&aggregate.Counts)
	aggregate.Markdown = renderAggregatePlanMarkdown(&aggregate)
	return aggregate
}

// addComponent appends one component result and updates aggregate counts.
func (a *terraformPlanAggregate) addComponent(component *terraformPlanAggregateComponent) {
	a.Components = append(a.Components, *component)
	a.Counts.Total++
	switch {
	case component.Skipped:
		a.Counts.Skipped++
	case component.HasErrors:
		a.Counts.Failed++
	default:
		a.Counts.Succeeded++
		if component.HasChanges {
			a.Counts.Changed++
		} else {
			a.Counts.NoChanges++
		}
	}
	a.addResourceCounts(component)
}

// addResourceCounts folds successful component resource counts into the aggregate totals.
func (a *terraformPlanAggregate) addResourceCounts(component *terraformPlanAggregateComponent) {
	if component.Data == nil || component.HasErrors || component.Skipped {
		return
	}
	counts := resourceCounts(component.Data)
	a.Counts.ResourcesToCreate += counts.Create
	a.Counts.ResourcesToChange += counts.Change
	a.Counts.ResourcesToReplace += counts.Replace
	a.Counts.ResourcesToDestroy += counts.Destroy
}

// buildPlanAggregateComponent parses one scheduler result into its CI rendering model.
func (p *Plugin) buildPlanAggregateComponent(result *schema.TerraformPlanCIResult) terraformPlanAggregateComponent {
	output := ansi.Strip(result.Output)
	parsed := p.parseOutputWithError(&plugin.HookContext{
		Command:      aggregateCommandPlan,
		Output:       output,
		CommandError: aggregateCommandError(result.Error),
		ExitCode:     result.ExitCode,
	})
	if result.Changed {
		parsed.HasChanges = true
	}

	data, _ := parsed.Data.(*plugin.TerraformOutputData)
	skipped, hasErrors, hasChanges := aggregateComponentFlags(result, parsed)
	status := aggregateComponentStatus(skipped, hasErrors, hasChanges)

	return terraformPlanAggregateComponent{
		Result:        *result,
		Parsed:        parsed,
		Data:          data,
		Status:        status,
		Summary:       componentSummaryText(result, parsed, data, status),
		CleanOutput:   cleanPlanOutput(output),
		HasChanges:    hasChanges,
		HasErrors:     hasErrors,
		Skipped:       skipped,
		DurationLabel: formatAggregateDuration(result),
	}
}

// aggregateCommandError converts scheduler error text into a static-sentinel error chain.
func aggregateCommandError(message string) error {
	if message == "" {
		return nil
	}
	return fmt.Errorf("%w: %s", errUtils.ErrTerraformExecFailed, message)
}

// aggregateComponentFlags derives status booleans from scheduler and parser outcomes.
func aggregateComponentFlags(result *schema.TerraformPlanCIResult, parsed *plugin.OutputResult) (bool, bool, bool) {
	skipped := result.Status == aggregateStatusSkipped || !result.Processed
	hasErrors := !skipped && (result.Status == aggregateStatusFailed || parsed.HasErrors)
	hasChanges := !skipped && !hasErrors && parsed.HasChanges
	return skipped, hasErrors, hasChanges
}

// aggregateComponentStatus maps aggregate component flags to the rendered status.
func aggregateComponentStatus(skipped, hasErrors, hasChanges bool) string {
	switch {
	case skipped:
		return aggregateStatusSkipped
	case hasErrors:
		return aggregateStatusFailed
	case hasChanges:
		return aggregateStatusChanged
	default:
		return aggregateStatusNoChanges
	}
}

// aggregateExitCode applies Terraform plan exit-code semantics across all components.
func aggregateExitCode(counts *terraformPlanAggregateCounts) int {
	if counts.Failed > 0 {
		return 1
	}
	if counts.Changed > 0 {
		return 2
	}
	return 0
}

// aggregateSummaryText formats the compact aggregate status line.
func aggregateSummaryText(counts *terraformPlanAggregateCounts) string {
	return fmt.Sprintf(
		"%d components: %d changed, %d failed, %d skipped",
		counts.Total,
		counts.Changed,
		counts.Failed,
		counts.Skipped,
	)
}

// componentSummaryText chooses the per-component summary shown in CI tables.
func componentSummaryText(result *schema.TerraformPlanCIResult, parsed *plugin.OutputResult, data *plugin.TerraformOutputData, status string) string {
	switch status {
	case aggregateStatusFailed:
		return failedComponentSummary(result, parsed)
	case aggregateStatusSkipped:
		if result.Error != "" {
			return result.Error
		}
		return aggregateStatusSkipped
	}
	if data != nil && data.ChangedResult != "" {
		return data.ChangedResult
	}
	if parsed != nil && parsed.HasChanges {
		return "Changes detected"
	}
	return "No changes"
}

// failedComponentSummary returns the most useful failure text for a component.
func failedComponentSummary(result *schema.TerraformPlanCIResult, parsed *plugin.OutputResult) string {
	if result.Error != "" {
		return result.Error
	}
	if parsed != nil && len(parsed.Errors) > 0 {
		return strings.Join(parsed.Errors, "; ")
	}
	return aggregateStatusFailed
}
