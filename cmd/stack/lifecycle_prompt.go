package stack

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
)

func needsLifecycleStack(o *lifecycleOptions) bool {
	return o.stack == "" && !o.all && !o.affected
}

func needsLifecycleComponent(o *lifecycleOptions) bool {
	return o.pickComponents
}

// checkLifecyclePromptAvailability fails before loading configuration when a
// missing scope or component cannot be collected interactively.
func checkLifecyclePromptAvailability(o *lifecycleOptions) error {
	if !o.ci && flags.IsInteractive() {
		return nil
	}
	if needsLifecycleStack(o) {
		return errUtils.Build(fmt.Errorf("%w: specify --stack, --all, or --affected", errUtils.ErrInvalidFlag)).
			WithHint("Pass --stack=<stack> or run interactively to choose a stack. Use --all or --affected for selection across stacks.").
			Err()
	}
	if needsLifecycleComponent(o) {
		return errUtils.Build(fmt.Errorf("%w: --component without a value requires an interactive terminal", errUtils.ErrInvalidFlag)).
			WithHint("Pass --component=NAME (repeat or comma-separate names), or run interactively to choose components.").
			Err()
	}
	return nil
}

// resolveLifecycleTargets uses the same selector as Terraform, with options
// from the already loaded mixed-kind inventory. Only bare --component opens its picker.
func resolveLifecycleTargets(cmd *cobra.Command, o *lifecycleOptions, stacks map[string]any, handlers map[string]*lifecycle.Handler) error {
	if err := checkLifecyclePromptAvailability(o); err != nil {
		return err
	}
	if needsLifecycleStack(o) {
		if err := promptLifecycleStack(cmd, o, stacks, handlers); err != nil {
			return err
		}
	}
	if err := checkLifecycleStackExists(stacks, nil, o.stack); err != nil && !o.affected {
		return err
	}
	if needsLifecycleComponent(o) {
		return promptLifecycleComponents(cmd, o, stacks, handlers)
	}
	return nil
}

func promptLifecycleStack(cmd *cobra.Command, o *lifecycleOptions, stacks map[string]any, handlers map[string]*lifecycle.Handler) error {
	var options []string
	for stack := range stacks {
		names := lifecycleComponentNames(stacks, stack, handlers)
		if containsLifecycleComponents(names, o.components) {
			options = append(options, stack)
		}
	}
	if len(options) == 0 {
		return fmt.Errorf("%w: no stacks match components %q", errUtils.ErrNoStacksToSelect, o.components)
	}
	stack, err := promptLifecycleTarget(cmd, "stack", "Choose a stack", options)
	if err != nil {
		return err
	}
	if err := cmd.Flags().Set("stack", stack); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrSetFlag, err)
	}
	o.stack = stack
	return nil
}

func promptLifecycleTarget(cmd *cobra.Command, name, title string, options []string) (string, error) {
	if err := cmd.Context().Err(); err != nil {
		return "", err
	}
	slices.Sort(options)
	value, err := flags.PromptForValue(name, title, options)
	if err != nil {
		return "", err
	}
	if !slices.Contains(options, value) {
		return "", fmt.Errorf("%w: no %s selected", errUtils.ErrMissingInput, name)
	}
	return value, cmd.Context().Err()
}

// lifecycleComponentNames excludes abstract, disabled, and unsupported kinds
// from the component picker. Repeated names across kinds appear once.
func lifecycleComponentNames(stacks map[string]any, stack string, handlers map[string]*lifecycle.Handler) []string {
	section, _ := stacks[stack].(map[string]any)
	kinds, _ := section["components"].(map[string]any)
	var names []string
	for kind, value := range kinds {
		if _, ok := handlers[kind]; !ok {
			continue
		}
		components, _ := value.(map[string]any)
		for name, value := range components {
			component, _ := value.(map[string]any)
			metadata, _ := component["metadata"].(map[string]any)
			if component == nil || metadata["type"] == "abstract" || metadata["enabled"] == false {
				continue
			}
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

func checkLifecycleComponentExists(stacks map[string]any, affected []schema.Affected, o *lifecycleOptions) error {
	for _, name := range o.components {
		if !lifecycleComponentExists(stacks, affected, o.stack, name) {
			return errUtils.Build(fmt.Errorf("%w: component %q was not found in the selected stacks", errUtils.ErrInvalidComponent, name)).
				WithHint("Run `atmos list components` to see the available components.").Err()
		}
	}
	return nil
}

func lifecycleComponentExists(stacks map[string]any, affected []schema.Affected, selectedStack, name string) bool {
	for stack, value := range stacks {
		if (selectedStack == "" || selectedStack == stack) && lifecycleStackContainsComponent(value, name) {
			return true
		}
	}
	return slices.ContainsFunc(affected, func(a schema.Affected) bool {
		return a.Component == name && (selectedStack == "" || selectedStack == a.Stack)
	})
}

func lifecycleStackContainsComponent(value any, name string) bool {
	section, _ := value.(map[string]any)
	kinds, _ := section["components"].(map[string]any)
	for _, value := range kinds {
		components, _ := value.(map[string]any)
		if _, ok := components[name]; ok {
			return true
		}
	}
	return false
}

// lifecycleComponentSelect follows the optional-value sentinel convention.
const lifecycleComponentSelect = "__SELECT__"

func parseLifecycleComponents(cmd *cobra.Command, v *viper.Viper, o *lifecycleOptions) error {
	values := v.GetStringSlice("component")
	if cmd.Flags().Changed("component") && len(values) == 0 {
		return fmt.Errorf("%w: --component requires a nonempty name; use bare --component to choose interactively", errUtils.ErrInvalidFlag)
	}
	for _, name := range values {
		name = strings.TrimSpace(name)
		switch name {
		case lifecycleComponentSelect:
			o.pickComponents = true
		case "":
			return fmt.Errorf("%w: --component names must not be empty", errUtils.ErrInvalidFlag)
		default:
			if !slices.Contains(o.components, name) {
				o.components = append(o.components, name)
			}
		}
	}
	return nil
}

func containsLifecycleComponents(options, names []string) bool {
	for _, name := range names {
		if !slices.Contains(options, name) {
			return false
		}
	}
	return true
}

func promptLifecycleComponents(cmd *cobra.Command, o *lifecycleOptions, stacks map[string]any, handlers map[string]*lifecycle.Handler) error {
	if err := cmd.Context().Err(); err != nil {
		return err
	}
	var options []string
	for stack := range stacks {
		if o.stack == "" || o.stack == stack {
			options = append(options, lifecycleComponentNames(stacks, stack, handlers)...)
		}
	}
	slices.Sort(options)
	options = slices.Compact(options)
	if len(options) == 0 {
		return fmt.Errorf("%w: no components define %s in the selected stacks", errUtils.ErrNoComponentsToSelect, o.verb)
	}
	if !containsLifecycleComponents(options, o.components) {
		return fmt.Errorf("%w: --component names must be available in the selected stacks", errUtils.ErrInvalidComponent)
	}
	selected, err := flags.PromptForMultipleValuesWithPreselection("component", "Choose components", options, o.components)
	if err != nil {
		return err
	}
	if len(selected) == 0 || !containsLifecycleComponents(options, selected) {
		return fmt.Errorf("%w: no components selected", errUtils.ErrMissingInput)
	}
	o.components = selected
	o.pickComponents = false
	return cmd.Context().Err()
}
