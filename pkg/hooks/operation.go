package hooks

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	yamlv2 "gopkg.in/yaml.v2"
	yamlv3 "gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// OperationContext describes a repo-level operation (one that has no stack or component
// instance, such as `atmos docs generate`) whose lifecycle events run hooks.
type OperationContext struct {
	// Type is the operation family. It becomes the policy component_type and the
	// template/env namespace (for example "docs").
	Type string
	// Name identifies the operation target. It becomes the policy component and the
	// ATMOS_COMPONENT value (for example the docs.generate doc-type key).
	Name string
	// ConfigPath is where the hooks are declared (for example docs.generate.readme.hooks).
	// It names the location in configuration errors.
	ConfigPath string
	// Env holds extra ATMOS_* variables exported to every hook run.
	Env map[string]string
	// Data is exposed to hook templates under the Type key (for example `.docs.output_path`).
	Data map[string]any
}

// NewOperationHooks builds hooks for a repo-level operation from an untyped hooks map
// (for example docs.generate.<type>.hooks). Unlike GetHooks it does not describe a
// component: the declarations are decoded and validated directly, while the governing
// policy bundle is loaded exactly as it is for component hooks. The result works with
// SetOutcome and RunOperation (or RunAll).
func NewOperationHooks(atmosConfig *schema.AtmosConfiguration, raw map[string]any, op OperationContext) (*Hooks, error) {
	defer perf.Track(atmosConfig, "hooks.NewOperationHooks")()

	bundle, snapshot, err := loadPolicyGovernance()
	if err != nil {
		return nil, err
	}
	items, normalized, err := decodeOperationHooks(atmosConfig, raw, op.ConfigPath)
	if err != nil {
		return nil, err
	}
	warnForeignEvents(items, op.Type)
	h := &Hooks{
		config: atmosConfig,
		info: &schema.ConfigAndStacksInfo{
			ComponentType:    op.Type,
			ComponentFromArg: op.Name,
		},
		items:        items,
		operation:    true,
		policyBundle: bundle,
		policyCI:     snapshot,
	}
	h.sections = map[string]any{}
	if normalized != nil {
		h.sections["hooks"] = normalized
	}
	h.SetOperationContext(op)
	return h, nil
}

// decodeOperationHooks decodes and validates the declarations. The second return value is a
// type-normalized copy used as the raw section for execution-time template rendering.
func decodeOperationHooks(atmosConfig *schema.AtmosConfiguration, raw map[string]any, configPath string) (map[string]Hook, map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil, nil
	}
	if err := requireHookMap(raw, configPath); err != nil {
		return nil, nil, err
	}
	data, err := yamlv2.Marshal(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errUtils.ErrInvalidHooksSection, err)
	}
	var items map[string]Hook
	if err := yamlv2.Unmarshal(data, &items); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errUtils.ErrInvalidHooksSection, err)
	}
	var edition string
	if atmosConfig != nil {
		edition = atmosConfig.Edition
	}
	if err := ValidateEvents(edition, items); err != nil {
		return nil, nil, err
	}
	if err := validateOperationHooks(items); err != nil {
		return nil, nil, err
	}
	var normalized map[string]any
	if err := yamlv3.Unmarshal(data, &normalized); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errUtils.ErrInvalidHooksSection, err)
	}
	return items, normalized, nil
}

// foreignOperationEvents returns, per hook name, the declared events that belong to another
// operation family and so can never fire for this one.
func foreignOperationEvents(items map[string]Hook, opType string) map[string][]string {
	foreign := map[string][]string{}
	for name := range items {
		for _, event := range items[name].Events {
			parts := strings.Split(string(HookEvent(event).Normalize()), ".")
			if len(parts) < 2 || parts[1] != opType {
				foreign[name] = append(foreign[name], event)
			}
		}
	}
	return foreign
}

// warnForeignEvents logs a warning for hooks whose events are valid but belong to another
// operation. Such hooks are legal, so this is not an error.
func warnForeignEvents(items map[string]Hook, opType string) {
	if opType == "" {
		return
	}
	foreign := foreignOperationEvents(items, opType)
	names := make([]string, 0, len(foreign))
	for name := range foreign {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		log.Warn("Hook declares events that never fire for this operation",
			"hook", name, "events", foreign[name],
			"supported", []string{"before." + opType + ".generate", "after." + opType + ".generate"})
	}
}

// validateOperationHooks applies the checks governing policy hooks get, so a misconfigured hook
// fails before the operation starts instead of warning at run time. Hooks are checked in name order.
func validateOperationHooks(items map[string]Hook) error {
	names := make([]string, 0, len(items))
	for name := range items {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		hook := items[name]
		if isDeprecatedCIKind(hook.Kind) {
			continue
		}
		label := fmt.Sprintf("hook %q (kind %s", name, hook.Kind)
		if hook.Type != "" {
			label += ", type " + hook.Type
		}
		label += ")"
		kind, ok := GetKind(hook.Kind)
		if !ok {
			return fmt.Errorf("%s: %w", label, unknownHookKindError(name, hook.Kind))
		}
		if err := validatePolicyHookDefinition(name, &hook, kind); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
	}
	return nil
}

// requireHookMap checks that every hook is a map of hook definition fields. A list of hooks or a
// bare value has the wrong shape and would otherwise surface as a decoder error.
func requireHookMap(raw map[string]any, configPath string) error {
	if configPath == "" {
		configPath = "hooks"
	}
	names := make([]string, 0, len(raw))
	for name := range raw {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		switch raw[name].(type) {
		case map[string]any, map[any]any:
			continue
		}
		cause := fmt.Errorf("%w: %s.%s must be a map of hook fields, not a list or a plain value", errUtils.ErrInvalidHooksSection, configPath, name)
		return errUtils.Build(cause).
			WithExplanationf("%s must be a map of hook name to hook definition, but %q is not a map.", configPath, name).
			WithHintf("Write each hook under its own name, for example:\n%s:\n  publish:\n    kind: step\n    events: [after.docs.generate]\n    type: http", configPath).
			WithContext("path", configPath).
			Err()
	}
	return nil
}

// SetOperationContext replaces the environment and template data exposed to hooks. Operations
// whose context changes over their lifecycle (for example an output path that is only known
// after generation) call it between events.
func (h *Hooks) SetOperationContext(op OperationContext) {
	defer perf.Track(h.config, "hooks.Hooks.SetOperationContext")()

	h.operationEnv = maps.Clone(op.Env)
	if h.sections == nil {
		h.sections = map[string]any{}
	}
	if op.Type != "" {
		h.sections[op.Type] = maps.Clone(op.Data)
	}
}

// RunOperation runs the hooks registered for event using the context the Hooks value was
// built with. Callers set the lifecycle result with SetOutcome first.
func (h *Hooks) RunOperation(event HookEvent, cmd *cobra.Command) error {
	defer perf.Track(h.config, "hooks.Hooks.RunOperation")()

	return h.RunAll(event, h.config, h.info, cmd, nil)
}
