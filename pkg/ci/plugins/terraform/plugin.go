// Package terraform provides the CI Plugin implementation for Terraform.
package terraform

import (
	"embed"
	"fmt"
	"strconv"

	e "github.com/cloudposse/atmos/internal/exec"
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

// Ensure Plugin implements Plugin and ComponentConfigurationResolver.
var (
	_ plugin.Plugin                         = (*Plugin)(nil)
	_ plugin.ComponentConfigurationResolver = (*Plugin)(nil)
)

func init() {
	// Self-register on package import.
	if err := ci.RegisterPlugin(&Plugin{}); err != nil {
		// Panic on registration failure - this is a programming error.
		panic(fmt.Sprintf("failed to register terraform CI plugin: %v", err))
	}
}

// GetType returns the component type.
func (p *Plugin) GetType() string {
	defer perf.Track(nil, "terraform.Plugin.GetType")()

	return "terraform"
}

// GetHookBindings returns the hook bindings for Terraform CI integration.
func (p *Plugin) GetHookBindings() []plugin.HookBinding {
	defer perf.Track(nil, "terraform.Plugin.GetHookBindings")()

	return []plugin.HookBinding{
		{
			Event:    "before.terraform.plan",
			Actions:  []plugin.HookAction{plugin.ActionCheck},
			Template: "plan", // No template for check.
		},
		{
			Event:    "after.terraform.plan",
			Actions:  []plugin.HookAction{plugin.ActionSummary, plugin.ActionOutput, plugin.ActionUpload, plugin.ActionCheck},
			Template: "plan",
		},
		{
			Event:    "after.terraform.apply",
			Actions:  []plugin.HookAction{plugin.ActionSummary, plugin.ActionOutput},
			Template: "apply",
		},
		{
			Event:    "before.terraform.apply",
			Actions:  []plugin.HookAction{plugin.ActionDownload},
			Template: "", // No template for download.
		},
	}
}

// GetDefaultTemplates returns the embedded default templates.
func (p *Plugin) GetDefaultTemplates() embed.FS {
	defer perf.Track(nil, "terraform.Plugin.GetDefaultTemplates")()

	return defaultTemplates
}

// BuildTemplateContext creates a TerraformTemplateContext from execution results.
// Returns an extended context with terraform-specific fields for template rendering.
func (p *Plugin) BuildTemplateContext(
	info *schema.ConfigAndStacksInfo,
	ciCtx *provider.Context,
	output string,
	command string,
) (any, error) {
	defer perf.Track(nil, "terraform.Plugin.BuildTemplateContext")()

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
		Output:        output,
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

// ParseOutput parses terraform command output.
func (p *Plugin) ParseOutput(output string, command string) (*plugin.OutputResult, error) {
	defer perf.Track(nil, "terraform.Plugin.ParseOutput")()

	return ParseOutput(output, command), nil
}

// GetOutputVariables returns CI output variables for a command.
func (p *Plugin) GetOutputVariables(result *plugin.OutputResult, command string) map[string]string {
	defer perf.Track(nil, "terraform.Plugin.GetOutputVariables")()

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

// GetArtifactKey generates the artifact storage key for a command.
// TODO: Consider changing Plugin interface to return (string, error)
// to align with planfile.GenerateKey validation pattern. Currently uses defensive
// placeholders since the key is only used for debug logging.
func (p *Plugin) GetArtifactKey(info *schema.ConfigAndStacksInfo, command string) string {
	defer perf.Track(nil, "terraform.Plugin.GetArtifactKey")()

	// Validate required fields.
	if info == nil {
		log.Warn("GetArtifactKey called with nil info, using placeholder key")
		return "unknown/unknown.tfplan"
	}

	stack := info.Stack
	component := info.ComponentFromArg
	if stack == "" {
		log.Warn("GetArtifactKey called with empty Stack", "component", component)
		stack = "unknown"
	}
	if component == "" {
		log.Warn("GetArtifactKey called with empty ComponentFromArg", "stack", stack)
		component = "unknown"
	}

	// Default pattern: stack/component.tfplan
	return fmt.Sprintf("%s/%s.tfplan", stack, component)
}

// ResolveArtifactPath derives the planfile path from component and stack information.
// During `terraform plan`, the planfile path is generated internally but not propagated
// to PostRunE hooks. This method reconstructs it so CI hooks can find and upload the planfile.
// It also populates resolved fields on info needed for metadata (ContextPrefix, Component, etc.).
func (p *Plugin) ResolveComponentPlanfilePath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, error) {
	defer perf.Track(atmosConfig, "terraform.Plugin.ResolveArtifactPath")()

	if info.Stack == "" || info.ComponentFromArg == "" {
		return "", fmt.Errorf("both stack and component are required to resolve planfile path")
	}

	resolvedInfo, err := e.ProcessStacks(atmosConfig, *info, true, false, false, nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to resolve component path: %w", err)
	}

	// Carry over resolved fields needed by CI hooks for metadata.
	info.ContextPrefix = resolvedInfo.ContextPrefix
	info.Component = resolvedInfo.Component
	info.FinalComponent = resolvedInfo.FinalComponent
	info.ComponentFolderPrefix = resolvedInfo.ComponentFolderPrefix
	info.ComponentFolderPrefixReplaced = resolvedInfo.ComponentFolderPrefixReplaced
	info.ComponentSection = resolvedInfo.ComponentSection

	return e.ConstructTerraformComponentPlanfilePath(atmosConfig, &resolvedInfo), nil
}
