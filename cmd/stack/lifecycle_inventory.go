package stack

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component/custom"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
)

// lifecycleEvalSections are the only component sections the lifecycle graph needs. Vars,
// hooks, and runtime output references stay unevaluated until the selected child starts.
var lifecycleEvalSections = []string{"dependencies", "metadata", "settings"}

// loadLifecycleInventory discovers the lifecycle handlers and describes the stack inventory.
//
// Handler discovery comes first so that every component kind declared by a custom command
// (component.type) is registered before stacks are described. Without that, components of a
// custom kind are not part of the inventory and silently vanish from the plan.
func loadLifecycleInventory(cmd *cobra.Command, config *schema.AtmosConfiguration, verb string, authManager auth.AuthManager) (map[string]any, map[string]*lifecycle.Handler, error) {
	defer perf.Track(config, "stack.loadLifecycleInventory")()

	handlers := discoverLifecycleCommands(cmd.Root(), config, verb)
	kinds, err := registerCustomComponentKinds(config)
	if err != nil {
		return nil, nil, err
	}
	stacks, err := e.ExecuteDescribeStacksWithEvalSections(
		config, "", nil, nil, nil, false, true, false, true, nil, authManager, nil, nil,
		e.DescribeStacksErrorOptions{ExtraComponentTypes: kinds}, lifecycleEvalSections,
	)
	if err != nil {
		return nil, nil, err
	}
	return stacks, handlers, nil
}

// The registerCustomComponentKinds function registers every component kind declared by a custom
// command the same way running that command does, and returns the kind names in a stable order.
// A kind declared by several commands, such as deploy and destroy, registers once.
func registerCustomComponentKinds(config *schema.AtmosConfiguration) ([]string, error) {
	declared := make(map[string]string)
	collectCustomComponentKinds(config.Commands, declared)
	kinds := make([]string, 0, len(declared))
	for kind := range declared {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		if err := custom.EnsureRegistered(kind, customKindBasePath(config, kind, declared[kind])); err != nil {
			return nil, fmt.Errorf("registering component kind %q: %w", kind, err)
		}
	}
	return kinds, nil
}

// collectCustomComponentKinds walks the custom command tree and records each declared kind with
// the first non-empty component.base_path found for it.
func collectCustomComponentKinds(commands []schema.Command, declared map[string]string) {
	for i := range commands {
		c := &commands[i]
		if c.Component != nil && c.Component.Type != "" {
			if current, seen := declared[c.Component.Type]; !seen || current == "" {
				declared[c.Component.Type] = c.Component.BasePath
			}
		}
		collectCustomComponentKinds(c.Commands, declared)
	}
}

// customKindBasePath mirrors how custom commands register a kind: the command's own
// component.base_path, then the kind's plugin configuration, then components/<kind>.
func customKindBasePath(config *schema.AtmosConfiguration, kind, declared string) string {
	if declared != "" {
		return declared
	}
	if plugin, ok := config.Components.Plugins[kind].(map[string]any); ok {
		if path, ok := plugin["base_path"].(string); ok && path != "" {
			return path
		}
	}
	return filepath.ToSlash(filepath.Join("components", kind))
}
