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

// Execute runs all CI actions for a hook event.
// Returns nil if not in CI or if the event is not handled.
func Execute(opts ExecuteOptions) error {
	defer perf.Track(opts.AtmosConfig, "ci.Execute")()

	// 1. Detect CI platform or use forced mode.
	var platform Provider
	var err error

	if opts.ForceCIMode {
		// Force CI mode - try to detect platform, fall back to generic if none detected.
		platform = Detect()
		if platform == nil {
			log.Debug("CI mode forced but no platform detected, using generic provider")
			platform = NewGenericProvider()
		}
	} else {
		// Normal mode - require platform detection.
		platform, err = DetectOrError()
		if err != nil {
			log.Debug("CI platform not detected, skipping CI hooks", "error", err)
			return nil
		}
	}

	// 2. Get component type from event if not provided.
	componentType := opts.ComponentType
	if componentType == "" {
		componentType = extractComponentType(opts.Event)
	}

	if componentType == "" {
		log.Debug("Could not determine component type from event", "event", opts.Event)
		return nil
	}

	// 3. Get the component CI provider.
	provider, ok := GetComponentProvider(componentType)
	if !ok {
		log.Debug("No CI provider registered for component type", "component_type", componentType)
		return nil
	}

	// 4. Find matching hook binding.
	bindings := HookBindings(provider.GetHookBindings())
	binding := bindings.GetBindingForEvent(opts.Event)
	if binding == nil {
		log.Debug("Provider does not handle this event", "event", opts.Event, "component_type", componentType)
		return nil
	}

	// 5. Get CI context.
	ciCtx, err := platform.Context()
	if err != nil {
		log.Warn("Failed to get CI context", "error", err)
		// Continue with nil context - templates will handle missing data.
		ciCtx = nil
	}

	// 6. Extract command from event.
	command := extractCommand(opts.Event)

	// 7. Parse output.
	result, err := provider.ParseOutput(opts.Output, command)
	if err != nil {
		log.Warn("Failed to parse command output", "error", err)
		// Continue with nil result.
		result = &OutputResult{}
	}

	// 8. Execute each action in the binding.
	for _, action := range binding.Actions {
		if err := executeAction(action, opts, provider, platform, ciCtx, binding, command, result); err != nil {
			// Log error but continue with other actions.
			log.Warn("CI action failed", "action", action, "error", err)
		}
	}

	return nil
}

// executeAction executes a single CI action.
func executeAction(
	action HookAction,
	opts ExecuteOptions,
	provider ComponentCIProvider,
	platform Provider,
	ciCtx *Context,
	binding *HookBinding,
	command string,
	result *OutputResult,
) error {
	defer perf.Track(opts.AtmosConfig, "ci.executeAction")()

	switch action {
	case ActionSummary:
		return executeSummaryAction(opts, provider, platform, ciCtx, binding, command, result)

	case ActionOutput:
		return executeOutputAction(opts, provider, platform, result, command)

	case ActionUpload:
		return executeUploadAction(opts, provider, command)

	case ActionDownload:
		return executeDownloadAction(opts, provider, command)

	case ActionCheck:
		return executeCheckAction(opts, provider, platform, ciCtx, result, command)

	default:
		log.Debug("Unknown CI action", "action", action)
		return nil
	}
}

// executeSummaryAction writes to the CI job summary.
func executeSummaryAction(
	opts ExecuteOptions,
	provider ComponentCIProvider,
	platform Provider,
	ciCtx *Context,
	binding *HookBinding,
	command string,
	result *OutputResult,
) error {
	defer perf.Track(opts.AtmosConfig, "ci.executeSummaryAction")()

	if binding.Template == "" {
		log.Debug("No template specified for summary action")
		return nil
	}

	writer := platform.OutputWriter()
	if writer == nil {
		log.Debug("CI platform does not support summaries")
		return nil
	}

	// Build template context.
	// Provider returns an extended context type (e.g., *TerraformTemplateContext) that embeds *TemplateContext.
	ctx, err := provider.BuildTemplateContext(opts.Info, ciCtx, opts.Output, command)
	if err != nil {
		return errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithExplanation("Failed to build template context").
			Err()
	}

	// Load and render template.
	loader := templates.NewLoader(opts.AtmosConfig)
	rendered, err := loader.LoadAndRender(
		provider.GetType(),
		binding.Template,
		provider.GetDefaultTemplates(),
		ctx,
	)
	if err != nil {
		return errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithExplanation("Failed to render template").
			WithContext("template", binding.Template).
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
		"stack", opts.Info.Stack,
		"component", opts.Info.ComponentFromArg,
		"template", binding.Template,
	)
	return nil
}

// executeOutputAction writes CI output variables.
func executeOutputAction(
	opts ExecuteOptions,
	provider ComponentCIProvider,
	platform Provider,
	result *OutputResult,
	command string,
) error {
	defer perf.Track(opts.AtmosConfig, "ci.executeOutputAction")()

	writer := platform.OutputWriter()
	if writer == nil {
		log.Debug("CI platform does not support outputs")
		return nil
	}

	// Get output variables from provider.
	vars := provider.GetOutputVariables(result, command)

	// Add common variables.
	vars["stack"] = opts.Info.Stack
	vars["component"] = opts.Info.ComponentFromArg
	vars["command"] = command

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
func executeUploadAction(
	opts ExecuteOptions,
	provider ComponentCIProvider,
	command string,
) error {
	defer perf.Track(opts.AtmosConfig, "ci.executeUploadAction")()

	// Artifact upload is handled separately by the planfile store.
	// This action is a marker that upload should occur.
	key := provider.GetArtifactKey(opts.Info, command)
	log.Debug("Artifact upload requested", "key", key)

	// The actual upload is performed by the planfile system.
	// This action ensures the hook binding declares the intent.
	return nil
}

// executeDownloadAction downloads an artifact.
func executeDownloadAction(
	opts ExecuteOptions,
	provider ComponentCIProvider,
	command string,
) error {
	defer perf.Track(opts.AtmosConfig, "ci.executeDownloadAction")()

	// Artifact download is handled separately by the planfile store.
	// This action is a marker that download should occur.
	key := provider.GetArtifactKey(opts.Info, command)
	log.Debug("Artifact download requested", "key", key)

	// The actual download is performed by the planfile system.
	// This action ensures the hook binding declares the intent.
	return nil
}

// executeCheckAction performs a validation/check.
func executeCheckAction(
	opts ExecuteOptions,
	provider ComponentCIProvider,
	platform Provider,
	ciCtx *Context,
	result *OutputResult,
	command string,
) error {
	defer perf.Track(opts.AtmosConfig, "ci.executeCheckAction")()

	// Check actions are provider-specific.
	// For now, this is a placeholder for future drift detection, etc.
	log.Debug("Check action not yet implemented")
	return nil
}

// extractComponentType extracts the component type from a hook event.
// e.g., "after.terraform.plan" -> "terraform".
func extractComponentType(event string) string {
	parts := strings.Split(event, ".")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

// extractCommand extracts the command from a hook event.
// e.g., "after.terraform.plan" -> "plan".
func extractCommand(event string) string {
	parts := strings.Split(event, ".")
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}
