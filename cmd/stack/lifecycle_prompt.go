package stack

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
)

func needsLifecycleStack(o *lifecycleOptions) bool {
	return o.stack == "" && !o.all && !o.affected
}

func needsLifecycleComponent(o *lifecycleOptions) bool {
	return o.component == "" && !o.all && !o.affected && len(o.tags) == 0 && len(o.labels) == 0
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
		return errUtils.Build(fmt.Errorf("%w: component is required without a bulk selection", errUtils.ErrInvalidPositionalArgs)).
			WithHint("Pass a component or run interactively to choose one. Use --all for every component in the stack, or filter with --affected, --tags, or --labels.").
			Err()
	}
	return nil
}

// resolveLifecycleTargets uses the same selector as Terraform, with options
// from the already loaded mixed-kind inventory. Bulk selections skip the component picker.
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
		options := lifecycleComponentNames(stacks, o.stack, handlers)
		if len(options) == 0 {
			return fmt.Errorf("%w: no components define %s in stack %q", errUtils.ErrNoComponentsToSelect, o.verb, o.stack)
		}
		component, err := promptLifecycleTarget(cmd, "component", "Choose a component", options)
		if err != nil {
			return err
		}
		o.component = component
	}
	return nil
}

func promptLifecycleStack(cmd *cobra.Command, o *lifecycleOptions, stacks map[string]any, handlers map[string]*lifecycle.Handler) error {
	var options []string
	for stack := range stacks {
		names := lifecycleComponentNames(stacks, stack, handlers)
		if o.component == "" || slices.Contains(names, o.component) {
			options = append(options, stack)
		}
	}
	if len(options) == 0 {
		return fmt.Errorf("%w: no stacks match component %q", errUtils.ErrNoStacksToSelect, o.component)
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
	if o.component == "" {
		return nil
	}
	for stack, value := range stacks {
		if o.stack != "" && o.stack != stack {
			continue
		}
		if lifecycleStackContainsComponent(value, o.component) {
			return nil
		}
	}
	for i := range affected {
		if affected[i].Component == o.component && (o.stack == "" || o.stack == affected[i].Stack) {
			return nil
		}
	}
	return errUtils.Build(fmt.Errorf("%w: component %q was not found in the selected stacks", errUtils.ErrInvalidComponent, o.component)).
		WithHint("Run `atmos list components` to see the available components.").
		Err()
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
