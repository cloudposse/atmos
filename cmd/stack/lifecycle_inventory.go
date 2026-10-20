package stack

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component/custom"
	cfg "github.com/cloudposse/atmos/pkg/config"
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

// checkLifecycleAuth checks every selected component before any child starts. It
// only reads configuration; identity selection and authentication stay in the child.
func checkLifecycleAuth(cmd *cobra.Command, run *lifecycleRun) error {
	if run.options.dryRun || run.options.identity == cfg.IdentityFlagDisabledValue {
		return nil
	}
	ids := make([]string, 0, len(run.plan.Graph.Nodes))
	for id := range run.plan.Graph.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := cmd.Context().Err(); err != nil {
			return err
		}
		node := run.plan.Graph.Nodes[id]
		merged, err := auth.MergeComponentAuthFromConfig(&run.config.Auth, node.Metadata, run.config, cfg.AuthSectionName)
		if err == nil {
			err = checkLifecycleIdentity(merged, run.options.identity)
		}
		if err != nil {
			return lifecycleAuthConfigError(run.config, err, lifecycle.Label(node))
		}
	}
	return nil
}

// checkLifecycleIdentity validates a named identity or configured defaults without
// choosing among identities or changing the native command's no-default behavior.
func checkLifecycleIdentity(config *schema.AuthConfig, requested string) error {
	if requested == cfg.IdentityFlagSelectValue {
		return nil
	}
	if requested == "" {
		for name := range config.Identities {
			if config.Identities[name].Default && config.Identities[name].Kind == "" {
				return missingLifecycleIdentity(name)
			}
		}
		return nil
	}
	name := requested
	if _, ok := config.Identities[name]; !ok {
		if _, known := config.IdentityCaseMap[strings.ToLower(name)]; known {
			name = strings.ToLower(name)
		}
	}
	identity, ok := config.Identities[name]
	if !ok || identity.Kind == "" {
		return missingLifecycleIdentity(requested)
	}
	return nil
}

func missingLifecycleIdentity(name string) error {
	return errUtils.Build(errUtils.ErrIdentityNotFound).
		WithExplanationf("Identity `%s` is not defined with a kind in the loaded auth configuration.", name).
		WithContext("identity", name).
		WithHint("Run `atmos auth list` to see available identities, or define the identity in `atmos.yaml` or the active profile.").Err()
}

func lifecycleAuthConfigError(config *schema.AtmosConfiguration, err error, component string) error {
	builder := errUtils.Build(err).
		WithHintf("Fix authentication for `%s` and retry; no component operations were started.", component)
	identity, ok := errUtils.GetContext(err, "identity")
	if !ok || cfg.HasExplicitProfile() {
		return builder.Err()
	}
	// Reuse profile discovery, but never prompt/re-exec here: component and stack
	// selections may have been prompted and must not be lost during preflight.
	profiles, _ := cfg.ProfilesWithIdentity(config, identity)
	if len(profiles) == 1 {
		builder.WithHintf("Re-run with `--profile=%s` to load identity `%s`.", profiles[0], identity)
	} else if len(profiles) > 1 {
		sort.Strings(profiles)
		builder.WithHintf("Identity `%s` is defined in profiles: %s. Re-run with `--profile=<name>` to load one.", identity, strings.Join(profiles, ", "))
	}
	return builder.Err()
}

// checkLifecycleExecution checks approval requirements and identity configuration
// before starting any component operations.
func checkLifecycleExecution(cmd *cobra.Command, run *lifecycleRun) error {
	if err := checkLifecycleApproval(run.plan, run.options); err != nil {
		return err
	}
	return checkLifecycleAuth(cmd, run)
}
