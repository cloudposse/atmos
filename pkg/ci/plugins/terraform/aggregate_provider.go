package terraform

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

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
func (p *Plugin) writeAggregateOutputs(ctx *plugin.HookContext, aggregate *terraformPlanAggregate) error {
	writer := ctx.Provider.OutputWriter()
	if writer == nil {
		log.Debug("CI platform does not support outputs")
		return nil
	}

	vars := aggregateOutputVariables(ctx, aggregate)
	if ctx.Config != nil && len(ctx.Config.CI.Output.Variables) > 0 {
		vars = filterVariables(vars, ctx.Config.CI.Output.Variables)
	}
	return writeAggregateOutputVariables(writer, vars)
}

// aggregateOutputVariables returns the deterministic output variable map.
func aggregateOutputVariables(ctx *plugin.HookContext, aggregate *terraformPlanAggregate) map[string]string {
	counts := &aggregate.Counts
	return map[string]string{
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
		"command":               aggregate.Command,
		"stack":                 aggregateStackValue(ctx.Info),
		"component":             aggregateComponentName,
	}
}

// writeAggregateOutputVariables writes variables in stable key order.
func writeAggregateOutputVariables(writer provider.OutputWriter, vars map[string]string) error {
	keys := make([]string, 0, len(vars))
	for key := range vars {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	writeErrs := make([]error, 0)
	for _, key := range keys {
		if err := writer.WriteOutput(key, vars[key]); err != nil {
			writeErrs = append(writeErrs, fmt.Errorf("%w: failed to write aggregate CI output %q: %w", errUtils.ErrCIOutputWriteFailed, key, err))
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
func (p *Plugin) uploadAggregatePlanfiles(ctx *plugin.HookContext, aggregate *terraformPlanAggregate) error {
	for i := range aggregate.Components {
		component := &aggregate.Components[i]
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
func (p *Plugin) updateAggregateCheckRuns(ctx *plugin.HookContext, aggregate *terraformPlanAggregate) {
	for i := range aggregate.Components {
		component := &aggregate.Components[i]
		componentCtx := p.aggregateComponentContext(ctx, component)
		if component.Skipped {
			componentCtx.CommandError = aggregateCommandError(component.Summary)
			componentCtx.ExitCode = 1
		}
		if err := p.updateCheckRun(componentCtx, component.Parsed); err != nil {
			logCheckRunError("CI aggregate check run update skipped", err)
		}
	}
}

// aggregateComponentContext builds a component-scoped hook context from aggregate data.
func (p *Plugin) aggregateComponentContext(ctx *plugin.HookContext, component *terraformPlanAggregateComponent) *plugin.HookContext {
	info := schema.ConfigAndStacksInfo{}
	if ctx.Info != nil {
		info = *ctx.Info
	}
	info.Stack = component.Result.Stack
	info.StackFromArg = component.Result.Stack
	info.Component = component.Result.Component
	info.ComponentFromArg = component.Result.Component
	command := aggregateCommandPlan
	if ctx != nil {
		command = normalizeAggregateCommand(ctx.Command)
	}

	return &plugin.HookContext{
		Event:               ctx.Event,
		Command:             command,
		EventPrefix:         ctx.EventPrefix,
		Config:              ctx.Config,
		Info:                &info,
		Output:              component.Result.Output,
		CommandError:        aggregateCommandError(component.Result.Error),
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
