// Package cloudformation provides CI job summaries for native CloudFormation components.
package cloudformation

import (
	"embed"
	"fmt"
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

// onAfterOperation renders and writes the CI job summary for one CloudFormation
// operation. Errors here are returned (not swallowed) so RunCIHooks can log
// them — but a summary failure never propagates back into the underlying
// CloudFormation command result, which has already completed by this point.
func (p *Plugin) onAfterOperation(ctx *plugin.HookContext) error {
	defer perf.Track(ctx.Config, "cloudformationci.Plugin.onAfterOperation")()

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
		Output:        strings.TrimSpace(ctx.Output),
		Custom:        map[string]any{},
	}

	return &TemplateContext{
		TemplateContext: base,
		StackName:       result.StackName,
		ChangeSetName:   result.ChangeSetName,
		ResourceChanges: result.ResourceChanges,
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

	StackName       string
	ChangeSetName   string
	ResourceChanges int
	DriftStatus     string
	DriftedCount    int
}

func isSummaryEnabled(atmosConfig *schema.AtmosConfiguration) bool {
	if atmosConfig == nil {
		return true
	}
	if atmosConfig.CI.Summary.Enabled == nil {
		return true
	}
	return *atmosConfig.CI.Summary.Enabled
}
