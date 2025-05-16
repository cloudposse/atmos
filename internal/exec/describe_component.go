package exec

import (
	log "github.com/charmbracelet/log"
	"github.com/samber/lo"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

type DescribeComponentParams struct {
	Component            string
	Stack                string
	ProcessTemplates     bool
	ProcessYamlFunctions bool
	Skip                 []string
	Query                string
	Pager                string
	Format               string
	File                 string
}

type DescribeComponentExec struct {
	pageCreator              pager.PageCreator
	printOrWriteToFile       func(atmosConfig *schema.AtmosConfiguration, format string, file string, data any) error
	IsTTYSupportForStdout    func() bool
	initCliConfig            func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error)
	executeDescribeComponent func(component string, stack string, processTemplates bool, processYamlFunctions bool, skip []string) (map[string]any, error)
	evaluateYqExpression     func(atmosConfig *schema.AtmosConfiguration, data any, yq string) (any, error)
}

func NewDescribeComponentExec() *DescribeComponentExec {
	return &DescribeComponentExec{
		printOrWriteToFile:       printOrWriteToFile,
		IsTTYSupportForStdout:    term.IsTTYSupportForStdout,
		pageCreator:              pager.New(),
		initCliConfig:            cfg.InitCliConfig,
		executeDescribeComponent: ExecuteDescribeComponent,
		evaluateYqExpression:     u.EvaluateYqExpression,
	}
}

func (d *DescribeComponentExec) ExecuteDescribeComponentCmd(describeComponentParams DescribeComponentParams) error {
	component := describeComponentParams.Component
	stack := describeComponentParams.Stack
	processTemplates := describeComponentParams.ProcessTemplates
	processYamlFunctions := describeComponentParams.ProcessYamlFunctions
	skip := describeComponentParams.Skip
	query := describeComponentParams.Query
	pager := describeComponentParams.Pager
	format := describeComponentParams.Format
	file := describeComponentParams.File

	var err error
	var atmosConfig schema.AtmosConfiguration

	atmosConfig, err = d.initCliConfig(schema.ConfigAndStacksInfo{
		ComponentFromArg: component,
		Stack:            stack,
	}, true)
	if err != nil {
		return err
	}

	componentSection, err := d.executeDescribeComponent(
		component,
		stack,
		processTemplates,
		processYamlFunctions,
		skip,
	)
	if err != nil {
		return err
	}

	var res any
	if pager != "" {
		atmosConfig.Settings.Terminal.Pager = pager
	}

	if query != "" {
		res, err = d.evaluateYqExpression(&atmosConfig, componentSection, query)
		if err != nil {
			return err
		}
	} else {
		res = componentSection
	}

	if atmosConfig.Settings.Terminal.IsPagerEnabled() {
		err = d.viewConfig(&atmosConfig, component, format, res)
		switch err.(type) {
		case DescribeConfigFormatError:
			return err
		case nil:
			return nil
		default:
			log.Debug("Failed to use pager")
		}
	}

	err = d.printOrWriteToFile(&atmosConfig, format, file, res)
	if err != nil {
		return err
	}

	return nil
}

func (d *DescribeComponentExec) viewConfig(atmosConfig *schema.AtmosConfiguration, displayName string, format string, data any) error {
	if !d.IsTTYSupportForStdout() {
		return ErrTTYNotSupported
	}
	var content string
	var err error
	switch format {
	case "yaml":
		content, err = u.GetHighlightedYAML(atmosConfig, data)
		if err != nil {
			return err
		}
	case "json":
		content, err = u.GetHighlightedJSON(atmosConfig, data)
		if err != nil {
			return err
		}
	default:
		return DescribeConfigFormatError{
			format,
		}
	}
	if err := d.pageCreator.Run(displayName, content); err != nil {
		return err
	}
	return nil
}

// ExecuteDescribeComponent describes component config
func ExecuteDescribeComponent(
	component string,
	stack string,
	processTemplates bool,
	processYamlFunctions bool,
	skip []string,
) (map[string]any, error) {
	var configAndStacksInfo schema.ConfigAndStacksInfo
	configAndStacksInfo.ComponentFromArg = component
	configAndStacksInfo.Stack = stack
	configAndStacksInfo.ComponentSection = make(map[string]any)

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	configAndStacksInfo.ComponentType = cfg.TerraformComponentType
	configAndStacksInfo, err = ProcessStacks(atmosConfig, configAndStacksInfo, true, processTemplates, processYamlFunctions, skip)
	configAndStacksInfo.ComponentSection[cfg.ComponentTypeSectionName] = cfg.TerraformComponentType
	if err != nil {
		configAndStacksInfo.ComponentType = cfg.HelmfileComponentType
		configAndStacksInfo, err = ProcessStacks(atmosConfig, configAndStacksInfo, true, processTemplates, processYamlFunctions, skip)
		configAndStacksInfo.ComponentSection[cfg.ComponentTypeSectionName] = cfg.HelmfileComponentType
		if err != nil {
			configAndStacksInfo.ComponentSection[cfg.ComponentTypeSectionName] = ""
			return nil, err
		}
	}

	return configAndStacksInfo.ComponentSection, nil
}

// FilterAbstractComponents This function removes abstract components and returns the list of components.
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
