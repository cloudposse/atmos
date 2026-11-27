package shared

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"al.essio.dev/pkg/shellescape"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// SelectPlanPath finds the named deployments of a physical root before plan hooks run.
// The caller uses the chosen address for both hooks and execution.
func SelectPlanPath(cmd *cobra.Command, info *schema.ConfigAndStacksInfo) (string, string, error) {
	defer perf.Track(nil, "shared.SelectPlanPath")()

	config, err := initCliConfig(*info, true)
	if err != nil {
		return "", "", err
	}
	root, err := os.Stat(info.ComponentFromArg)
	if err != nil {
		return "", "", err
	}
	stacks, err := executeDescribeStacks(&config, info.Stack, nil, []string{"terraform"}, nil, false, false, false, false, nil, nil)
	if err != nil {
		return "", "", err
	}
	targets := matchingPlanTargets(&config, stacks, root)
	commands := slices.Sorted(maps.Keys(targets))
	switch len(commands) {
	case 0:
		return "", "", fmt.Errorf("%w: no registered Terraform component uses %s", errUtils.ErrComponentNotInStack, info.ComponentFromArg)
	case 1:
		target := targets[commands[0]]
		return target.component, target.stack, nil
	}
	if ci.ModeEnabled(cmd) || !isInteractiveFn() {
		return "", "", errUtils.Build(errUtils.ErrAmbiguousComponentPath).
			WithExplanationf("Several deployments use %s. Choose a component and stack.", info.ComponentFromArg).
			WithExample("```shell\n" + strings.Join(commands, "\n") + "\n```").Err()
	}
	choice, err := selectFromOptions("deployment", "Which deployment should be planned?", commands)
	if err != nil {
		return "", "", err
	}
	target := targets[choice]
	return target.component, target.stack, nil
}

type planTarget struct{ component, stack string }

func matchingPlanTargets(config *schema.AtmosConfiguration, stacks map[string]any, root os.FileInfo) map[string]planTarget {
	targets := make(map[string]planTarget)
	for stack, raw := range stacks {
		stackData, _ := raw.(map[string]any)
		components, _ := stackData["components"].(map[string]any)
		terraform, _ := components["terraform"].(map[string]any)
		for name, rawEntry := range terraform {
			entry, _ := rawEntry.(map[string]any)
			if !IsComponentDeployable(entry) {
				continue
			}
			path := planSourcePath(config, name, entry, root.IsDir())
			if dir, err := os.Stat(path); err == nil && os.SameFile(root, dir) {
				command := shellescape.QuoteCommand([]string{"atmos", "terraform", "plan", name, "-s", stack})
				targets[command] = planTarget{name, stack}
			}
		}
	}
	return targets
}

func planSourcePath(config *schema.AtmosConfiguration, name string, entry map[string]any, directory bool) string {
	settings, _ := entry["settings"].(map[string]any)
	configuration, _ := settings["configuration"].(map[string]any)
	if configuration["provider"] == "terragrunt" && schema.ConfigurationOwned(settings) {
		source, _ := configuration["source"].(string)
		if source == "" {
			return ""
		}
		if directory {
			source = filepath.Dir(source)
		}
		return filepath.Join(config.BasePathAbsolute, source)
	}
	physical, _ := entry["component"].(string)
	return filepath.Join(config.TerraformDirAbsolutePath, cmp.Or(physical, name))
}
