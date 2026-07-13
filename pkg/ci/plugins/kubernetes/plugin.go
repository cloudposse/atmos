// Package kubernetes provides CI job summaries for native Kubernetes components.
package kubernetes

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	ci "github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:embed templates/*.md
var defaultTemplates embed.FS

// Plugin implements plugin.Plugin for Kubernetes CI summaries.
type Plugin struct{}

var _ plugin.Plugin = (*Plugin)(nil)

func init() {
	if err := ci.RegisterPlugin(&Plugin{}); err != nil {
		panic(fmt.Sprintf("failed to register kubernetes CI plugin: %v", err))
	}
}

// GetType returns the component type.
func (p *Plugin) GetType() string {
	defer perf.Track(nil, "kubernetes.Plugin.GetType")()
	return "kubernetes"
}

// GetHookBindings returns the Kubernetes CI summary hook bindings.
func (p *Plugin) GetHookBindings() []plugin.HookBinding {
	defer perf.Track(nil, "kubernetes.Plugin.GetHookBindings")()

	return []plugin.HookBinding{
		{Event: "after.kubernetes.render", Handler: p.onAfterCommand},
		{Event: "after.kubernetes.plan", Handler: p.onAfterCommand},
		{Event: "after.kubernetes.diff", Handler: p.onAfterCommand},
		{Event: "after.kubernetes.apply", Handler: p.onAfterCommand},
		{Event: "after.kubernetes.deploy", Handler: p.onAfterCommand},
		{Event: "after.kubernetes.delete", Handler: p.onAfterCommand},
		{Event: "after.kubernetes.validate", Handler: p.onAfterCommand},
	}
}

func (p *Plugin) onAfterCommand(ctx *plugin.HookContext) error {
	defer perf.Track(ctx.Config, "kubernetes.Plugin.onAfterCommand")()

	if !isSummaryEnabled(ctx.Config) {
		return nil
	}

	writer := ctx.Provider.OutputWriter()
	if writer == nil {
		log.Debug("CI platform does not support summaries")
		return nil
	}

	rendered, err := p.renderSummary(ctx)
	if err != nil {
		return err
	}
	if rendered == "" {
		return nil
	}
	if err := writer.WriteSummary(rendered); err != nil {
		return errUtils.Build(errUtils.ErrCISummaryWriteFailed).
			WithCause(err).
			WithExplanation("Failed to write Kubernetes CI summary").
			Err()
	}
	return nil
}

func (p *Plugin) renderSummary(ctx *plugin.HookContext) (string, error) {
	result := normalizeResult(ctx)
	tmplCtx := newTemplateContext(result)

	templateName := "summary"
	if ctx.Config != nil && ctx.Config.CI.Summary.Template != "" {
		templateName = ctx.Config.CI.Summary.Template
	}

	rendered, err := ctx.TemplateLoader.LoadAndRender("kubernetes", templateName, defaultTemplates, tmplCtx)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithExplanation("Failed to render Kubernetes CI summary").
			WithContext("template", templateName).
			Err()
	}
	return rendered, nil
}

func normalizeResult(ctx *plugin.HookContext) *schema.KubernetesCIResult {
	if result, ok := ctx.Aggregate.(*schema.KubernetesCIResult); ok && result != nil {
		return result
	}
	if result, ok := ctx.Aggregate.(schema.KubernetesCIResult); ok {
		return &result
	}

	result := &schema.KubernetesCIResult{
		Command:      ctx.Command,
		ExitCode:     ctx.ExitCode,
		ActionCounts: map[string]int{},
	}
	if ctx.Info != nil {
		result.Stack = ctx.Info.Stack
		result.Component = ctx.Info.ComponentFromArg
	}
	if ctx.CommandError != nil {
		result.Error = ctx.CommandError.Error()
	}
	return result
}

type templateContext struct {
	*schema.KubernetesCIResult
	Status string
	Counts []actionCount
	// Diff is the aggregated, truncated unified diff across all changed objects,
	// rendered into a single collapsible details block (mirrors the Terraform
	// plugin's .Output). Empty when no object carries a diff.
	Diff string
}

type actionCount struct {
	Action string
	Count  int
}

func newTemplateContext(result *schema.KubernetesCIResult) templateContext {
	counts := sortedActionCounts(result.ActionCounts)
	return templateContext{
		KubernetesCIResult: result,
		Status:             summaryStatus(result),
		Counts:             counts,
		Diff:               aggregateDiff(result),
	}
}

// aggregateDiff concatenates each changed object's unified diff under a
// `# <Resource> <ns>/<name>` header, then caps the total via the shared
// truncation helper so the CI job summary stays within platform limits.
func aggregateDiff(result *schema.KubernetesCIResult) string {
	var b strings.Builder
	for _, action := range result.Actions {
		if action.Diff == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "# %s %s\n", action.Resource, objectRef(action.Namespace, action.Name))
		b.WriteString(action.Diff)
		if !strings.HasSuffix(action.Diff, "\n") {
			b.WriteString("\n")
		}
	}
	return plugin.TruncateDetail(b.String())
}

// objectRef formats a namespaced object reference, omitting the namespace for
// cluster-scoped objects.
func objectRef(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "/" + name
}

func sortedActionCounts(counts map[string]int) []actionCount {
	if len(counts) == 0 {
		return nil
	}
	actions := make([]string, 0, len(counts))
	for action := range counts {
		actions = append(actions, action)
	}
	sort.Strings(actions)

	result := make([]actionCount, 0, len(actions))
	for _, action := range actions {
		result = append(result, actionCount{Action: action, Count: counts[action]})
	}
	return result
}

func summaryStatus(result *schema.KubernetesCIResult) string {
	if result.Error != "" || result.ExitCode != 0 {
		return "failed"
	}
	if result.ActionCounts["create"] > 0 || result.ActionCounts["changed"] > 0 {
		return "changed"
	}
	if result.ActionCounts["no-change"] > 0 && len(result.ActionCounts) == 1 {
		return "no changes"
	}
	return "succeeded"
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
