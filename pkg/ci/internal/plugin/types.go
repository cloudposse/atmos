// Package plugin defines the CI plugin interface and related types for component type abstractions.
package plugin

import (
	"embed"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// HookAction represents what CI action to perform.
type HookAction string

const (
	// ActionSummary writes to job summary ($GITHUB_STEP_SUMMARY).
	ActionSummary HookAction = "summary"

	// ActionOutput writes to CI outputs ($GITHUB_OUTPUT).
	ActionOutput HookAction = "output"

	// ActionUpload uploads an artifact (e.g., planfile).
	ActionUpload HookAction = "upload"

	// ActionDownload downloads an artifact.
	ActionDownload HookAction = "download"

	// ActionCheck validates or checks (e.g., drift detection).
	ActionCheck HookAction = "check"
)

// HookBinding declares what happens at a specific hook event.
type HookBinding struct {
	// Event is the hook event pattern (e.g., "after.terraform.plan").
	Event string

	// Actions lists the CI actions to perform at this event.
	Actions []HookAction

	// Template is the template name for summary action (e.g., "plan" -> templates/plan.md).
	// Empty if no template is needed.
	Template string
}

// Plugin is implemented by component types that support CI integration.
// Covers templates, outputs, and artifacts for pipeline automation.
// Unlike Provider (which represents CI platforms like GitHub/GitLab), this interface
// represents component types (terraform, helmfile) and their CI behavior.
type Plugin interface {
	// GetType returns the component type (e.g., "terraform", "helmfile").
	GetType() string

	// GetHookBindings returns all hook bindings for this provider.
	// Declares which events this provider handles and what actions occur at each.
	GetHookBindings() []HookBinding

	// GetDefaultTemplates returns the embedded filesystem containing default templates.
	// Templates are stored as {command}.md (e.g., templates/plan.md, templates/apply.md).
	GetDefaultTemplates() embed.FS

	// BuildTemplateContext creates a template context from execution results.
	// The context is used to render the summary template.
	// Returns an extended context type (e.g., *TerraformTemplateContext) that embeds *TemplateContext.
	BuildTemplateContext(
		info *schema.ConfigAndStacksInfo,
		ciCtx *provider.Context,
		output string,
		command string,
	) (any, error)

	// ParseOutput parses command output to extract structured data.
	// Returns metadata about the execution (changes, errors, etc.).
	ParseOutput(output string, command string) (*OutputResult, error)

	// GetOutputVariables returns CI output variables for a command.
	// These are written to $GITHUB_OUTPUT or equivalent.
	GetOutputVariables(result *OutputResult, command string) map[string]string

	// GetArtifactKey generates the artifact storage key for a command.
	// Used for uploading/downloading planfiles and other artifacts.
	GetArtifactKey(info *schema.ConfigAndStacksInfo, command string) string
}

// ComponentConfigurationResolver is an optional interface that Plugins can implement
// to resolve artifact paths (e.g., planfile paths) when not explicitly provided.
// The executor checks for this interface before upload/download actions and uses it
// to derive the path from component and stack information.
type ComponentConfigurationResolver interface {
	// ResolveComponentPlanfilePath derives the planfile path from component/stack information.
	// It also populates resolved fields on info needed for metadata (e.g., ContextPrefix).
	// Returns the resolved path, or empty string if resolution is not possible.
	ResolveComponentPlanfilePath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, error)
}

// OutputResult contains parsed command output.
type OutputResult struct {
	// ExitCode is the command exit code.
	ExitCode int

	// HasChanges indicates if there are pending changes.
	HasChanges bool

	// HasErrors indicates if there were errors.
	HasErrors bool

	// Errors contains error messages if HasErrors is true.
	Errors []string

	// Data contains component-specific parsed data.
	// For terraform: *TerraformOutputData
	// For helmfile: *HelmfileOutputData
	Data any
}

// TerraformOutputData contains terraform-specific output data.
type TerraformOutputData struct {
	// ResourceCounts contains resource change counts.
	ResourceCounts ResourceCounts

	// CreatedResources contains addresses of resources to be created.
	CreatedResources []string

	// UpdatedResources contains addresses of resources to be updated.
	UpdatedResources []string

	// ReplacedResources contains addresses of resources to be replaced.
	ReplacedResources []string

	// DeletedResources contains addresses of resources to be destroyed.
	DeletedResources []string

	// MovedResources contains resources that have been moved.
	MovedResources []MovedResource

	// ImportedResources contains addresses of resources to be imported.
	ImportedResources []string

	// Outputs contains terraform output values (after apply).
	Outputs map[string]TerraformOutput

	// ChangedResult contains the plan summary text.
	ChangedResult string
}

// TerraformOutput represents a single terraform output value.
type TerraformOutput struct {
	// Value is the output value (string, number, bool, list, map).
	Value any

	// Type is the output type (string, number, bool, list, map, object).
	Type string

	// Sensitive indicates whether the output is marked as sensitive.
	Sensitive bool
}

// MovedResource represents a resource that has been moved.
type MovedResource struct {
	// From is the original resource address.
	From string

	// To is the new resource address.
	To string
}

// ResourceCounts contains resource change counts.
type ResourceCounts struct {
	// Create is the number of resources to create.
	Create int

	// Change is the number of resources to change.
	Change int

	// Replace is the number of resources to replace.
	Replace int

	// Destroy is the number of resources to destroy.
	Destroy int
}

// HelmfileOutputData contains helmfile-specific output data.
type HelmfileOutputData struct {
	// Releases contains information about releases.
	Releases []ReleaseInfo
}

// ReleaseInfo contains helmfile release information.
type ReleaseInfo struct {
	// Name is the release name.
	Name string

	// Namespace is the Kubernetes namespace.
	Namespace string

	// Status is the release status.
	Status string
}

// TemplateContext contains all data available to CI summary templates.
type TemplateContext struct {
	// Component is the component name.
	Component string

	// ComponentType is the component type (e.g., "terraform", "helmfile").
	ComponentType string

	// Stack is the stack name.
	Stack string

	// Command is the command that was executed (e.g., "plan", "apply").
	Command string

	// CI contains CI platform metadata.
	CI *provider.Context

	// Result contains parsed output data.
	Result *OutputResult

	// Output is the raw command output.
	Output string

	// Custom contains custom variables from configuration.
	Custom map[string]any
}

// GetBindingForEvent returns the hook binding for a specific event, or nil if not found.
func (bindings HookBindings) GetBindingForEvent(event string) *HookBinding {
	defer perf.Track(nil, "plugin.HookBindings.GetBindingForEvent")()

	for i := range bindings {
		if bindings[i].Event == event {
			return &bindings[i]
		}
	}
	return nil
}

// HookBindings is a slice of HookBinding with helper methods.
type HookBindings []HookBinding

// HasAction returns true if the binding has the specified action.
func (b *HookBinding) HasAction(action HookAction) bool {
	defer perf.Track(nil, "plugin.HookBinding.HasAction")()

	for _, a := range b.Actions {
		if a == action {
			return true
		}
	}
	return false
}
