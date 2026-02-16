package hooks

import (
	"fmt"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ci"
	_ "github.com/cloudposse/atmos/pkg/ci/plugins/terraform" // Register terraform CI provider.
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
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

	sections, err := e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
		Component:            info.ComponentFromArg,
		Stack:                info.Stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 []string{},
		AuthManager:          nil,
	})
	if err != nil {
		return &Hooks{}, fmt.Errorf("failed to execute describe component: %w", err)
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

	for _, hook := range h.items {
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
func RunCIHooks(event HookEvent, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, output string, forceCIMode bool) error {
	defer perf.Track(atmosConfig, "hooks.RunCIHooks")()

	log.Debug("Running CI hooks", "event", event, "force_ci", forceCIMode)

	// Check if CI is enabled in config or forced via flag.
	if !forceCIMode && atmosConfig != nil && !atmosConfig.CI.Enabled {
		log.Debug("CI integration disabled in config")
		return nil
	}

	// Execute CI actions based on provider bindings.
	return ci.Execute(ci.ExecuteOptions{
		Event:       string(event),
		AtmosConfig: atmosConfig,
		Info:        info,
		Output:      output,
		ForceCIMode: forceCIMode,
	})
}
