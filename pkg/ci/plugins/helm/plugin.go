// Package helm provides the CI Plugin implementation for native Helm components.
package helm

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	ci "github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:embed templates/*.md
var defaultTemplates embed.FS

// Plugin implements plugin.Plugin for native Helm components.
type Plugin struct{}

var _ plugin.Plugin = (*Plugin)(nil)

func init() {
	if err := ci.RegisterPlugin(&Plugin{}); err != nil {
		panic(fmt.Sprintf("failed to register helm CI plugin: %v", err))
	}
}

// GetType returns the component type.
func (p *Plugin) GetType() string {
	defer perf.Track(nil, "helmci.Plugin.GetType")()
	return "helm"
}

// GetHookBindings returns the hook bindings for Helm CI summaries.
func (p *Plugin) GetHookBindings() []plugin.HookBinding {
	defer perf.Track(nil, "helmci.Plugin.GetHookBindings")()

	return []plugin.HookBinding{
		{Event: "after.helm.template", Handler: p.onAfterOperation},
		{Event: "after.helm.diff", Handler: p.onAfterOperation},
		{Event: "after.helm.apply", Handler: p.onAfterOperation},
		{Event: "after.helm.deploy", Handler: p.onAfterOperation},
		{Event: "after.helm.delete", Handler: p.onAfterOperation},
	}
}

func (p *Plugin) onAfterOperation(ctx *plugin.HookContext) error {
	defer perf.Track(ctx.Config, "helmci.Plugin.onAfterOperation")()

	if !isSummaryEnabled(ctx.Config) {
		return nil
	}

	writer := ctx.Provider.OutputWriter()
	if writer == nil {
		return nil
	}

	tmplCtx := p.buildTemplateContext(ctx)
	templateName := helmTemplateName(ctx.Command)
	if ctx.Config != nil && ctx.Config.CI.Summary.Template != "" {
		templateName = ctx.Config.CI.Summary.Template
	}
	if templateName == "" {
		return nil
	}

	rendered, err := ctx.TemplateLoader.LoadAndRender("helm", templateName, defaultTemplates, tmplCtx)
	if err != nil {
		return err
	}
	return writer.WriteSummary(rendered)
}

func (p *Plugin) buildTemplateContext(ctx *plugin.HookContext) *TemplateContext {
	defer perf.Track(ctx.Config, "helmci.Plugin.buildTemplateContext")()

	data := normalizeSummary(ctx.Aggregate)
	if data.Command == "" {
		data.Command = ctx.Command
	}
	if data.Component == "" && ctx.Info != nil {
		data.Component = ctx.Info.ComponentFromArg
	}
	if data.Stack == "" && ctx.Info != nil {
		data.Stack = ctx.Info.Stack
	}

	result := &plugin.OutputResult{
		ExitCode:  ctx.ExitCode,
		HasErrors: ctx.ExitCode != 0 || ctx.CommandError != nil,
		Data:      data,
	}
	if ctx.CommandError != nil {
		result.Errors = []string{ctx.CommandError.Error()}
	}

	base := &plugin.TemplateContext{
		Component:     data.Component,
		ComponentType: "helm",
		Stack:         data.Stack,
		Command:       data.Command,
		CI:            ctx.CICtx,
		Result:        result,
		Output:        ctx.Output,
		Custom:        map[string]any{},
	}

	return &TemplateContext{
		TemplateContext: base,
		CommandTitle:    title(data.Command),
		Chart:           data.Chart,
		ReleaseName:     data.ReleaseName,
		Namespace:       data.Namespace,
		Target:          data.Target,
		ObjectCount:     data.ObjectCount,
		ObjectKinds:     data.ObjectKinds,
		ManifestBytes:   data.ManifestBytes,
		Message:         data.Message,
		Diff:            plugin.TruncateDetail(data.Diff),
	}
}

// TemplateContext extends the base template context with Helm-specific fields.
type TemplateContext struct {
	*plugin.TemplateContext

	CommandTitle  string
	Chart         string
	ReleaseName   string
	Namespace     string
	Target        string
	ObjectCount   int
	ObjectKinds   []string
	ManifestBytes int
	Message       string
	// Diff is the unified diff produced by `helm diff`/`plan` (empty otherwise).
	Diff string
}

// Summary is the structured payload native Helm execution passes to the CI plugin.
type Summary struct {
	Component     string
	Stack         string
	Command       string
	Chart         string
	ReleaseName   string
	Namespace     string
	Target        string
	ObjectCount   int
	ObjectKinds   []string
	ManifestBytes int
	Message       string
	// Diff is the unified diff produced by `helm diff`/`plan` (empty otherwise).
	Diff string
}

func normalizeSummary(value any) Summary {
	switch v := value.(type) {
	case Summary:
		return v
	case *Summary:
		if v == nil {
			return Summary{}
		}
		return *v
	case map[string]any:
		return summaryFromMap(v)
	default:
		return Summary{}
	}
}

func summaryFromMap(m map[string]any) Summary {
	s := Summary{
		Component:     stringValue(m["component"]),
		Stack:         stringValue(m["stack"]),
		Command:       stringValue(m["command"]),
		Chart:         stringValue(m["chart"]),
		ReleaseName:   stringValue(m["release_name"]),
		Namespace:     stringValue(m["namespace"]),
		Target:        stringValue(m["target"]),
		ObjectCount:   intValue(m["object_count"]),
		ObjectKinds:   stringSliceValue(m["object_kinds"]),
		ManifestBytes: intValue(m["manifest_bytes"]),
		Message:       stringValue(m["message"]),
		Diff:          stringValue(m["diff"]),
	}
	sort.Strings(s.ObjectKinds)
	return s
}

func helmTemplateName(command string) string {
	switch command {
	case "render":
		return "template"
	case "plan":
		return "diff"
	case "deploy":
		return "apply"
	case "destroy":
		return "delete"
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

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", value)
}

func intValue(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func stringSliceValue(value any) []string {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s := stringValue(item); s != "" {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

func title(command string) string {
	if command == "" {
		return "Helm"
	}
	return strings.ToUpper(command[:1]) + command[1:]
}
