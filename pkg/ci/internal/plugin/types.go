// Package plugin defines the CI plugin interface and related types for component type abstractions.
package plugin

import (
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/ci/templates"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:generate mockgen -typed -destination=../../../mock_plugin_test.go -package=ci github.com/cloudposse/atmos/pkg/ci/internal/plugin Plugin

// HookHandler is the callback signature for plugin event handlers.
// Plugins implement handlers that own all action logic for their events.
type HookHandler func(ctx *HookContext) error

// HookContext provides everything a plugin callback needs to execute CI actions.
type HookContext struct {
	// Event is the full hook event (e.g., "after.terraform.plan").
	Event string

	// Command is the extracted command (e.g., "plan", "apply").
	Command string

	// EventPrefix is "before" or "after".
	EventPrefix string

	// Config is the Atmos configuration.
	Config *schema.AtmosConfiguration

	// Info contains component and stack information.
	Info *schema.ConfigAndStacksInfo

	// Output is the raw command output.
	Output string

	// CommandError is the error from the command execution, if any.
	CommandError error

	// Provider is the detected CI platform provider.
	Provider provider.Provider

	// CICtx is the CI platform context (repo, SHA, PR, etc.).
	CICtx *provider.Context

	// TemplateLoader loads and renders CI summary templates.
	TemplateLoader *templates.Loader

	// CreatePlanfileStore is a lazy factory for creating planfile stores.
	// Not all events need a store, so it's created on demand.
	// Returns an any that should be type-asserted to planfile.Store by the handler.
	CreatePlanfileStore func() (any, error)
}

// HookBinding declares what happens at a specific hook event.
type HookBinding struct {
	// Event is the hook event pattern (e.g., "after.terraform.plan").
	Event string

	// Handler is the callback that owns all action logic for this event.
	Handler HookHandler
}

// Plugin is implemented by component types that support CI integration.
// Plugins own all action logic via Handler callbacks in their HookBindings.
// Unlike Provider (which represents CI platforms like GitHub/GitLab), this interface
// represents component types (terraform, helmfile) and their CI behavior.
type Plugin interface {
	// GetType returns the component type (e.g., "terraform", "helmfile").
	GetType() string

	// GetHookBindings returns all hook bindings for this plugin.
	// Each binding declares an event and a Handler callback that owns all action logic.
	GetHookBindings() []HookBinding
}

// HookBindings is a slice of HookBinding with helper methods.
type HookBindings []HookBinding

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

	// Warnings contains full warning block text extracted from terraform output.
	Warnings []string
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
