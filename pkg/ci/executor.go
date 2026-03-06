package ci

import (
	"os"
	"strings"

	"github.com/cloudposse/atmos/pkg/ci/artifact"
	_ "github.com/cloudposse/atmos/pkg/ci/artifact/github" // Register github artifact store.
	_ "github.com/cloudposse/atmos/pkg/ci/artifact/local" // Register local artifact store.
	_ "github.com/cloudposse/atmos/pkg/ci/artifact/s3" // Register s3 artifact store.
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile/adapter"
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

	// CommandError is the error from the command execution, if any.
	// When set, check runs are updated with failure status.
	CommandError error
}

// Execute runs all CI actions for a hook event.
// Returns nil if not in CI or if the event is not handled.
func Execute(opts ExecuteOptions) error {
	defer perf.Track(opts.AtmosConfig, "ci.Execute")()

	// Detect CI platform.
	// Only ForceCIMode (--ci flag) triggers the generic provider fallback.
	// ci.enabled in config means "CI features are available" but requires
	// an actual CI platform to be detected (or --ci flag) to activate.
	platform := detectPlatform(opts.ForceCIMode)
	if platform == nil {
		return nil
	}

	// Get plugin and binding for this event.
	pl, binding := getPluginAndBinding(opts)
	if pl == nil || binding == nil {
		return nil
	}

	// Dispatch to the plugin's handler callback.
	if binding.Handler == nil {
		log.Debug("Binding has no handler", "event", opts.Event)
		return nil
	}

	hookCtx := buildHookContext(opts, platform)
	if err := binding.Handler(hookCtx); err != nil {
		log.Warn("CI hook handler failed", "event", opts.Event, "error", err)
	}

	return nil
}

// detectPlatform detects the CI platform based on environment.
func detectPlatform(forceCIMode bool) provider.Provider {
	if forceCIMode {
		platform := Detect()
		if platform == nil {
			log.Debug("CI mode forced but no platform detected, using generic provider")
			generic, err := Get("generic")
			if err != nil {
				log.Warn("Failed to get generic CI provider", "error", err)
				return nil
			}
			return generic
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

// getPluginAndBinding gets the CI plugin and hook binding for an event.
func getPluginAndBinding(opts ExecuteOptions) (plugin.Plugin, *plugin.HookBinding) {
	componentType := opts.ComponentType
	if componentType == "" {
		componentType = extractComponentType(opts.Event)
	}

	if componentType == "" {
		log.Debug("Could not determine component type from event", "event", opts.Event)
		return nil, nil
	}

	pl, ok := GetPlugin(componentType)
	if !ok {
		log.Debug("No CI plugin registered for component type", "component_type", componentType)
		return nil, nil
	}

	bindings := plugin.HookBindings(pl.GetHookBindings())
	binding := bindings.GetBindingForEvent(opts.Event)
	if binding == nil {
		log.Debug("Plugin does not handle this event", "event", opts.Event, "component_type", componentType)
		return nil, nil
	}

	return pl, binding
}

// buildHookContext assembles the HookContext with all dependencies for callback-based dispatch.
func buildHookContext(opts ExecuteOptions, platform provider.Provider) *plugin.HookContext {
	defer perf.Track(opts.AtmosConfig, "ci.buildHookContext")()

	ciCtx, err := platform.Context()
	if err != nil {
		log.Warn("Failed to get CI context", "error", err)
		ciCtx = nil
	}

	command := extractCommand(opts.Event)
	eventPrefix := extractEventPrefix(opts.Event)
	loader := templates.NewLoader(opts.AtmosConfig)

	return &plugin.HookContext{
		Event:        opts.Event,
		Command:      command,
		EventPrefix:  eventPrefix,
		Config:       opts.AtmosConfig,
		Info:         opts.Info,
		Output:       opts.Output,
		CommandError: opts.CommandError,
		Provider:     platform,
		CICtx:        ciCtx,
		TemplateLoader: loader,
		CreatePlanfileStore: func() (any, error) {
			return createPlanfileStore(opts)
		},
	}
}

// createPlanfileStore creates a planfile store from ExecuteOptions.
func createPlanfileStore(opts ExecuteOptions) (planfile.Store, error) {
	defer perf.Track(opts.AtmosConfig, "ci.createPlanfileStore")()

	artOpts := artifact.StoreOptions{
		AtmosConfig: opts.AtmosConfig,
	}

	// Use the default store from configuration if available.
	if opts.AtmosConfig != nil {
		planfilesConfig := opts.AtmosConfig.Components.Terraform.Planfiles
		if planfilesConfig.Default != "" {
			if storeSpec, ok := planfilesConfig.Stores[planfilesConfig.Default]; ok {
				artOpts.Type = storeSpec.Type
				artOpts.Options = storeSpec.Options
				backend, err := artifact.NewStore(artOpts)
				if err != nil {
					return nil, err
				}
				return adapter.NewStore(backend), nil
			}
		}
	}

	// Fall back to environment-based detection.
	if envOpts := detectStoreFromEnv(); envOpts != nil {
		envOpts.AtmosConfig = opts.AtmosConfig
		backend, err := artifact.NewStore(*envOpts)
		if err != nil {
			return nil, err
		}
		return adapter.NewStore(backend), nil
	}

	// Default to local storage.
	artOpts.Type = "local"
	artOpts.Options = map[string]any{
		"path": ".atmos/planfiles",
	}
	backend, err := artifact.NewStore(artOpts)
	if err != nil {
		return nil, err
	}
	return adapter.NewStore(backend), nil
}

// detectStoreFromEnv detects the artifact store from environment variables.
func detectStoreFromEnv() *artifact.StoreOptions {
	defer perf.Track(nil, "ci.detectStoreFromEnv")()

	// Check for S3 configuration.
	if bucket := os.Getenv("ATMOS_PLANFILE_BUCKET"); bucket != "" {
		return &artifact.StoreOptions{
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
		return &artifact.StoreOptions{
			Type: "github-artifacts",
			Options: map[string]any{
				"prefix": "planfile",
			},
		}
	}

	return nil
}

// extractEventPrefix extracts the prefix from a hook event.
// Example: "before.terraform.plan" → "before".
func extractEventPrefix(event string) string {
	parts := strings.Split(event, ".")
	if len(parts) >= 1 {
		return parts[0]
	}
	return ""
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
