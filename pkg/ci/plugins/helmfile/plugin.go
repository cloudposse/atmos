// Package helmfile provides the CI Plugin implementation for Helmfile components.
package helmfile

import (
	"embed"
	"fmt"
	"strings"

	ci "github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:embed templates/*.md
var defaultTemplates embed.FS

// Plugin implements plugin.Plugin for Helmfile components.
type Plugin struct{}

var _ plugin.Plugin = (*Plugin)(nil)

func init() {
	if err := ci.RegisterPlugin(&Plugin{}); err != nil {
		panic(fmt.Sprintf("failed to register helmfile CI plugin: %v", err))
	}
}

// GetType returns the component type.
func (p *Plugin) GetType() string {
	defer perf.Track(nil, "helmfileci.Plugin.GetType")()
	return "helmfile"
}

// GetHookBindings returns the hook bindings for Helmfile CI summaries.
func (p *Plugin) GetHookBindings() []plugin.HookBinding {
	defer perf.Track(nil, "helmfileci.Plugin.GetHookBindings")()

	return []plugin.HookBinding{
		{Event: "after.helmfile.template", Handler: p.onAfterOperation},
		{Event: "after.helmfile.diff", Handler: p.onAfterOperation},
		{Event: "after.helmfile.apply", Handler: p.onAfterOperation},
		{Event: "after.helmfile.sync", Handler: p.onAfterOperation},
		{Event: "after.helmfile.deploy", Handler: p.onAfterOperation},
		{Event: "after.helmfile.destroy", Handler: p.onAfterOperation},
	}
}

func (p *Plugin) onAfterOperation(ctx *plugin.HookContext) error {
	defer perf.Track(ctx.Config, "helmfileci.Plugin.onAfterOperation")()

	if !isSummaryEnabled(ctx.Config) {
		return nil
	}

	writer := ctx.Provider.OutputWriter()
	if writer == nil {
		return nil
	}

	tmplCtx := p.buildTemplateContext(ctx)
	templateName := helmfileTemplateName(ctx.Command)
	if ctx.Config != nil && ctx.Config.CI.Summary.Template != "" {
		templateName = ctx.Config.CI.Summary.Template
	}
	if templateName == "" {
		return nil
	}

	rendered, err := ctx.TemplateLoader.LoadAndRender("helmfile", templateName, defaultTemplates, tmplCtx)
	if err != nil {
		return err
	}
	return writer.WriteSummary(rendered)
}

func (p *Plugin) buildTemplateContext(ctx *plugin.HookContext) *TemplateContext {
	defer perf.Track(ctx.Config, "helmfileci.Plugin.buildTemplateContext")()

	component, stack := "", ""
	if ctx.Info != nil {
		component = ctx.Info.ComponentFromArg
		stack = ctx.Info.Stack
	}

	result := &plugin.OutputResult{
		ExitCode:  ctx.ExitCode,
		HasErrors: ctx.ExitCode != 0 || ctx.CommandError != nil,
	}
	if ctx.CommandError != nil {
		result.Errors = []string{ctx.CommandError.Error()}
	}

	base := &plugin.TemplateContext{
		Component:     component,
		ComponentType: "helmfile",
		Stack:         stack,
		Command:       ctx.Command,
		CI:            ctx.CICtx,
		Result:        result,
		Output:        strings.TrimSpace(ctx.Output),
		Custom:        map[string]any{},
	}

	return &TemplateContext{
		TemplateContext: base,
		CommandTitle:    title(ctx.Command),
	}
}

// TemplateContext extends the base template context with Helmfile-specific fields.
type TemplateContext struct {
	*plugin.TemplateContext

	CommandTitle string
}

func helmfileTemplateName(command string) string {
	switch command {
	case "sync", "deploy":
		return "apply"
	default:
		return command
	}
}

func isSummaryEnabled(cfg *schema.AtmosConfiguration) bool {
	if cfg == nil {
		return true
	}
	if cfg.CI.Summary.Enabled == nil {
		return true
	}
	return *cfg.CI.Summary.Enabled
}

func title(command string) string {
	if command == "" {
		return "Helmfile"
	}
	return strings.ToUpper(command[:1]) + command[1:]
}
