package terraform

import (
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	aggregateDurationBase          = 10
	aggregateDetailTruncatedPrefix = "... output truncated ...\n"
	aggregateDurationSuffix        = "ms"
	markdownEmptyValue             = "-"
	markdownLineBreak              = "\n"
	markdownTableRowEnd            = " |\n"
	markdownTableSeparator         = " | "
)

// renderAggregatePlanMarkdown builds the deterministic CI job summary body.
func renderAggregatePlanMarkdown(aggregate *terraformPlanAggregate) string {
	var b strings.Builder
	b.WriteString("## Terraform Plan Summary\n\n")
	b.WriteString(aggregate.Summary)
	b.WriteString("\n\n")
	writeAggregateResultTable(&b, &aggregate.Counts)
	writeAggregateResourceTable(&b, &aggregate.Counts)
	writeAggregateGroupTable(&b, aggregate.Components)
	writeAggregateComponentTable(&b, aggregate.Components)
	writeAggregateDetails(&b, "Failed Components", aggregate.Components, func(c *terraformPlanAggregateComponent) bool {
		return c.HasErrors
	})
	writeAggregateDetails(&b, "Changed Components", aggregate.Components, func(c *terraformPlanAggregateComponent) bool {
		return c.HasChanges
	})
	return b.String()
}

// writeAggregateResultTable renders component status totals.
func writeAggregateResultTable(b *strings.Builder, counts *terraformPlanAggregateCounts) {
	b.WriteString("| Result | Components |\n")
	b.WriteString("| --- | ---: |\n")
	writeMarkdownCountRow(b, "Changed", counts.Changed)
	writeMarkdownCountRow(b, "Failed", counts.Failed)
	writeMarkdownCountRow(b, "No changes", counts.NoChanges)
	writeMarkdownCountRow(b, "Skipped", counts.Skipped)
	b.WriteString(markdownLineBreak)
}

// writeAggregateResourceTable renders aggregate Terraform resource action totals.
func writeAggregateResourceTable(b *strings.Builder, counts *terraformPlanAggregateCounts) {
	b.WriteString("| Resource Action | Count |\n")
	b.WriteString("| --- | ---: |\n")
	writeMarkdownCountRow(b, "Add", counts.ResourcesToCreate)
	writeMarkdownCountRow(b, "Change", counts.ResourcesToChange)
	writeMarkdownCountRow(b, "Replace", counts.ResourcesToReplace)
	writeMarkdownCountRow(b, "Destroy", counts.ResourcesToDestroy)
	b.WriteString(markdownLineBreak)
}

// writeAggregateGroupTable renders component names grouped by result.
func writeAggregateGroupTable(b *strings.Builder, components []terraformPlanAggregateComponent) {
	b.WriteString("| Group | Components |\n")
	b.WriteString("| --- | --- |\n")
	writeAggregateGroup(b, "Failed", components, func(c *terraformPlanAggregateComponent) bool {
		return c.HasErrors
	})
	writeAggregateGroup(b, "Changed", components, func(c *terraformPlanAggregateComponent) bool {
		return c.HasChanges
	})
	writeAggregateGroup(b, "No changes", components, func(c *terraformPlanAggregateComponent) bool {
		return !c.HasErrors && !c.HasChanges && !c.Skipped
	})
	writeAggregateGroup(b, "Skipped", components, func(c *terraformPlanAggregateComponent) bool {
		return c.Skipped
	})
	b.WriteString(markdownLineBreak)
}

// writeAggregateComponentTable renders the per-component status table.
func writeAggregateComponentTable(b *strings.Builder, components []terraformPlanAggregateComponent) {
	b.WriteString("| Stack | Component | Status | Summary | Add | Change | Replace | Destroy | Duration |\n")
	b.WriteString("| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: |\n")
	for i := range components {
		writeAggregateComponentRow(b, &components[i])
	}
}

// writeAggregateComponentRow renders one per-component table row.
func writeAggregateComponentRow(b *strings.Builder, component *terraformPlanAggregateComponent) {
	counts := resourceCounts(component.Data)
	b.WriteString("| ")
	b.WriteString(markdownTableCell(component.Result.Stack))
	b.WriteString(markdownTableSeparator)
	b.WriteString(markdownTableCell(component.Result.Component))
	b.WriteString(markdownTableSeparator)
	b.WriteString(markdownTableCell(component.Status))
	b.WriteString(markdownTableSeparator)
	b.WriteString(markdownTableCell(component.Summary))
	b.WriteString(markdownTableSeparator)
	b.WriteString(strconv.Itoa(counts.Create))
	b.WriteString(markdownTableSeparator)
	b.WriteString(strconv.Itoa(counts.Change))
	b.WriteString(markdownTableSeparator)
	b.WriteString(strconv.Itoa(counts.Replace))
	b.WriteString(markdownTableSeparator)
	b.WriteString(strconv.Itoa(counts.Destroy))
	b.WriteString(markdownTableSeparator)
	b.WriteString(component.DurationLabel)
	b.WriteString(markdownTableRowEnd)
}

// writeMarkdownCountRow renders one two-column Markdown count row.
func writeMarkdownCountRow(b *strings.Builder, label string, count int) {
	b.WriteString("| ")
	b.WriteString(label)
	b.WriteString(markdownTableSeparator)
	b.WriteString(strconv.Itoa(count))
	b.WriteString(markdownTableRowEnd)
}

// writeAggregateGroup renders one summary-table row for a component status group.
func writeAggregateGroup(b *strings.Builder, label string, components []terraformPlanAggregateComponent, include func(*terraformPlanAggregateComponent) bool) {
	values := make([]string, 0)
	for i := range components {
		component := &components[i]
		if include(component) {
			values = append(values, markdownTableCell(component.Result.Stack+"/"+component.Result.Component))
		}
	}
	if len(values) == 0 {
		values = append(values, markdownEmptyValue)
	}
	b.WriteString("| ")
	b.WriteString(label)
	b.WriteString(markdownTableSeparator)
	b.WriteString(strings.Join(values, ", "))
	b.WriteString(markdownTableRowEnd)
}

// writeAggregateDetails renders collapsible output sections for selected components.
func writeAggregateDetails(b *strings.Builder, title string, components []terraformPlanAggregateComponent, include func(*terraformPlanAggregateComponent) bool) {
	selected := selectedAggregateComponents(components, include)
	if len(selected) == 0 {
		return
	}

	b.WriteString("\n### ")
	b.WriteString(title)
	b.WriteString(markdownLineBreak)
	for _, component := range selected {
		writeAggregateDetail(b, component)
	}
}

// selectedAggregateComponents returns components selected for detailed rendering.
func selectedAggregateComponents(components []terraformPlanAggregateComponent, include func(*terraformPlanAggregateComponent) bool) []*terraformPlanAggregateComponent {
	selected := make([]*terraformPlanAggregateComponent, 0)
	for i := range components {
		component := &components[i]
		if include(component) {
			selected = append(selected, component)
		}
	}
	return selected
}

// writeAggregateDetail renders one collapsible component detail section.
func writeAggregateDetail(b *strings.Builder, component *terraformPlanAggregateComponent) {
	body := truncateAggregateDetail(aggregateDetailBody(component))
	b.WriteString("\n<details><summary>")
	b.WriteString(markdownInline(component.Result.Stack + "/" + component.Result.Component))
	b.WriteString(" - ")
	b.WriteString(markdownInline(component.Summary))
	b.WriteString("</summary>\n\n```hcl\n")
	b.WriteString(body)
	if body != "" && !strings.HasSuffix(body, markdownLineBreak) {
		b.WriteString(markdownLineBreak)
	}
	b.WriteString("```\n</details>\n")
}

// aggregateDetailBody chooses the best output body for a detail section.
func aggregateDetailBody(component *terraformPlanAggregateComponent) string {
	if !component.HasErrors {
		return component.CleanOutput
	}
	if component.Parsed != nil {
		if body := strings.TrimSpace(strings.Join(component.Parsed.Errors, markdownLineBreak)); body != "" {
			return body
		}
	}
	if component.Result.Error != "" {
		return component.Result.Error
	}
	return component.CleanOutput
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
	return aggregateDetailTruncatedPrefix + strings.TrimLeft(value[start:], "\r\n")
}

// markdownTableCell escapes text for a Markdown table cell.
func markdownTableCell(value string) string {
	value = strings.ReplaceAll(value, markdownLineBreak, " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.TrimSpace(value)
	if value == "" {
		return markdownEmptyValue
	}
	return value
}

// markdownInline normalizes text used inside inline Markdown elements.
func markdownInline(value string) string {
	value = strings.ReplaceAll(value, markdownLineBreak, " ")
	value = strings.TrimSpace(value)
	if value == "" {
		return markdownEmptyValue
	}
	return value
}

// formatAggregateDuration formats captured scheduler timing for CI output.
func formatAggregateDuration(result *schema.TerraformPlanCIResult) string {
	if result.DurationMS > 0 {
		return strconv.FormatInt(result.DurationMS, aggregateDurationBase) + aggregateDurationSuffix
	}
	if !result.StartedAt.IsZero() && !result.FinishedAt.IsZero() {
		return strconv.FormatInt(result.FinishedAt.Sub(result.StartedAt).Milliseconds(), aggregateDurationBase) + aggregateDurationSuffix
	}
	return markdownEmptyValue
}
