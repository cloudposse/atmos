// Package terraform provides the CI Plugin implementation for Terraform.
package terraform

import (
	"embed"
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
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
		{
			Event:   "before.terraform.deploy",
			Handler: p.onBeforeDeploy,
		},
		{
			Event:   "after.terraform.deploy",
			Handler: p.onAfterDeploy,
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
		Output:        cleanOutput(output, command),
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
func (p *Plugin) getOutputVariables(result *plugin.OutputResult, command string) map[string]string {
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

	// Add success indicator for apply commands.
	if command == "apply" {
		vars["success"] = strconv.FormatBool(!result.HasErrors)
	}

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

// getArtifactKey generates the artifact storage key for a command using the KeyPattern.
// Returns an error if required fields (Stack, Component, SHA) are empty.
func (p *Plugin) getArtifactKey(info *schema.ConfigAndStacksInfo, ciCtx *provider.Context) (string, error) {
	defer perf.Track(nil, "terraform.Plugin.getArtifactKey")()

	pattern := planfile.DefaultKeyPattern()
	// TODO: override from config if set via components.terraform.planfiles.key_pattern.

	keyCtx := &planfile.KeyContext{}
	if info != nil {
		keyCtx.Stack = info.Stack
		keyCtx.Component = info.ComponentFromArg
		keyCtx.ComponentPath = info.ComponentFolderPrefix
	}
	if ciCtx != nil {
		keyCtx.SHA = ciCtx.SHA
		keyCtx.Branch = ciCtx.Branch
		if ciCtx.PullRequest != nil {
			keyCtx.PRNumber = ciCtx.PullRequest.Number
		}
		keyCtx.RunID = ciCtx.RunID
	}

	return pattern.GenerateKey(keyCtx)
}

// planOutputMarkers are searched in order to find where the meaningful plan output starts.
// Everything before the first match is stripped (data source reads, state refreshes, etc.).
var planOutputMarkers = []string{
	"Terraform will perform the following actions:",
}

// noChangesMarker identifies output where terraform found no differences.
const noChangesMarker = "No changes."

// cleanOutput strips noisy preamble from terraform output based on command type.
// For plan: strips data source reads and state refreshes, returns empty for no-changes.
// For apply: returns full output (the apply result is always meaningful).
func cleanOutput(output, command string) string {
	if command == "apply" {
		return cleanApplyOutput(output)
	}
	return cleanPlanOutput(output)
}

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

// applyOutputMarker identifies where the meaningful apply output starts.
const applyOutputMarker = "Apply complete!"

// cleanApplyOutput strips noisy preamble from terraform apply output,
// keeping the apply result summary and outputs.
func cleanApplyOutput(output string) string {
	if idx := strings.Index(output, applyOutputMarker); idx >= 0 {
		return output[idx:]
	}
	// If no marker found, return full output (might be error output).
	return output
}
