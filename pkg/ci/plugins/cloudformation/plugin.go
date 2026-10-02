// Package cloudformation provides CI job summaries for native CloudFormation components.
package cloudformation

import (
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	ci "github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:embed templates/*.md
var defaultTemplates embed.FS

// Plugin implements plugin.Plugin for CloudFormation components.
type Plugin struct{}

var _ plugin.Plugin = (*Plugin)(nil)

func init() {
	if err := ci.RegisterPlugin(&Plugin{}); err != nil {
		panic(fmt.Sprintf("failed to register cloudformation CI plugin: %v", err))
	}
}

// GetType returns the component type.
func (p *Plugin) GetType() string {
	defer perf.Track(nil, "cloudformationci.Plugin.GetType")()
	return cfg.CloudFormationComponentType
}

// GetHookBindings returns the CloudFormation CI summary hook bindings.
func (p *Plugin) GetHookBindings() []plugin.HookBinding {
	defer perf.Track(nil, "cloudformationci.Plugin.GetHookBindings")()

	return []plugin.HookBinding{
		{Event: "after.aws/cloudformation.diff", Handler: p.onAfterOperation},
		{Event: "after.aws/cloudformation.apply", Handler: p.onAfterOperation},
		{Event: "after.aws/cloudformation.delete", Handler: p.onAfterOperation},
		{Event: "after.aws/cloudformation.drift-detect", Handler: p.onAfterOperation},
		{Event: "after.aws/cloudformation.drift-describe", Handler: p.onAfterOperation},
	}
}

// onAfterOperation renders and writes the CI job summary and the CI output
// variables for one CloudFormation operation. Errors here are returned (not
// swallowed) so RunCIHooks can log them — but a summary failure never
// propagates back into the underlying CloudFormation command result, which has
// already completed by this point. Output variables are warn-only, like the
// terraform plugin: a failure writing them never hides the summary's error.
func (p *Plugin) onAfterOperation(ctx *plugin.HookContext) error {
	defer perf.Track(ctx.Config, "cloudformationci.Plugin.onAfterOperation")()

	// Summary rendering/writes and output variables are independent channels.
	// Preserve the summary error for diagnostics, but always attempt outputs.
	summaryErr := p.writeSummary(ctx)
	if isOutputEnabled(ctx.Config) {
		if err := p.writeOutputs(ctx); err != nil {
			log.Warn("CloudFormation CI output failed", "error", err)
		}
	}
	return summaryErr
}

// writeSummary renders and writes the job summary for one operation.
func (p *Plugin) writeSummary(ctx *plugin.HookContext) error {
	if !isSummaryEnabled(ctx.Config) {
		return nil
	}

	writer := ctx.Provider.OutputWriter()
	if writer == nil {
		log.Debug("CI platform does not support summaries")
		return nil
	}

	templateName := ctx.Command
	if ctx.Config != nil && ctx.Config.CI.Summary.Template != "" {
		templateName = ctx.Config.CI.Summary.Template
	}
	if templateName == "" {
		return nil
	}

	tmplCtx := p.buildTemplateContext(ctx)
	rendered, err := ctx.TemplateLoader.LoadAndRender("cloudformation", templateName, defaultTemplates, tmplCtx)
	if err != nil {
		return errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithExplanation("Failed to render CloudFormation CI summary").
			WithContext("template", templateName).
			Err()
	}

	if err := writer.WriteSummary(rendered); err != nil {
		return errUtils.Build(errUtils.ErrCISummaryWriteFailed).
			WithCause(err).
			WithExplanation("Failed to write CloudFormation CI summary").
			Err()
	}
	return nil
}

// writeOutputs writes the CI output variables (for example GITHUB_OUTPUT) for
// one operation: has_changes, changeset_name, stack_status, drifted, and the
// stack Outputs after apply. Stack outputs come from the masked set the
// `output` verb presents, so NoEcho values are never exported in clear text.
func (p *Plugin) writeOutputs(ctx *plugin.HookContext) error {
	defer perf.Track(ctx.Config, "cloudformationci.Plugin.writeOutputs")()

	writer := ctx.Provider.OutputWriter()
	if writer == nil {
		log.Debug("CI platform does not support outputs")
		return nil
	}

	vars := outputVariables(ctx, aggregateResult(ctx))
	if ctx.Config != nil && len(ctx.Config.CI.Output.Variables) > 0 {
		vars = filterVariables(vars, ctx.Config.CI.Output.Variables)
	}
	// Stack outputs bypass the whitelist, as terraform outputs do: they are the
	// operation's payload, not bookkeeping variables.
	for key, value := range aggregateResult(ctx).Outputs {
		vars["output_"+key] = value
	}

	keys := make([]string, 0, len(vars))
	for key := range vars {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := writer.WriteOutput(key, vars[key]); err != nil {
			log.Warn("Failed to write CI output", "key", key, "error", err)
		}
	}
	return nil
}

// outputVariables builds the bookkeeping output variables for an operation.
// The has_changes variable appears for diff/apply, drifted for the drift verbs,
// and stack_status when the operation observed a final status.
func outputVariables(ctx *plugin.HookContext, result *schema.CloudFormationCIResult) map[string]string {
	vars := map[string]string{
		"stack":     result.Stack,
		"component": result.Component,
		"command":   verbName(ctx.Command, result.Command),
		"exit_code": strconv.Itoa(ctx.ExitCode),
	}
	if vars["stack"] == "" && ctx.Info != nil {
		vars["stack"] = ctx.Info.Stack
	}
	if vars["component"] == "" && ctx.Info != nil {
		vars["component"] = ctx.Info.ComponentFromArg
	}
	setIfNotEmpty(vars, "stack_name", result.StackName)
	setIfNotEmpty(vars, "changeset_name", result.ChangeSetName)
	setIfNotEmpty(vars, "stack_status", result.StackStatus)

	switch ctx.Command {
	case "diff", "apply":
		vars["has_changes"] = strconv.FormatBool(result.HasChanges)
	case "drift-detect", "drift-describe":
		vars["drifted"] = strconv.FormatBool(result.DriftStatus == "DRIFTED")
		setIfNotEmpty(vars, "drift_status", result.DriftStatus)
		vars["drifted_resource_count"] = strconv.Itoa(result.DriftedCount)
	}
	return vars
}

// setIfNotEmpty sets vars[key] only for a non-empty value.
func setIfNotEmpty(vars map[string]string, key, value string) {
	if value != "" {
		vars[key] = value
	}
}

// filterVariables keeps only the whitelisted variables.
func filterVariables(vars map[string]string, allowed []string) map[string]string {
	allowedSet := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = true
	}
	filtered := make(map[string]string, len(vars))
	for key, value := range vars {
		if allowedSet[key] {
			filtered[key] = value
		}
	}
	return filtered
}

// isOutputEnabled defaults output variables to enabled unless configuration explicitly disables them.
func isOutputEnabled(atmosConfig *schema.AtmosConfiguration) bool {
	if atmosConfig == nil || atmosConfig.CI.Output.Enabled == nil {
		return true
	}
	return *atmosConfig.CI.Output.Enabled
}

// verbAliases lists the CLI verbs that dispatch as a given hook command: `plan` fires the diff
// hooks and `deploy` fires the apply hooks, so a summary titled after the hook command alone
// would call a plan a "diff".
var verbAliases = map[string][]string{
	"diff":  {"plan"},
	"apply": {"deploy"},
}

// verbName returns the verb the user invoked: the aggregate's recorded CLI verb when it is the
// hook command itself or one of its aliases, otherwise the hook command.
func verbName(hookCommand, invoked string) string {
	if invoked == hookCommand {
		return hookCommand
	}
	for _, alias := range verbAliases[hookCommand] {
		if invoked == alias {
			return alias
		}
	}
	return hookCommand
}

// verbTitle capitalizes each word of a hyphenated verb ("drift-detect" becomes "Drift Detect").
func verbTitle(verb string) string {
	words := strings.Split(verb, "-")
	for i, word := range words {
		if word != "" {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

// buildTemplateContext assembles the CloudFormation-specific template context
// from the hook context's aggregate result (populated by
// pkg/component/aws/cloudformation's runCIHook).
func (p *Plugin) buildTemplateContext(ctx *plugin.HookContext) *TemplateContext {
	defer perf.Track(ctx.Config, "cloudformationci.Plugin.buildTemplateContext")()

	result := aggregateResult(ctx)

	component, stack := result.Component, result.Stack
	if ctx.Info != nil {
		if component == "" {
			component = ctx.Info.ComponentFromArg
		}
		if stack == "" {
			stack = ctx.Info.Stack
		}
	}

	outputResult := &plugin.OutputResult{
		ExitCode:  ctx.ExitCode,
		HasErrors: ctx.ExitCode != 0 || ctx.CommandError != nil || result.Error != "",
	}
	switch {
	case ctx.CommandError != nil:
		outputResult.Errors = []string{ctx.CommandError.Error()}
	case result.Error != "":
		outputResult.Errors = []string{result.Error}
	}

	base := &plugin.TemplateContext{
		Component:     component,
		ComponentType: cfg.CloudFormationComponentType,
		Stack:         stack,
		Command:       ctx.Command,
		CI:            ctx.CICtx,
		Result:        outputResult,
		Output:        plugin.TruncateDetail(ctx.Output),
		Custom:        map[string]any{},
	}

	verb := verbName(ctx.Command, result.Command)
	return &TemplateContext{
		TemplateContext: base,
		Verb:            verb,
		VerbTitle:       verbTitle(verb),
		StackName:       result.StackName,
		ChangeSetName:   result.ChangeSetName,
		ResourceChanges: result.ResourceChanges,
		NoOp:            result.NoOp,
		HasChanges:      result.HasChanges,
		FailOnDrift:     result.FailOnDrift,
		StackStatus:     result.StackStatus,
		DriftStatus:     result.DriftStatus,
		DriftedCount:    result.DriftedCount,
	}
}

// aggregateResult normalizes ctx.Aggregate to a *schema.CloudFormationCIResult,
// treating a missing or mistyped aggregate as an empty result rather than panicking.
func aggregateResult(ctx *plugin.HookContext) *schema.CloudFormationCIResult {
	if result, ok := ctx.Aggregate.(*schema.CloudFormationCIResult); ok && result != nil {
		return result
	}
	if result, ok := ctx.Aggregate.(schema.CloudFormationCIResult); ok {
		return &result
	}
	return &schema.CloudFormationCIResult{}
}

// TemplateContext extends the base template context with CloudFormation-specific fields.
type TemplateContext struct {
	*plugin.TemplateContext

	// Verb is the CLI verb the user invoked (for example "plan" for the diff hooks), and
	// VerbTitle is its title-cased form. Summaries use them so a plan is not called a diff.
	Verb      string
	VerbTitle string

	StackName       string
	ChangeSetName   string
	ResourceChanges int
	// NoOp is true when the changeset reported no changes.
	NoOp bool
	// HasChanges is true when a diff/apply changeset contains changes.
	HasChanges bool
	// FailOnDrift is true when drift detect ran with --fail-on-drift.
	FailOnDrift bool
	StackStatus string
	DriftStatus string

	DriftedCount int
}

// isSummaryEnabled defaults summaries to enabled unless configuration explicitly disables them.
func isSummaryEnabled(atmosConfig *schema.AtmosConfiguration) bool {
	if atmosConfig == nil {
		return true
	}
	if atmosConfig.CI.Summary.Enabled == nil {
		return true
	}
	return *atmosConfig.CI.Summary.Enabled
}
