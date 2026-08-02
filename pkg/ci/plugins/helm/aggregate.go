package helm

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const helmAggregateMarkdownMaxBytes = 960 * 1024

type helmAggregate struct {
	Command    string
	Components []helmAggregateComponent
	Counts     helmAggregateCounts
}

type helmAggregateComponent struct {
	Result  schema.HelmCIResult
	Summary Summary
	Status  string
}

type helmAggregateCounts struct {
	Total     int
	Succeeded int
	Failed    int
	Changed   int
	NoChanges int
	Skipped   int
}

type helmAggregateCountRow struct {
	label string
	count int
}

// onAfterAggregate writes one deterministic job summary for a graph-backed
// Helm plan or apply command.
func (p *Plugin) onAfterAggregate(ctx *plugin.HookContext) error {
	defer perf.Track(ctx.Config, "helmci.Plugin.onAfterAggregate")()

	resultSet, ok := normalizeHelmAggregate(ctx.Aggregate)
	if !ok || len(resultSet.Results) == 0 {
		log.Debug("Skipping aggregate Helm CI hook: no results")
		return nil
	}
	if !isSummaryEnabled(ctx.Config) {
		return nil
	}
	writer := ctx.Provider.OutputWriter()
	if writer == nil {
		return nil
	}
	aggregate := buildHelmAggregate(resultSet)
	return writer.WriteSummary(renderHelmAggregateMarkdown(&aggregate))
}

func normalizeHelmAggregate(value any) (schema.HelmCIResultSet, bool) {
	switch typed := value.(type) {
	case schema.HelmCIResultSet:
		return typed, true
	case *schema.HelmCIResultSet:
		if typed == nil {
			return schema.HelmCIResultSet{}, false
		}
		return *typed, true
	default:
		return schema.HelmCIResultSet{}, false
	}
}

func buildHelmAggregate(resultSet schema.HelmCIResultSet) helmAggregate {
	results := append([]schema.HelmCIResult(nil), resultSet.Results...)
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Stack != results[j].Stack {
			return results[i].Stack < results[j].Stack
		}
		if results[i].Component != results[j].Component {
			return results[i].Component < results[j].Component
		}
		return results[i].NodeID < results[j].NodeID
	})

	aggregate := helmAggregate{Command: normalizeHelmAggregateCommand(resultSet.Command)}
	aggregate.Components = make([]helmAggregateComponent, 0, len(results))
	for i := range results {
		component := helmAggregateComponent{
			Result:  results[i],
			Summary: normalizeSummary(results[i].Summary),
		}
		component.Status = helmAggregateStatus(aggregate.Command, &component)
		aggregate.Components = append(aggregate.Components, component)
		aggregate.Counts.add(component.Status)
	}
	return aggregate
}

func normalizeHelmAggregateCommand(command string) string {
	switch command {
	case "apply", "deploy":
		return "apply"
	default:
		return "plan"
	}
}

func helmAggregateStatus(command string, component *helmAggregateComponent) string {
	if component.Result.Status == "failed" || component.Result.Error != "" || component.Result.ExitCode != 0 {
		return "failed"
	}
	if component.Result.Status == "skipped" || !component.Result.Processed {
		return "skipped"
	}
	if command == "plan" {
		if strings.TrimSpace(component.Summary.Diff) != "" {
			return "changed"
		}
		return "no changes"
	}
	return "succeeded"
}

func (counts *helmAggregateCounts) add(status string) {
	counts.Total++
	switch status {
	case "failed":
		counts.Failed++
	case "changed":
		counts.Changed++
	case "no changes":
		counts.NoChanges++
	case "skipped":
		counts.Skipped++
	default:
		counts.Succeeded++
	}
}

func renderHelmAggregateMarkdown(aggregate *helmAggregate) string {
	var builder strings.Builder
	builder.WriteString("## Helm ")
	builder.WriteString(helmAggregateCommandLabel(aggregate.Command))
	builder.WriteString(" Summary\n\n")
	builder.WriteString(helmAggregateSummaryText(aggregate.Command, &aggregate.Counts))
	builder.WriteString("\n\n")
	writeHelmAggregateCounts(&builder, aggregate.Command, &aggregate.Counts)
	writeHelmAggregateComponents(&builder, aggregate.Components)
	writeHelmAggregateDetails(&builder, aggregate.Command, aggregate.Components)
	return enforceHelmAggregateMarkdownLimit(builder.String())
}

func helmAggregateCommandLabel(command string) string {
	if command == "apply" {
		return "Apply"
	}
	return "Plan"
}

func helmAggregateSummaryText(command string, counts *helmAggregateCounts) string {
	if command == "apply" {
		return fmt.Sprintf(
			"Processed %d component(s): %d succeeded, %d failed, %d skipped.",
			counts.Total,
			counts.Succeeded,
			counts.Failed,
			counts.Skipped,
		)
	}
	return fmt.Sprintf(
		"Processed %d component(s): %d changed, %d unchanged, %d failed, %d skipped.",
		counts.Total,
		counts.Changed,
		counts.NoChanges,
		counts.Failed,
		counts.Skipped,
	)
}

func writeHelmAggregateCounts(builder *strings.Builder, command string, counts *helmAggregateCounts) {
	builder.WriteString("| Result | Components |\n")
	builder.WriteString("| --- | ---: |\n")
	rows := make([]helmAggregateCountRow, 0, 4)
	if command == "apply" {
		rows = append(rows, helmAggregateCountRow{label: "Succeeded", count: counts.Succeeded})
	} else {
		rows = append(rows,
			helmAggregateCountRow{label: "Changed", count: counts.Changed},
			helmAggregateCountRow{label: "No changes", count: counts.NoChanges},
		)
	}
	rows = append(rows,
		helmAggregateCountRow{label: "Failed", count: counts.Failed},
		helmAggregateCountRow{label: "Skipped", count: counts.Skipped},
	)
	for _, row := range rows {
		builder.WriteString("| ")
		builder.WriteString(row.label)
		builder.WriteString(" | ")
		builder.WriteString(strconv.Itoa(row.count))
		builder.WriteString(" |\n")
	}
	builder.WriteString("\n")
}

func writeHelmAggregateComponents(builder *strings.Builder, components []helmAggregateComponent) {
	builder.WriteString("| Stack | Component | Status | Chart | Release | Namespace | Target | Duration |\n")
	builder.WriteString("| --- | --- | --- | --- | --- | --- | --- | ---: |\n")
	for i := range components {
		component := &components[i]
		values := []string{
			component.Result.Stack,
			component.Result.Component,
			component.Status,
			component.Summary.Chart,
			component.Summary.ReleaseName,
			component.Summary.Namespace,
			component.Summary.Target,
			formatHelmAggregateDuration(component.Result.DurationMS),
		}
		builder.WriteString("| ")
		for index, value := range values {
			if index > 0 {
				builder.WriteString(" | ")
			}
			builder.WriteString(helmMarkdownCell(value))
		}
		builder.WriteString(" |\n")
	}
	builder.WriteString("\n")
}

func writeHelmAggregateDetails(builder *strings.Builder, command string, components []helmAggregateComponent) {
	for i := range components {
		component := &components[i]
		if component.Status != "failed" && !(command == "plan" && component.Status == "changed") {
			continue
		}
		var detail strings.Builder
		detail.WriteString("<details><summary>")
		detail.WriteString(helmMarkdownCell(component.Result.Stack + "/" + component.Result.Component + ": " + component.Status))
		detail.WriteString("</summary>\n\n")
		if component.Status == "failed" {
			detail.WriteString("```text\n")
			detail.WriteString(plugin.TruncateDetail(component.Result.Error))
			detail.WriteString("\n```\n")
		} else {
			detail.WriteString("```diff\n")
			detail.WriteString(plugin.TruncateDetail(component.Summary.Diff))
			detail.WriteString("\n```\n")
		}
		detail.WriteString("\n</details>\n\n")
		if builder.Len()+detail.Len() > helmAggregateMarkdownMaxBytes {
			builder.WriteString("> [!WARNING]\n> Additional component details were omitted to stay below GitHub Actions' job summary limit.\n")
			return
		}
		builder.WriteString(detail.String())
	}
}

func formatHelmAggregateDuration(milliseconds int64) string {
	if milliseconds <= 0 {
		return "-"
	}
	return strconv.FormatInt(milliseconds, 10) + "ms"
}

func helmMarkdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\r", " ")
	return strings.ReplaceAll(value, "\n", " ")
}

func enforceHelmAggregateMarkdownLimit(markdown string) string {
	if len(markdown) <= helmAggregateMarkdownMaxBytes {
		return markdown
	}
	notice := "\n\n> [!WARNING]\n> Summary truncated to stay below GitHub Actions' job summary limit.\n"
	limit := helmAggregateMarkdownMaxBytes - len(notice)
	return trimHelmAggregateMarkdownToLimit(markdown, limit) + notice
}

func trimHelmAggregateMarkdownToLimit(markdown string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(markdown) <= maxBytes {
		return markdown
	}

	end := maxBytes
	if lineEnd := strings.LastIndexByte(markdown[:maxBytes], '\n'); lineEnd > 0 {
		end = lineEnd
	}
	for end > 0 && !utf8.ValidString(markdown[:end]) {
		end--
	}
	return strings.TrimRight(markdown[:end], "\r\n")
}
