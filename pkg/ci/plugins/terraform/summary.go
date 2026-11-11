package terraform

import (
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
)

// summaryTemplateName prefers the configured override over the command name.
func summaryTemplateName(ctx *plugin.HookContext) string {
	if ctx.Config != nil && ctx.Config.CI.Summary.Template != "" {
		return ctx.Config.CI.Summary.Template
	}
	return ctx.Command
}

// renderSummary renders the normal component summary without provider writes.
// The enriched result preserves command errors and scheduler outcomes.
func (p *Plugin) renderSummary(ctx *plugin.HookContext, result *plugin.OutputResult) (string, error) {
	templateName := summaryTemplateName(ctx)
	if templateName == "" {
		return "", nil
	}

	// Build template context.
	tmplCtx, err := p.buildTemplateContext(ctx.Info, ctx.CICtx, ctx.Output, ctx.Command, result)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithExplanation("Failed to build template context").
			Err()
	}

	// Load and render template.
	rendered, err := ctx.TemplateLoader.LoadAndRender(
		"terraform",
		templateName,
		defaultTemplates,
		tmplCtx,
	)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithExplanation("Failed to render template").
			WithContext("template", templateName).
			Err()
	}

	return rendered, nil
}
