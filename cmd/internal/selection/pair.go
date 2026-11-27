// Package selection resolves single-component commands against the stack inventory.
package selection

import (
	"fmt"
	"maps"
	"slices"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ci"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Interactive reports whether missing instance arguments may be filled in.
func Interactive(cmd *cobra.Command) bool {
	defer perf.Track(nil, "selection.Interactive")()
	return flags.IsInteractive() && !ci.ModeEnabled(cmd)
}

// Resolve fills missing component and stack arguments from compatible deployments.
func Resolve(cmd *cobra.Command, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "selection.Resolve")()
	if (info.ComponentFromArg != "" && info.Stack != "") || !Interactive(cmd) {
		return nil
	}
	config, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return err
	}
	stacks, err := e.ExecuteDescribeStacksWithAuthDisabled(&config, info.Stack, nil, []string{info.ComponentType}, []string{"metadata"}, false, false, false, false, nil, nil, true)
	if err != nil {
		return err
	}
	return selectPair(info, stacks, flags.PromptForValue)
}

func selectPair(info *schema.ConfigAndStacksInfo, inventory map[string]any, prompt func(string, string, []string) (string, error)) error {
	options := deploymentsByComponent(info, inventory)
	if len(options) == 0 {
		switch {
		case info.ComponentFromArg != "":
			return fmt.Errorf("%w: no stack contains a deployable %s component named %q", errUtils.ErrNoOptionsAvailable, info.ComponentType, info.ComponentFromArg)
		case info.Stack != "":
			return fmt.Errorf("%w: stack %q contains no deployable %s components", errUtils.ErrNoOptionsAvailable, info.Stack, info.ComponentType)
		default:
			return fmt.Errorf("%w: no deployable %s components were found", errUtils.ErrNoOptionsAvailable, info.ComponentType)
		}
	}
	name, err := choose("component", "Choose a "+info.ComponentType+" component", slices.Sorted(maps.Keys(options)), prompt)
	if err != nil {
		return err
	}
	stacks := options[name]
	slices.Sort(stacks)
	stack, err := choose("stack", "Choose a stack for "+name, stacks, prompt)
	if err != nil {
		return err
	}
	info.ComponentFromArg, info.Stack = name, stack
	return nil
}

func deploymentsByComponent(info *schema.ConfigAndStacksInfo, inventory map[string]any) map[string][]string {
	options := map[string][]string{}
	for stack, raw := range inventory {
		if info.Stack != "" && stack != info.Stack {
			continue
		}
		doc, _ := raw.(map[string]any)
		kinds, _ := doc["components"].(map[string]any)
		components, _ := kinds[info.ComponentType].(map[string]any)
		for name, raw := range components {
			if (info.ComponentFromArg == "" || info.ComponentFromArg == name) && deployable(raw) {
				options[name] = append(options[name], stack)
			}
		}
	}
	return options
}

func deployable(raw any) bool {
	component, ok := raw.(map[string]any)
	metadata, _ := component["metadata"].(map[string]any)
	enabled, specified := metadata["enabled"].(bool)
	return ok && !e.IsComponentAbstract(metadata) && (!specified || enabled)
}

func choose(field, title string, options []string, prompt func(string, string, []string) (string, error)) (string, error) {
	if len(options) == 1 {
		return options[0], nil
	}
	return prompt(field, title, options)
}
