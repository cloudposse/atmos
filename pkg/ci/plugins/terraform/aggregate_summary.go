package terraform

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// writeAggregatePlanSummaries appends whole component summaries within the shared
// budget. Skipped components remain in the overview since they have no plan.
func (p *Plugin) writeAggregatePlanSummaries(b *strings.Builder, ctx *plugin.HookContext, components []terraformPlanAggregateComponent) int {
	omitted := 0
	for i := range components {
		component := &components[i]
		if component.Skipped {
			continue
		}
		rendered := p.renderAggregateComponentSummary(ctx, component)
		section := "\n\n" + rendered + "\n"
		if b.Len()+len(section)+aggregateMarkdownNoticeReserveBytes > aggregateMarkdownMaxBytes {
			omitted++
			continue
		}
		b.WriteString(section)
	}
	return omitted
}

// renderAggregateComponentSummary reuses the normal plan template and its
// overrides, preserving the scheduler's authoritative component status.
func (p *Plugin) renderAggregateComponentSummary(ctx *plugin.HookContext, component *terraformPlanAggregateComponent) string {
	componentCtx := p.aggregateComponentContext(ctx, component)
	componentCtx.Output = ansi.Strip(componentCtx.Output)
	// Aggregate metrics describe the parent command, not this component.
	componentCtx.Info.ExecMetadataRawMetrics = nil
	result := *component.Parsed
	result.HasErrors = component.HasErrors
	result.HasChanges = component.HasChanges
	if result.HasErrors && len(result.Errors) == 0 {
		result.Errors = []string{component.Summary}
	}

	rendered, err := p.renderSummary(componentCtx, &result)
	if err == nil {
		return rendered
	}
	log.Warn("CI component summary template failed", "stack", component.Result.Stack,
		"component", component.Result.Component, "error", err)
	return fmt.Sprintf("### %s/%s\n\nStatus: %s — %s\n\n> Component summary template failed; see component logs for full output.\n",
		markdownInline(component.Result.Stack), markdownInline(component.Result.Component),
		component.Status, markdownInline(component.Summary))
}
