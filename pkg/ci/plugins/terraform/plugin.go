// Package terraform provides the CI Plugin implementation for Terraform.
package terraform

import (
	"embed"
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"

	ci "github.com/cloudposse/atmos/pkg/ci"
)

//go:embed templates/*.md
var defaultTemplates embed.FS

// Plugin implements plugin.Plugin for Terraform.
type Plugin struct{}

// Ensure Plugin implements plugin.Plugin.
var _ plugin.Plugin = (*Plugin)(nil)

func init() {
	// Self-register on package import.
	if err := ci.RegisterPlugin(&Plugin{}); err != nil {
		// Panic on registration failure - this is a programming error.
		panic(fmt.Sprintf("failed to register terraform CI plugin: %v", err))
	}
}

// GetType returns the component type.
func (p *Plugin) GetType() string {
	return "terraform"
}

// GetHookBindings returns the hook bindings for Terraform CI integration.
// Each binding has a Handler callback that owns all action logic for the event.
func (p *Plugin) GetHookBindings() []plugin.HookBinding {
	defer perf.Track(nil, "terraform.Plugin.GetHookBindings")()

	return []plugin.HookBinding{
		{
			Event:   "before.terraform.plan",
			Handler: p.onBeforePlan,
		},
		{
			Event:   "after.terraform.plan",
			Handler: p.onAfterPlan,
		},
		{
			Event:   "before.terraform.apply",
			Handler: p.onBeforeApply,
		},
		{
			Event:   "after.terraform.apply",
			Handler: p.onAfterApply,
		},
	}
}

// buildTemplateContext creates a TerraformTemplateContext from execution results.
// Returns an extended context with terraform-specific fields for template rendering.
func (p *Plugin) buildTemplateContext(
	info *schema.ConfigAndStacksInfo,
	ciCtx *provider.Context,
	output string,
	command string,
) (any, error) {
	defer perf.Track(nil, "terraform.Plugin.buildTemplateContext")()

	// Parse the output to get structured data.
	result := ParseOutput(output, command)

	// Build base context.
	baseCtx := &plugin.TemplateContext{
		Component:     info.ComponentFromArg,
		ComponentType: "terraform",
		Stack:         info.Stack,
		Command:       command,
		CI:            ciCtx,
		Result:        result,
		Output:        cleanPlanOutput(output),
		Custom:        make(map[string]any),
	}

	// Extract terraform-specific data.
	var tfData *plugin.TerraformOutputData
	if result != nil && result.Data != nil {
		tfData, _ = result.Data.(*plugin.TerraformOutputData)
	}

	// Return extended context with terraform-specific fields.
	return NewTemplateContext(baseCtx, tfData), nil
}

// getOutputVariables returns CI output variables for a command.
func (p *Plugin) getOutputVariables(result *plugin.OutputResult, _ string) map[string]string {
	defer perf.Track(nil, "terraform.Plugin.getOutputVariables")()

	vars := make(map[string]string)

	// Return empty vars if result is nil.
	if result == nil {
		return vars
	}

	// Common outputs.
	vars["has_changes"] = strconv.FormatBool(result.HasChanges)
	vars["has_errors"] = strconv.FormatBool(result.HasErrors)
	vars["exit_code"] = strconv.Itoa(result.ExitCode)

	// Terraform-specific outputs.
	if result.Data != nil {
		if data, ok := result.Data.(*plugin.TerraformOutputData); ok {
			vars["resources_to_create"] = strconv.Itoa(data.ResourceCounts.Create)
			vars["resources_to_change"] = strconv.Itoa(data.ResourceCounts.Change)
			vars["resources_to_replace"] = strconv.Itoa(data.ResourceCounts.Replace)
			vars["resources_to_destroy"] = strconv.Itoa(data.ResourceCounts.Destroy)
		}
	}

	return vars
}

// getArtifactKey generates the artifact storage key for a command.
func (p *Plugin) getArtifactKey(info *schema.ConfigAndStacksInfo, _ string) string {
	defer perf.Track(nil, "terraform.Plugin.getArtifactKey")()

	// Validate required fields.
	if info == nil {
		log.Warn("getArtifactKey called with nil info, using placeholder key")
		return "unknown/unknown.tfplan"
	}

	stack := info.Stack
	component := info.ComponentFromArg
	if stack == "" {
		log.Warn("getArtifactKey called with empty Stack", "component", component)
		stack = "unknown"
	}
	if component == "" {
		log.Warn("getArtifactKey called with empty ComponentFromArg", "stack", stack)
		component = "unknown"
	}

	// Default pattern: stack/component.tfplan
	return fmt.Sprintf("%s/%s.tfplan", stack, component)
}

// planOutputMarkers are searched in order to find where the meaningful plan output starts.
// Everything before the first match is stripped (data source reads, state refreshes, etc.).
var planOutputMarkers = []string{
	"Terraform will perform the following actions:",
}

// noChangesMarker identifies output where terraform found no differences.
const noChangesMarker = "No changes."

// cleanPlanOutput strips noisy preamble (data source reads, state refreshes)
// from terraform plan output, keeping only the plan itself.
// Returns empty string for no-changes output (nothing useful to display).
func cleanPlanOutput(output string) string {
	// No-changes output has no plan to display.
	if strings.Contains(output, noChangesMarker) {
		return ""
	}

	for _, marker := range planOutputMarkers {
		if idx := strings.Index(output, marker); idx > 0 {
			return output[idx+len(marker):]
		}
	}
	return output
}
