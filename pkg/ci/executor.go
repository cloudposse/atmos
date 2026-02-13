package ci

import (
	"context"
	"io"
	"os"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/planfile"
	_ "github.com/cloudposse/atmos/pkg/ci/planfile/github" // Register github store.
	_ "github.com/cloudposse/atmos/pkg/ci/planfile/local"  // Register local store.
	_ "github.com/cloudposse/atmos/pkg/ci/planfile/s3"     // Register s3 store.
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
		// Check if action is enabled in config.
		if !isActionEnabled(ctx.Opts.AtmosConfig, action) {
			log.Debug("CI action disabled by config", "action", action)
			continue
		}
		if err := executeAction(action, ctx); err != nil {
			log.Warn("CI action failed", "action", action, "error", err)
		}
	}
}

// isActionEnabled checks if a CI action is enabled in the configuration.
// Returns true if the action should be executed, false if it should be skipped.
// When config is nil or the feature is not explicitly configured, defaults are:
// - Summary: enabled by default
// - Output: enabled by default
// - Checks: disabled by default (requires extra permissions)
// - Upload/Download: always enabled (controlled by planfile config).
func isActionEnabled(cfg *schema.AtmosConfiguration, action HookAction) bool {
	// No config means use defaults (enabled for most actions).
	if cfg == nil {
		return action != ActionCheck // Checks disabled by default.
	}

	switch action {
	case ActionSummary:
		// Summary is enabled by default. Only skip if explicitly disabled.
		// nil means "not set" = use default (enabled).
		if cfg.CI.Summary.Enabled == nil {
			return true
		}
		return *cfg.CI.Summary.Enabled
	case ActionOutput:
		// Output is enabled by default. Only skip if explicitly disabled.
		// nil means "not set" = use default (enabled).
		if cfg.CI.Output.Enabled == nil {
			return true
		}
		return *cfg.CI.Output.Enabled
	case ActionCheck:
		// Checks are disabled by default (require extra permissions).
		// nil means "not set" = use default (disabled).
		if cfg.CI.Checks.Enabled == nil {
			return false
		}
		return *cfg.CI.Checks.Enabled
	case ActionUpload, ActionDownload:
		// Upload/Download are always enabled (controlled by planfile config).
		return true
	default:
		return true
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

	// Get template name - prefer config override, fall back to binding.
	templateName := ctx.Binding.Template
	if cfg := ctx.Opts.AtmosConfig; cfg != nil && cfg.CI.Summary.Template != "" {
		templateName = cfg.CI.Summary.Template
	}

	if templateName == "" {
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
		templateName,
		ctx.Provider.GetDefaultTemplates(),
		tmplCtx,
	)
	if err != nil {
		return errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithExplanation("Failed to render template").
			WithContext("template", templateName).
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
		"template", templateName,
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

	// Filter by configured variables if specified.
	if cfg := ctx.Opts.AtmosConfig; cfg != nil && len(cfg.CI.Output.Variables) > 0 {
		vars = filterVariables(vars, cfg.CI.Output.Variables)
	}

	// Write each variable.
	for key, value := range vars {
		if err := writer.WriteOutput(key, value); err != nil {
			log.Warn("Failed to write CI output", "key", key, "error", err)
		}
	}

	log.Debug("Wrote CI outputs", "count", len(vars))
	return nil
}

// filterVariables filters a map of variables to only include those in the allowed list.
func filterVariables(vars map[string]string, allowed []string) map[string]string {
	if len(allowed) == 0 {
		return vars
	}
	allowedSet := make(map[string]bool)
	for _, v := range allowed {
		allowedSet[v] = true
	}
	filtered := make(map[string]string)
	for k, v := range vars {
		if allowedSet[k] {
			filtered[k] = v
		}
	}
	return filtered
}

// executeUploadAction uploads a planfile to the configured storage backend.
// This action is triggered after a terraform plan command completes.
func executeUploadAction(ctx *actionContext) error {
	defer perf.Track(ctx.Opts.AtmosConfig, "ci.executeUploadAction")()

	planfilePath, key, skip := validateUploadPrerequisites(ctx)
	if skip {
		return nil
	}

	store, err := createPlanfileStore(ctx)
	if err != nil {
		return errUtils.Build(errUtils.ErrPlanfileUploadFailed).WithCause(err).
			WithExplanation("Failed to create planfile store").Err()
	}

	if err := uploadPlanfile(ctx, store, planfilePath, key); err != nil {
		return err
	}

	logArtifactOperation("Uploaded", key, store.Name(), "", ctx.Opts.Info)
	return nil
}

// validateUploadPrerequisites checks if upload can proceed and returns the path and key.
// When the planfile path is not explicitly set, it attempts to resolve it via ComponentConfigurationResolver.
func validateUploadPrerequisites(ctx *actionContext) (path, key string, skip bool) {
	path = ctx.Opts.Info.PlanFile
	if path == "" {
		if resolver, ok := ctx.Provider.(ComponentConfigurationResolver); ok {
			resolved, err := resolver.ResolveComponentPlanfilePath(ctx.Opts.AtmosConfig, ctx.Opts.Info)
			if err != nil {
				log.Debug("Failed to resolve artifact path for upload", "error", err)
			} else {
				ctx.Opts.Info.PlanFile = resolved
				path = resolved
			}
		}
	}
	if path == "" {
		log.Debug("No planfile path specified, skipping upload")
		return "", "", true
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Debug("Planfile does not exist, skipping upload", "path", path)
		return "", "", true
	}
	key = ctx.Provider.GetArtifactKey(ctx.Opts.Info, ctx.Command)
	if key == "" {
		log.Debug("Could not generate artifact key, skipping upload")
		return "", "", true
	}
	return path, key, false
}

// uploadPlanfile opens and uploads a planfile to the store.
func uploadPlanfile(ctx *actionContext, store planfile.Store, path, key string) error {
	f, err := os.Open(path)
	if err != nil {
		return errUtils.Build(errUtils.ErrPlanfileUploadFailed).WithCause(err).
			WithExplanation("Failed to open planfile for upload").WithContext("path", path).Err()
	}
	defer f.Close()

	metadata := buildPlanfileMetadata(ctx)
	if err := store.Upload(context.Background(), key, f, metadata); err != nil {
		return errUtils.Build(errUtils.ErrPlanfileUploadFailed).WithCause(err).
			WithExplanation("Failed to upload planfile to store").
			WithContext("key", key).WithContext("store", store.Name()).Err()
	}
	return nil
}

// executeDownloadAction downloads a planfile from the configured storage backend.
// This action is triggered before a terraform apply command runs.
func executeDownloadAction(ctx *actionContext) error {
	defer perf.Track(ctx.Opts.AtmosConfig, "ci.executeDownloadAction")()

	planfilePath, key, skip := validateDownloadPrerequisites(ctx)
	if skip {
		return nil
	}

	store, err := createPlanfileStore(ctx)
	if err != nil {
		return errUtils.Build(errUtils.ErrPlanfileDownloadFailed).WithCause(err).
			WithExplanation("Failed to create planfile store").Err()
	}

	if err := downloadPlanfile(store, planfilePath, key); err != nil {
		return err
	}

	logArtifactOperation("Downloaded", key, store.Name(), planfilePath, ctx.Opts.Info)
	return nil
}

// validateDownloadPrerequisites checks if download can proceed and returns the path and key.
// When the planfile path is not explicitly set, it attempts to resolve it via ComponentConfigurationResolver.
func validateDownloadPrerequisites(ctx *actionContext) (path, key string, skip bool) {
	path = ctx.Opts.Info.PlanFile
	if path == "" {
		if resolver, ok := ctx.Provider.(ComponentConfigurationResolver); ok {
			resolved, err := resolver.ResolveComponentPlanfilePath(ctx.Opts.AtmosConfig, ctx.Opts.Info)
			if err != nil {
				log.Debug("Failed to resolve artifact path for download", "error", err)
			} else {
				ctx.Opts.Info.PlanFile = resolved
				path = resolved
			}
		}
	}
	if path == "" {
		log.Debug("No planfile path specified, skipping download")
		return "", "", true
	}
	key = ctx.Provider.GetArtifactKey(ctx.Opts.Info, ctx.Command)
	if key == "" {
		log.Debug("Could not generate artifact key, skipping download")
		return "", "", true
	}
	return path, key, false
}

// downloadPlanfile downloads a planfile from the store and writes it to disk.
func downloadPlanfile(store planfile.Store, path, key string) error {
	reader, _, err := store.Download(context.Background(), key)
	if err != nil {
		return errUtils.Build(errUtils.ErrPlanfileDownloadFailed).WithCause(err).
			WithExplanation("Failed to download planfile from store").
			WithContext("key", key).WithContext("store", store.Name()).Err()
	}
	defer reader.Close()

	f, err := os.Create(path)
	if err != nil {
		return errUtils.Build(errUtils.ErrPlanfileDownloadFailed).WithCause(err).
			WithExplanation("Failed to create local planfile").WithContext("path", path).Err()
	}
	defer f.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return errUtils.Build(errUtils.ErrPlanfileDownloadFailed).WithCause(err).
			WithExplanation("Failed to write planfile to disk").WithContext("path", path).Err()
	}
	return nil
}

// logArtifactOperation logs details about a planfile upload/download operation.
func logArtifactOperation(op, key, storeName, path string, info *schema.ConfigAndStacksInfo) {
	args := []any{"key", key, "store", storeName}
	if path != "" {
		args = append(args, "path", path)
	}
	args = append(args, "stack", info.Stack, "component", info.ComponentFromArg)
	log.Debug(op+" planfile", args...)
}

// executeCheckAction performs a validation/check.
func executeCheckAction(ctx *actionContext) error {
	defer perf.Track(ctx.Opts.AtmosConfig, "ci.executeCheckAction")()

	// Check actions are provider-specific.
	// For now, this is a placeholder for future drift detection, etc.
	log.Debug("Check action not yet implemented")
	return nil
}

// createPlanfileStore creates a planfile store from configuration.
func createPlanfileStore(ctx *actionContext) (planfile.Store, error) {
	defer perf.Track(ctx.Opts.AtmosConfig, "ci.createPlanfileStore")()

	opts := planfile.StoreOptions{
		AtmosConfig: ctx.Opts.AtmosConfig,
	}

	// Use the default store from configuration if available.
	if ctx.Opts.AtmosConfig != nil {
		planfilesConfig := ctx.Opts.AtmosConfig.Components.Terraform.Planfiles
		if planfilesConfig.Default != "" {
			if storeSpec, ok := planfilesConfig.Stores[planfilesConfig.Default]; ok {
				opts.Type = storeSpec.Type
				opts.Options = storeSpec.Options
				return planfile.NewStore(opts)
			}
		}
	}

	// Fall back to environment-based detection.
	if storeOpts := detectStoreFromEnv(); storeOpts != nil {
		storeOpts.AtmosConfig = ctx.Opts.AtmosConfig
		return planfile.NewStore(*storeOpts)
	}

	// Default to local storage.
	opts.Type = "local"
	opts.Options = map[string]any{
		"path": ".atmos/planfiles",
	}
	return planfile.NewStore(opts)
}

// detectStoreFromEnv detects the planfile store from environment variables.
func detectStoreFromEnv() *planfile.StoreOptions {
	defer perf.Track(nil, "ci.detectStoreFromEnv")()

	// Check for S3 configuration.
	if bucket := os.Getenv("ATMOS_PLANFILE_BUCKET"); bucket != "" {
		return &planfile.StoreOptions{
			Type: "s3",
			Options: map[string]any{
				"bucket": bucket,
				"prefix": os.Getenv("ATMOS_PLANFILE_PREFIX"),
				"region": os.Getenv("AWS_REGION"),
			},
		}
	}

	// Check for GitHub Actions.
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return &planfile.StoreOptions{
			Type:    "github-artifacts",
			Options: map[string]any{},
		}
	}

	return nil
}

// buildPlanfileMetadata builds metadata for a planfile from action context.
func buildPlanfileMetadata(ctx *actionContext) *planfile.Metadata {
	defer perf.Track(ctx.Opts.AtmosConfig, "ci.buildPlanfileMetadata")()

	metadata := &planfile.Metadata{
		Stack:         ctx.Opts.Info.Stack,
		Component:     ctx.Opts.Info.ComponentFromArg,
		ComponentPath: ctx.Opts.Info.ComponentFolderPrefix,
		CreatedAt:     time.Now(),
	}

	// Add CI context if available.
	if ctx.CICtx != nil {
		metadata.SHA = ctx.CICtx.SHA
		metadata.Branch = ctx.CICtx.Branch
		metadata.RunID = ctx.CICtx.RunID
		metadata.Repository = ctx.CICtx.Repository
		if ctx.CICtx.PullRequest != nil {
			metadata.PRNumber = ctx.CICtx.PullRequest.Number
		}
	}

	// Add plan result data if available.
	if ctx.Result != nil {
		metadata.HasChanges = ctx.Result.HasChanges
		if tfData, ok := ctx.Result.Data.(*TerraformOutputData); ok {
			metadata.Additions = tfData.ResourceCounts.Create
			metadata.Changes = tfData.ResourceCounts.Change
			metadata.Destructions = tfData.ResourceCounts.Destroy
			metadata.PlanSummary = tfData.ChangedResult
		}
	}

	return metadata
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
