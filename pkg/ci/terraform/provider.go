// Package terraform provides the CI provider implementation for Terraform.
package terraform

import (
	"embed"
	"fmt"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:embed templates/*.md
var defaultTemplates embed.FS

// Provider implements ci.ComponentCIProvider for Terraform.
type Provider struct{}

// Ensure Provider implements ComponentCIProvider.
var _ ci.ComponentCIProvider = (*Provider)(nil)

func init() {
	// Self-register on package import.
	if err := ci.RegisterComponentProvider(&Provider{}); err != nil {
		// Panic on registration failure - this is a programming error.
		panic(fmt.Sprintf("failed to register terraform CI provider: %v", err))
	}
}

// GetType returns the component type.
func (p *Provider) GetType() string {
	return "terraform"
}

// GetHookBindings returns the hook bindings for Terraform CI integration.
func (p *Provider) GetHookBindings() []ci.HookBinding {
	return []ci.HookBinding{
		{
			Event:    "after.terraform.plan",
			Actions:  []ci.HookAction{ci.ActionSummary, ci.ActionOutput, ci.ActionUpload},
			Template: "plan",
		},
		{
			Event:    "after.terraform.apply",
			Actions:  []ci.HookAction{ci.ActionSummary, ci.ActionOutput},
			Template: "apply",
		},
		{
			Event:    "before.terraform.apply",
			Actions:  []ci.HookAction{ci.ActionDownload},
			Template: "", // No template for download.
		},
	}
}

// GetDefaultTemplates returns the embedded default templates.
func (p *Provider) GetDefaultTemplates() embed.FS {
	return defaultTemplates
}

// BuildTemplateContext creates a TerraformTemplateContext from execution results.
// Returns an extended context with terraform-specific fields for template rendering.
func (p *Provider) BuildTemplateContext(
	info *schema.ConfigAndStacksInfo,
	ciCtx *ci.Context,
	output string,
	command string,
) (any, error) {
	defer perf.Track(nil, "terraform.Provider.BuildTemplateContext")()

	// Parse the output to get structured data.
	result := ParseOutput(output, command)

	// Build base context.
	baseCtx := &ci.TemplateContext{
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
	var tfData *ci.TerraformOutputData
	if result != nil && result.Data != nil {
		tfData, _ = result.Data.(*ci.TerraformOutputData)
	}

	// Return extended context with terraform-specific fields.
	return NewTemplateContext(baseCtx, tfData), nil
}

// ParseOutput parses terraform command output.
func (p *Provider) ParseOutput(output string, command string) (*ci.OutputResult, error) {
	defer perf.Track(nil, "terraform.Provider.ParseOutput")()

	return ParseOutput(output, command), nil
}

// GetOutputVariables returns CI output variables for a command.
func (p *Provider) GetOutputVariables(result *ci.OutputResult, command string) map[string]string {
	defer perf.Track(nil, "terraform.Provider.GetOutputVariables")()

	vars := make(map[string]string)

	// Common outputs.
	vars["has_changes"] = fmt.Sprintf("%t", result.HasChanges)
	vars["has_errors"] = fmt.Sprintf("%t", result.HasErrors)
	vars["exit_code"] = fmt.Sprintf("%d", result.ExitCode)

	// Terraform-specific outputs.
	if data, ok := result.Data.(*ci.TerraformOutputData); ok {
		vars["resources_to_create"] = fmt.Sprintf("%d", data.ResourceCounts.Create)
		vars["resources_to_change"] = fmt.Sprintf("%d", data.ResourceCounts.Change)
		vars["resources_to_replace"] = fmt.Sprintf("%d", data.ResourceCounts.Replace)
		vars["resources_to_destroy"] = fmt.Sprintf("%d", data.ResourceCounts.Destroy)
	}

	return vars
}

// GetArtifactKey generates the artifact storage key for a command.
func (p *Provider) GetArtifactKey(info *schema.ConfigAndStacksInfo, command string) string {
	defer perf.Track(nil, "terraform.Provider.GetArtifactKey")()

	// Default pattern: stack/component.tfplan
	return fmt.Sprintf("%s/%s.tfplan", info.Stack, info.ComponentFromArg)
}
