package terraform

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	aggregateComponentName               = "aggregate"
	aggregateStackAll                    = "all"
	aggregateDetailOutputMaxBytes        = 12 * 1024
	aggregateDetailLineBacktrackMaxBytes = 4 * 1024
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
		left := results[i]
		right := results[j]
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

	for _, result := range results {
		component := p.buildPlanAggregateComponent(result)
		aggregate.Components = append(aggregate.Components, component)
		aggregate.Counts.Total++
		switch {
		case component.Skipped:
			aggregate.Counts.Skipped++
		case component.HasErrors:
			aggregate.Counts.Failed++
		default:
			aggregate.Counts.Succeeded++
			if component.HasChanges {
				aggregate.Counts.Changed++
			} else {
				aggregate.Counts.NoChanges++
			}
		}
		if component.Data != nil && !component.HasErrors && !component.Skipped {
			counts := resourceCounts(component.Data)
			aggregate.Counts.ResourcesToCreate += counts.Create
			aggregate.Counts.ResourcesToChange += counts.Change
			aggregate.Counts.ResourcesToReplace += counts.Replace
			aggregate.Counts.ResourcesToDestroy += counts.Destroy
		}
	}

	aggregate.ExitCode = aggregateExitCode(aggregate.Counts)
	aggregate.Summary = aggregateSummaryText(aggregate.Counts)
	aggregate.Markdown = renderAggregatePlanMarkdown(aggregate)
	return aggregate
}

// buildPlanAggregateComponent parses one scheduler result into its CI rendering model.
func (p *Plugin) buildPlanAggregateComponent(result schema.TerraformPlanCIResult) terraformPlanAggregateComponent {
	output := ansi.Strip(result.Output)
	commandErr := error(nil)
	if result.Error != "" {
		commandErr = errors.New(result.Error)
	}
	parsed := p.parseOutputWithError(&plugin.HookContext{
		Command:      "plan",
		Output:       output,
		CommandError: commandErr,
		ExitCode:     result.ExitCode,
	})
	if result.Changed {
		parsed.HasChanges = true
	}

	data, _ := parsed.Data.(*plugin.TerraformOutputData)
	skipped := result.Status == "skipped" || !result.Processed
	hasErrors := !skipped && (result.Status == "failed" || parsed.HasErrors)
	hasChanges := !skipped && !hasErrors && parsed.HasChanges
	status := "no changes"
	switch {
	case skipped:
		status = "skipped"
	case hasErrors:
		status = "failed"
	case hasChanges:
		status = "changed"
	}

	return terraformPlanAggregateComponent{
		Result:        result,
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

// aggregateExitCode applies Terraform plan exit-code semantics across all components.
func aggregateExitCode(counts terraformPlanAggregateCounts) int {
	if counts.Failed > 0 {
		return 1
	}
	if counts.Changed > 0 {
		return 2
	}
	return 0
}

// aggregateSummaryText formats the compact aggregate status line.
func aggregateSummaryText(counts terraformPlanAggregateCounts) string {
	return fmt.Sprintf(
		"%d components: %d changed, %d failed, %d skipped",
		counts.Total,
		counts.Changed,
		counts.Failed,
		counts.Skipped,
	)
}

// componentSummaryText chooses the per-component summary shown in CI tables.
func componentSummaryText(result schema.TerraformPlanCIResult, parsed *plugin.OutputResult, data *plugin.TerraformOutputData, status string) string {
	switch status {
	case "failed":
		if result.Error != "" {
			return result.Error
		}
		if parsed != nil && len(parsed.Errors) > 0 {
			return strings.Join(parsed.Errors, "; ")
		}
		return "failed"
	case "skipped":
		if result.Error != "" {
			return result.Error
		}
		return "skipped"
	}
	if data != nil && data.ChangedResult != "" {
		return data.ChangedResult
	}
	if parsed != nil && parsed.HasChanges {
		return "Changes detected"
	}
	return "No changes"
}

// renderAggregatePlanMarkdown builds the deterministic CI job summary body.
func renderAggregatePlanMarkdown(aggregate terraformPlanAggregate) string {
	var b strings.Builder
	counts := aggregate.Counts
	b.WriteString("## Terraform Plan Summary\n\n")
	b.WriteString(aggregate.Summary)
	b.WriteString("\n\n")
	b.WriteString("| Result | Components |\n")
	b.WriteString("| --- | ---: |\n")
	b.WriteString("| Changed | ")
	b.WriteString(strconv.Itoa(counts.Changed))
	b.WriteString(" |\n")
	b.WriteString("| Failed | ")
	b.WriteString(strconv.Itoa(counts.Failed))
	b.WriteString(" |\n")
	b.WriteString("| No changes | ")
	b.WriteString(strconv.Itoa(counts.NoChanges))
	b.WriteString(" |\n")
	b.WriteString("| Skipped | ")
	b.WriteString(strconv.Itoa(counts.Skipped))
	b.WriteString(" |\n\n")

	b.WriteString("| Resource Action | Count |\n")
	b.WriteString("| --- | ---: |\n")
	b.WriteString("| Add | ")
	b.WriteString(strconv.Itoa(counts.ResourcesToCreate))
	b.WriteString(" |\n")
	b.WriteString("| Change | ")
	b.WriteString(strconv.Itoa(counts.ResourcesToChange))
	b.WriteString(" |\n")
	b.WriteString("| Replace | ")
	b.WriteString(strconv.Itoa(counts.ResourcesToReplace))
	b.WriteString(" |\n")
	b.WriteString("| Destroy | ")
	b.WriteString(strconv.Itoa(counts.ResourcesToDestroy))
	b.WriteString(" |\n\n")

	b.WriteString("| Group | Components |\n")
	b.WriteString("| --- | --- |\n")
	writeAggregateGroup(&b, "Failed", aggregate.Components, func(c terraformPlanAggregateComponent) bool {
		return c.HasErrors
	})
	writeAggregateGroup(&b, "Changed", aggregate.Components, func(c terraformPlanAggregateComponent) bool {
		return c.HasChanges
	})
	writeAggregateGroup(&b, "No changes", aggregate.Components, func(c terraformPlanAggregateComponent) bool {
		return !c.HasErrors && !c.HasChanges && !c.Skipped
	})
	writeAggregateGroup(&b, "Skipped", aggregate.Components, func(c terraformPlanAggregateComponent) bool {
		return c.Skipped
	})
	b.WriteString("\n")

	b.WriteString("| Stack | Component | Status | Summary | Add | Change | Replace | Destroy | Duration |\n")
	b.WriteString("| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: |\n")
	for _, component := range aggregate.Components {
		counts := resourceCounts(component.Data)
		b.WriteString("| ")
		b.WriteString(markdownTableCell(component.Result.Stack))
		b.WriteString(" | ")
		b.WriteString(markdownTableCell(component.Result.Component))
		b.WriteString(" | ")
		b.WriteString(markdownTableCell(component.Status))
		b.WriteString(" | ")
		b.WriteString(markdownTableCell(component.Summary))
		b.WriteString(" | ")
		b.WriteString(strconv.Itoa(counts.Create))
		b.WriteString(" | ")
		b.WriteString(strconv.Itoa(counts.Change))
		b.WriteString(" | ")
		b.WriteString(strconv.Itoa(counts.Replace))
		b.WriteString(" | ")
		b.WriteString(strconv.Itoa(counts.Destroy))
		b.WriteString(" | ")
		b.WriteString(component.DurationLabel)
		b.WriteString(" |\n")
	}

	writeAggregateDetails(&b, "Failed Components", aggregate.Components, func(c terraformPlanAggregateComponent) bool {
		return c.HasErrors
	})
	writeAggregateDetails(&b, "Changed Components", aggregate.Components, func(c terraformPlanAggregateComponent) bool {
		return c.HasChanges
	})

	return b.String()
}

// writeAggregateGroup renders one summary-table row for a component status group.
func writeAggregateGroup(b *strings.Builder, label string, components []terraformPlanAggregateComponent, include func(terraformPlanAggregateComponent) bool) {
	values := make([]string, 0)
	for _, component := range components {
		if include(component) {
			values = append(values, markdownTableCell(component.Result.Stack+"/"+component.Result.Component))
		}
	}
	if len(values) == 0 {
		values = append(values, "-")
	}
	b.WriteString("| ")
	b.WriteString(label)
	b.WriteString(" | ")
	b.WriteString(strings.Join(values, ", "))
	b.WriteString(" |\n")
}

// writeAggregateDetails renders collapsible output sections for selected components.
func writeAggregateDetails(b *strings.Builder, title string, components []terraformPlanAggregateComponent, include func(terraformPlanAggregateComponent) bool) {
	var selected []terraformPlanAggregateComponent
	for _, component := range components {
		if include(component) {
			selected = append(selected, component)
		}
	}
	if len(selected) == 0 {
		return
	}

	b.WriteString("\n### ")
	b.WriteString(title)
	b.WriteString("\n")
	for _, component := range selected {
		label := component.Result.Stack + "/" + component.Result.Component
		body := component.CleanOutput
		if component.HasErrors {
			if component.Parsed != nil {
				body = strings.TrimSpace(strings.Join(component.Parsed.Errors, "\n"))
			}
			if body == "" {
				body = component.Result.Error
			}
			if body == "" {
				body = component.CleanOutput
			}
		}
		body = truncateAggregateDetail(body)
		b.WriteString("\n<details><summary>")
		b.WriteString(markdownInline(label))
		b.WriteString(" - ")
		b.WriteString(markdownInline(component.Summary))
		b.WriteString("</summary>\n\n```hcl\n")
		b.WriteString(body)
		if body != "" && !strings.HasSuffix(body, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("```\n</details>\n")
	}
}

// resourceCounts returns parsed Terraform resource counts with replace fallbacks.
func resourceCounts(data *plugin.TerraformOutputData) plugin.ResourceCounts {
	if data == nil {
		return plugin.ResourceCounts{}
	}
	counts := data.ResourceCounts
	if counts.Replace == 0 && len(data.ReplacedResources) > 0 {
		counts.Replace = len(data.ReplacedResources)
	}
	return counts
}

// truncateAggregateDetail caps long Terraform output while preserving the tail.
func truncateAggregateDetail(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= aggregateDetailOutputMaxBytes {
		return value
	}
	start := len(value) - aggregateDetailOutputMaxBytes
	if prev := strings.LastIndexByte(value[:start], '\n'); prev >= 0 && start-prev <= aggregateDetailLineBacktrackMaxBytes {
		start = prev + 1
	} else if next := strings.IndexByte(value[start:], '\n'); next >= 0 {
		start += next + 1
	}
	return "... output truncated ...\n" + strings.TrimLeft(value[start:], "\r\n")
}

// markdownTableCell escapes text for a Markdown table cell.
func markdownTableCell(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

// markdownInline normalizes text used inside inline Markdown elements.
func markdownInline(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

// formatAggregateDuration formats captured scheduler timing for CI output.
func formatAggregateDuration(result schema.TerraformPlanCIResult) string {
	if result.DurationMS > 0 {
		return strconv.FormatInt(result.DurationMS, 10) + "ms"
	}
	if !result.StartedAt.IsZero() && !result.FinishedAt.IsZero() {
		return strconv.FormatInt(result.FinishedAt.Sub(result.StartedAt).Milliseconds(), 10) + "ms"
	}
	return "-"
}

// writeAggregateSummary writes the rendered aggregate summary through the CI provider.
func (p *Plugin) writeAggregateSummary(ctx *plugin.HookContext, rendered string) error {
	writer := ctx.Provider.OutputWriter()
	if writer == nil {
		log.Debug("CI platform does not support summaries")
		return nil
	}
	return writer.WriteSummary(rendered)
}

// writeAggregateOutputs writes deterministic aggregate output variables through the CI provider.
func (p *Plugin) writeAggregateOutputs(ctx *plugin.HookContext, aggregate terraformPlanAggregate) error {
	writer := ctx.Provider.OutputWriter()
	if writer == nil {
		log.Debug("CI platform does not support outputs")
		return nil
	}

	counts := aggregate.Counts
	vars := map[string]string{
		"has_changes":           strconv.FormatBool(counts.Changed > 0),
		"has_errors":            strconv.FormatBool(counts.Failed > 0),
		"exit_code":             strconv.Itoa(aggregate.ExitCode),
		"resources_to_create":   strconv.Itoa(counts.ResourcesToCreate),
		"resources_to_change":   strconv.Itoa(counts.ResourcesToChange),
		"resources_to_replace":  strconv.Itoa(counts.ResourcesToReplace),
		"resources_to_destroy":  strconv.Itoa(counts.ResourcesToDestroy),
		"components_total":      strconv.Itoa(counts.Total),
		"components_succeeded":  strconv.Itoa(counts.Succeeded),
		"components_failed":     strconv.Itoa(counts.Failed),
		"components_changed":    strconv.Itoa(counts.Changed),
		"components_no_changes": strconv.Itoa(counts.NoChanges),
		"components_skipped":    strconv.Itoa(counts.Skipped),
		"summary":               aggregate.Markdown,
		"command":               "plan",
		"stack":                 aggregateStackValue(ctx.Info),
		"component":             aggregateComponentName,
	}

	if ctx.Config != nil && len(ctx.Config.CI.Output.Variables) > 0 {
		vars = filterVariables(vars, ctx.Config.CI.Output.Variables)
	}

	keys := make([]string, 0, len(vars))
	for key := range vars {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	writeErrs := make([]error, 0)
	for _, key := range keys {
		if err := writer.WriteOutput(key, vars[key]); err != nil {
			writeErrs = append(writeErrs, fmt.Errorf("failed to write aggregate CI output %q: %w", key, err))
		}
	}
	return errors.Join(writeErrs...)
}

// aggregateStackValue returns the requested stack or the aggregate all-stacks marker.
func aggregateStackValue(info *schema.ConfigAndStacksInfo) string {
	if info != nil && info.Stack != "" {
		return info.Stack
	}
	return aggregateStackAll
}

// uploadAggregatePlanfiles uploads planfiles only for completed, non-failed components.
func (p *Plugin) uploadAggregatePlanfiles(ctx *plugin.HookContext, aggregate terraformPlanAggregate) error {
	for _, component := range aggregate.Components {
		if component.Skipped || component.HasErrors {
			continue
		}
		componentCtx := p.aggregateComponentContext(ctx, component)
		if err := p.uploadPlanfile(componentCtx); err != nil {
			return err
		}
	}
	return nil
}

// updateAggregateCheckRuns serializes per-component status context updates.
func (p *Plugin) updateAggregateCheckRuns(ctx *plugin.HookContext, aggregate terraformPlanAggregate) {
	for _, component := range aggregate.Components {
		componentCtx := p.aggregateComponentContext(ctx, component)
		if component.Skipped {
			componentCtx.CommandError = errors.New(component.Summary)
			componentCtx.ExitCode = 1
		}
		if err := p.updateCheckRun(componentCtx, component.Parsed); err != nil {
			logCheckRunError("CI aggregate check run update skipped", err)
		}
	}
}

// aggregateComponentContext builds a component-scoped hook context from aggregate data.
func (p *Plugin) aggregateComponentContext(ctx *plugin.HookContext, component terraformPlanAggregateComponent) *plugin.HookContext {
	info := schema.ConfigAndStacksInfo{}
	if ctx.Info != nil {
		info = *ctx.Info
	}
	info.Stack = component.Result.Stack
	info.StackFromArg = component.Result.Stack
	info.Component = component.Result.Component
	info.ComponentFromArg = component.Result.Component

	commandErr := error(nil)
	if component.Result.Error != "" {
		commandErr = errors.New(component.Result.Error)
	}
	return &plugin.HookContext{
		Event:               ctx.Event,
		Command:             "plan",
		EventPrefix:         ctx.EventPrefix,
		Config:              ctx.Config,
		Info:                &info,
		Output:              component.Result.Output,
		CommandError:        commandErr,
		ExitCode:            component.Result.ExitCode,
		Provider:            ctx.Provider,
		CICtx:               ctx.CICtx,
		TemplateLoader:      ctx.TemplateLoader,
		CreatePlanfileStore: ctx.CreatePlanfileStore,
	}
}

// postAggregateComment creates or updates the aggregate PR comment through the CI provider.
func (p *Plugin) postAggregateComment(ctx *plugin.HookContext, renderedSummary string) error {
	if reason := shouldSkipComment(ctx, renderedSummary); reason != "" {
		log.Debug("Skipping aggregate PR comment", "reason", reason)
		return nil
	}

	behavior, err := resolveCommentBehavior(ctx.Config)
	if err != nil {
		return err
	}

	stack := aggregateStackValue(ctx.Info)
	marker := buildAggregateCommentMarker(ctx.Command, stack)
	opts := &provider.PostCommentOptions{
		Owner:    ctx.CICtx.RepoOwner,
		Repo:     ctx.CICtx.RepoName,
		PRNumber: ctx.CICtx.PullRequest.Number,
		Marker:   marker,
		Body:     marker + "\n" + renderedSummary,
		Behavior: behavior,
	}

	result, err := ctx.Provider.PostComment(context.Background(), opts)
	if err != nil {
		return err
	}
	logCommentResult(ctx.CICtx.PullRequest.Number, result)
	return nil
}

// buildAggregateCommentMarker returns the stable marker for aggregate PR comments.
func buildAggregateCommentMarker(command, stack string) string {
	if stack == "" {
		stack = aggregateStackAll
	}
	return fmt.Sprintf("<!-- atmos:ci:%s:aggregate:%s -->", command, stack)
}
