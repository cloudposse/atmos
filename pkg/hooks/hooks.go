package hooks

import (
	"fmt"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ci"
	_ "github.com/cloudposse/atmos/pkg/ci/executor"          // Register executor with ci.Execute.
	_ "github.com/cloudposse/atmos/pkg/ci/plugins/terraform" // Register terraform CI plugin.
	_ "github.com/cloudposse/atmos/pkg/ci/providers/generic" // Register generic CI provider.
	_ "github.com/cloudposse/atmos/pkg/ci/providers/github"  // Register GitHub Actions CI provider.
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

type Hooks struct {
	config *schema.AtmosConfiguration
	info   *schema.ConfigAndStacksInfo
	items  map[string]Hook
}

func (h Hooks) HasHooks() bool {
	return len(h.items) > 0
}

func GetHooks(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*Hooks, error) {
	if info.ComponentFromArg == "" || info.Stack == "" {
		return &Hooks{
			config: atmosConfig,
			info:   info,
			items:  nil,
		}, nil
	}

	// ProcessYamlFunctions must be false here. GetHooks runs in PreRunE before
	// auth credentials are provisioned (AuthManager is nil). YAML functions like
	// !terraform.state need AWS credentials to read S3 state — processing them
	// here would fail. The hooks section itself is static config (event names,
	// commands, store names) and does not use YAML functions.
	sections, err := e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
		Component:            info.ComponentFromArg,
		Stack:                info.Stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: false,
		Skip:                 []string{},
		AuthManager:          nil,
	})
	if err != nil {
		return &Hooks{}, err
	}

	hooksSection, ok := sections["hooks"].(map[string]any)
	if !ok {
		// No hooks defined or wrong type, return empty hooks.
		return &Hooks{
			config: atmosConfig,
			info:   info,
			items:  nil,
		}, nil
	}

	yamlData, err := yaml.Marshal(hooksSection)
	if err != nil {
		return &Hooks{}, fmt.Errorf("failed to marshal hooksSection: %w", err)
	}

	var items map[string]Hook
	err = yaml.Unmarshal(yamlData, &items)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal to Hooks: %w", err)
	}

	hooks := Hooks{
		config: atmosConfig,
		info:   info,
		items:  items,
	}

	return &hooks, nil
}

func (h Hooks) RunAll(event HookEvent, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, cmd *cobra.Command, args []string) error {
	log.Debug("Running hooks", "count", len(h.items))

	for name, hook := range h.items {
		if !hook.MatchesEvent(event) {
			log.Debug("Skipping hook, event not in hook events list", "hook", name, "event", event, "hook_events", hook.Events)
			continue
		}

		var hookCmd Command
		var err error

		switch hook.Command {
		case "store":
			hookCmd, err = NewStoreCommand(atmosConfig, info)
		// CI commands are deprecated - use RunCIHooks instead which automatically
		// triggers CI actions based on component provider bindings.
		case "ci.check", "ci.output", "ci.summary", "ci.upload", "ci.download":
			log.Debug("CI hook command deprecated, use RunCIHooks instead", "command", hook.Command)
			continue
		default:
			log.Debug("Unknown hook command", "command", hook.Command)
			continue
		}

		if err != nil {
			return err
		}

		if err = hookCmd.RunE(&hook, event, cmd, args); err != nil {
			return err
		}
	}
	return nil
}

// RunCIHooks executes CI actions based on provider bindings.
// This is called automatically during command execution if CI is enabled.
// The output parameter is the command output to process (e.g., terraform plan output).
// The forceCIMode parameter forces CI mode even when environment detection fails (--ci flag).
// The cmdErr parameter is the error from the command execution (nil on success).
func RunCIHooks(event HookEvent, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, output string, forceCIMode bool, cmdErr error) error {
	defer perf.Track(atmosConfig, "hooks.RunCIHooks")()

	log.Debug("Running CI hooks", "event", event, "force_ci", forceCIMode)

	// ci.enabled in atmos.yaml is the authority — if not set or false, CI is off.
	// The --ci flag / ATMOS_CI env var only controls provider fallback (generic vs auto-detect),
	// it cannot override a disabled config.
	if atmosConfig != nil && !atmosConfig.CI.Enabled {
		log.Debug("CI integration disabled in config (ci.enabled is not true)")
		return nil
	}

	// CI integration is experimental. Check settings.experimental to decide
	// whether to proceed, warn, or block — mirroring command-level behavior.
	// This runs after the ci.enabled check so the warning only appears when CI is active.
	if atmosConfig != nil {
		if err := checkExperimental(atmosConfig); err != nil {
			return err
		}
	}

	// Execute CI actions based on provider bindings.
	return ci.Execute(&ci.ExecuteOptions{
		Event:        string(event),
		AtmosConfig:  atmosConfig,
		Info:         info,
		Output:       output,
		ForceCIMode:  forceCIMode,
		CommandError: cmdErr,
	})
}

// ciExperimentalFeature is the feature name used in experimental warnings for CI hooks.
const ciExperimentalFeature = "ci"

// checkExperimental checks settings.experimental and returns an error if CI
// hooks should not run. Mirrors the command-level experimental gating in cmd/root.go.
func checkExperimental(atmosConfig *schema.AtmosConfiguration) error {
	mode := atmosConfig.Settings.Experimental
	if mode == "" {
		mode = "warn" // Default matches command-level behavior.
	}

	switch mode {
	case "silence":
		// Proceed without any warning.
		return nil
	case "disable":
		log.Debug("CI hooks disabled by settings.experimental=disable")
		return errUtils.Build(errUtils.ErrExperimentalDisabled).
			WithContext("feature", ciExperimentalFeature).
			WithHint("Enable with settings.experimental: warn").
			Err()
	case "warn":
		ui.Experimental(ciExperimentalFeature)
		return nil
	case "error":
		ui.Experimental(ciExperimentalFeature)
		return errUtils.Build(errUtils.ErrExperimentalRequiresIn).
			WithContext("feature", ciExperimentalFeature).
			WithHint("Enable with settings.experimental: warn").
			Err()
	default:
		// Unknown mode — treat as warn for forward compatibility.
		ui.Experimental(ciExperimentalFeature)
		return nil
	}
}
