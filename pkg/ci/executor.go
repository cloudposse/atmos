package ci

import (
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/templates"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecuteOptions contains options for executing CI hooks.
type ExecuteOptions struct {
	// Event is the hook event (e.g., "after.terraform.plan").
	Event string

	// AtmosConfig is the Atmos configuration.
	AtmosConfig *schema.AtmosConfiguration

	// Info contains component and stack information.
	Info *schema.ConfigAndStacksInfo

	// Output is the command output to process.
	Output string

	// ComponentType overrides the component type detection.
	// If empty, it's extracted from the event.
	ComponentType string

	// ForceCIMode forces CI mode even if environment detection fails.
	// This is set when --ci flag is used.
	ForceCIMode bool
}

// actionContext holds all context needed to execute a CI action.
type actionContext struct {
	Opts     ExecuteOptions
	Provider ComponentCIProvider
	Platform Provider
	CICtx    *Context
	Binding  *HookBinding
	Command  string
	Result   *OutputResult
}

// Execute runs all CI actions for a hook event.
// Returns nil if not in CI or if the event is not handled.
func Execute(opts ExecuteOptions) error {
	defer perf.Track(opts.AtmosConfig, "ci.Execute")()

	// Detect CI platform.
	platform := detectPlatform(opts.ForceCIMode)
	if platform == nil {
		return nil
	}

	// Get provider and binding for this event.
	provider, binding := getProviderAndBinding(opts)
	if provider == nil || binding == nil {
		return nil
	}

	// Build and execute actions.
	actCtx := buildActionContext(opts, platform, provider, binding)
	executeActions(actCtx, binding.Actions)

	return nil
}

// detectPlatform detects the CI platform based on environment.
func detectPlatform(forceCIMode bool) Provider {
	if forceCIMode {
		platform := Detect()
		if platform == nil {
			log.Debug("CI mode forced but no platform detected, using generic provider")
			return NewGenericProvider()
		}
		return platform
	}

	platform, err := DetectOrError()
	if err != nil {
		log.Debug("CI platform not detected, skipping CI hooks", "error", err)
		return nil
	}
	return platform
}

// getProviderAndBinding gets the component provider and hook binding for an event.
func getProviderAndBinding(opts ExecuteOptions) (ComponentCIProvider, *HookBinding) {
	componentType := opts.ComponentType
	if componentType == "" {
		componentType = extractComponentType(opts.Event)
	}

	if componentType == "" {
		log.Debug("Could not determine component type from event", "event", opts.Event)
		return nil, nil
	}

	provider, ok := GetComponentProvider(componentType)
	if !ok {
		log.Debug("No CI provider registered for component type", "component_type", componentType)
		return nil, nil
	}

	bindings := HookBindings(provider.GetHookBindings())
	binding := bindings.GetBindingForEvent(opts.Event)
	if binding == nil {
		log.Debug("Provider does not handle this event", "event", opts.Event, "component_type", componentType)
		return nil, nil
	}

	return provider, binding
}

// buildActionContext builds the action context for executing CI actions.
func buildActionContext(opts ExecuteOptions, platform Provider, provider ComponentCIProvider, binding *HookBinding) *actionContext {
	ciCtx, err := platform.Context()
	if err != nil {
		log.Warn("Failed to get CI context", "error", err)
		ciCtx = nil
	}

	command := extractCommand(opts.Event)

	result, err := provider.ParseOutput(opts.Output, command)
	if err != nil {
		log.Warn("Failed to parse command output", "error", err)
		result = &OutputResult{}
	}

	return &actionContext{
		Opts:     opts,
		Provider: provider,
		Platform: platform,
		CICtx:    ciCtx,
		Binding:  binding,
		Command:  command,
		Result:   result,
	}
}

// executeActions executes all actions in the binding.
func executeActions(ctx *actionContext, actions []HookAction) {
	for _, action := range actions {
		if err := executeAction(action, ctx); err != nil {
			log.Warn("CI action failed", "action", action, "error", err)
		}
	}
}

// executeAction executes a single CI action.
func executeAction(action HookAction, ctx *actionContext) error {
	defer perf.Track(ctx.Opts.AtmosConfig, "ci.executeAction")()

	switch action {
	case ActionSummary:
		return executeSummaryAction(ctx)

	case ActionOutput:
		return executeOutputAction(ctx)

	case ActionUpload:
		return executeUploadAction(ctx)

	case ActionDownload:
		return executeDownloadAction(ctx)

	case ActionCheck:
		return executeCheckAction(ctx)

	default:
		log.Debug("Unknown CI action", "action", action)
		return nil
	}
}

// executeSummaryAction writes to the CI job summary.
func executeSummaryAction(ctx *actionContext) error {
	defer perf.Track(ctx.Opts.AtmosConfig, "ci.executeSummaryAction")()

	if ctx.Binding.Template == "" {
		log.Debug("No template specified for summary action")
		return nil
	}

	writer := ctx.Platform.OutputWriter()
	if writer == nil {
		log.Debug("CI platform does not support summaries")
		return nil
	}

	// Build template context.
	// Provider returns an extended context type (e.g., *TerraformTemplateContext) that embeds *TemplateContext.
	tmplCtx, err := ctx.Provider.BuildTemplateContext(ctx.Opts.Info, ctx.CICtx, ctx.Opts.Output, ctx.Command)
	if err != nil {
		return errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithExplanation("Failed to build template context").
			Err()
	}

	// Load and render template.
	loader := templates.NewLoader(ctx.Opts.AtmosConfig)
	rendered, err := loader.LoadAndRender(
		ctx.Provider.GetType(),
		ctx.Binding.Template,
		ctx.Provider.GetDefaultTemplates(),
		tmplCtx,
	)
	if err != nil {
		return errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithExplanation("Failed to render template").
			WithContext("template", ctx.Binding.Template).
			Err()
	}

	// Write to job summary.
	if err := writer.WriteSummary(rendered); err != nil {
		return errUtils.Build(errUtils.ErrCIOutputWriteFailed).
			WithCause(err).
			WithExplanation("Failed to write CI summary").
			Err()
	}

	log.Debug("Wrote CI summary",
		"stack", ctx.Opts.Info.Stack,
		"component", ctx.Opts.Info.ComponentFromArg,
		"template", ctx.Binding.Template,
	)
	return nil
}

// executeOutputAction writes CI output variables.
func executeOutputAction(ctx *actionContext) error {
	defer perf.Track(ctx.Opts.AtmosConfig, "ci.executeOutputAction")()

	writer := ctx.Platform.OutputWriter()
	if writer == nil {
		log.Debug("CI platform does not support outputs")
		return nil
	}

	// Get output variables from provider.
	vars := ctx.Provider.GetOutputVariables(ctx.Result, ctx.Command)

	// Add common variables.
	vars["stack"] = ctx.Opts.Info.Stack
	vars["component"] = ctx.Opts.Info.ComponentFromArg
	vars["command"] = ctx.Command

	// Write each variable.
	for key, value := range vars {
		if err := writer.WriteOutput(key, value); err != nil {
			log.Warn("Failed to write CI output", "key", key, "error", err)
		}
	}

	log.Debug("Wrote CI outputs", "count", len(vars))
	return nil
}

// executeUploadAction uploads an artifact.
func executeUploadAction(ctx *actionContext) error {
	defer perf.Track(ctx.Opts.AtmosConfig, "ci.executeUploadAction")()

	// Artifact upload is handled separately by the planfile store.
	// This action is a marker that upload should occur.
	key := ctx.Provider.GetArtifactKey(ctx.Opts.Info, ctx.Command)
	log.Debug("Artifact upload requested", "key", key)

	// The actual upload is performed by the planfile system.
	// This action ensures the hook binding declares the intent.
	return nil
}

// executeDownloadAction downloads an artifact.
func executeDownloadAction(ctx *actionContext) error {
	defer perf.Track(ctx.Opts.AtmosConfig, "ci.executeDownloadAction")()

	// Artifact download is handled separately by the planfile store.
	// This action is a marker that download should occur.
	key := ctx.Provider.GetArtifactKey(ctx.Opts.Info, ctx.Command)
	log.Debug("Artifact download requested", "key", key)

	// The actual download is performed by the planfile system.
	// This action ensures the hook binding declares the intent.
	return nil
}

// executeCheckAction performs a validation/check.
func executeCheckAction(ctx *actionContext) error {
	defer perf.Track(ctx.Opts.AtmosConfig, "ci.executeCheckAction")()

	// Check actions are provider-specific.
	// For now, this is a placeholder for future drift detection, etc.
	log.Debug("Check action not yet implemented")
	return nil
}

// extractComponentType extracts the component type from a hook event.
// Example: "after.terraform.plan" -> "terraform".
func extractComponentType(event string) string {
	parts := strings.Split(event, ".")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

// extractCommand extracts the command from a hook event.
// Example: "after.terraform.plan" -> "plan".
func extractCommand(event string) string {
	parts := strings.Split(event, ".")
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}
