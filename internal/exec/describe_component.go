package exec

import (
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteDescribeComponentCmd executes `describe component` command
func ExecuteDescribeComponentCmd(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument `component`")
	}

	_, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return err
	}

	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	format, err := flags.GetString("format")
	if err != nil {
		return err
	}

	file, err := flags.GetString("file")
	if err != nil {
		return err
	}

	processTemplates, err := flags.GetBool("process-templates")
	if err != nil {
		return err
	}

	query, err := flags.GetString("query")
	if err != nil {
		return err
	}

	component := args[0]

	componentSection, err := ExecuteDescribeComponent(component, stack, processTemplates)
	if err != nil {
		return err
	}

	var res any

	if query != "" {
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
		if err != nil {
			return err
		}

		res, err = u.EvaluateYqExpression(&atmosConfig, componentSection, query)
		if err != nil {
			return err
		}
	} else {
		res = componentSection
	}

	err = printOrWriteToFile(format, file, res)
	if err != nil {
		return err
	}

	return nil
}

// ExecuteDescribeComponent describes component config
func ExecuteDescribeComponent(
	component string,
	stack string,
	processTemplates bool,
) (map[string]any, error) {
	var configAndStacksInfo schema.ConfigAndStacksInfo
	configAndStacksInfo.ComponentFromArg = component
	configAndStacksInfo.Stack = stack

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	configAndStacksInfo.ComponentType = "terraform"
	configAndStacksInfo, err = ProcessStacks(atmosConfig, configAndStacksInfo, true, processTemplates)
	if err != nil {
		configAndStacksInfo.ComponentType = "helmfile"
		configAndStacksInfo, err = ProcessStacks(atmosConfig, configAndStacksInfo, true, processTemplates)
		if err != nil {
			return nil, err
		}
	}

	return configAndStacksInfo.ComponentSection, nil
}

// FilterAbstractComponents This function removes abstract components and returns the list of components
func FilterAbstractComponents(componentsMap map[string]any) []string {
	if componentsMap == nil {
		return []string{}
	}
	components := make([]string, 0)
	for _, k := range lo.Keys(componentsMap) {
		componentMap, ok := componentsMap[k].(map[string]any)
		if !ok {
			components = append(components, k)
			continue
		}

		metadata, ok := componentMap["metadata"].(map[string]any)
		if !ok {
			components = append(components, k)
			continue
		}
		if componentType, ok := metadata["type"].(string); ok && componentType == "abstract" {
			continue
		}
		if componentEnabled, ok := metadata["enabled"].(bool); ok && !componentEnabled {
			continue
		}
		components = append(components, k)
	}
	return components
}
