package hooks

import (
	"fmt"
	u "github.com/cloudposse/atmos/pkg/utils"
	"os"
	"strings"

	"github.com/charmbracelet/log"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

//type WriteCommand struct {
//	//StoreCommand
//	StoreCommand StoreCommand
//}

type WriteOutput struct {
	Content      string            `yaml:"content"`
	Replacements map[string]string `json:"replacements"`
}
type WriteHook struct {
	Hook
	Name   string                 `yaml:"name"`
	Output map[string]WriteOutput `yaml:"output"`
	config *schema.AtmosConfiguration
	info   *schema.ConfigAndStacksInfo
}

func GetWriteHook(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, Name string) (*WriteHook, error) {
	sections, err := e.ExecuteDescribeComponent(info.ComponentFromArg, info.Stack, true, true, []string{})
	if err != nil {
		return nil, fmt.Errorf("failed to execute describe component: %w", err)
	}

	hooksSection := sections["hooks"].(map[string]any)

	yamlData, err := yaml.Marshal(hooksSection)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal hooksSection: %w", err)
	}

	var items map[string]WriteHook
	err = yaml.Unmarshal(yamlData, &items)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal to Hooks: %w", err)
	}

	for k, v := range items {
		v.config = atmosConfig
		v.info = info

		if k == Name {
			return &v, nil
		}
	}

	return nil, fmt.Errorf("failed to find hook %q in config", Name)
}

func (c *WriteHook) processWriteCommand(hook *Hook) error {
	log.Debug("processWriteCommand", "WriteHook", c)
	if len(c.Output) == 0 {
		log.Info("skipping hook. no output configured.", "hook", hook.Name, "outputs", hook.Outputs)
		return nil
	}

	log.Debug("executing 'after-terraform-apply' hook", "hook", hook.Name, "command", hook.Command)
	for fileName, output := range c.Output {
		var newContent = fmt.Sprintf("# This file is stack generated\n# Hook: `%s`\n# Component: %s\n# Stack: %s\n# StackFile: %s\n", c.Name, c.info.Component, c.info.Stack, c.info.StackFile) + output.Content

		replaceMap := make(map[string]any)
		for replaceKey, replaceOutputKey := range output.Replacements {
			//for _, replaceOutputKey := range output.Replacements {
			_, replaceOutput := c.getOutputValue(replaceOutputKey)
			d, _ := u.ConvertToYAML(replaceOutput)
			replaceMap[replaceKey] = d

			//yamlData, err := yaml.Marshal(replaceOutput)
			//if err != nil {
			//	return fmt.Errorf("failed to marshal output: %w", err)
			//}
			//newContent = strings.ReplaceAll(newContent, replaceKey, d)

		}
		log.Info("ReplacementMap", "replaceMap", replaceMap)
		//newContent = e.ProcessCustomYamlTags(c.config, replaceMap, newContent, [])
		//convertedReplaceMap := schema.AtmosSectionMapType(replaceMap)
		result, _ := e.ProcessCustomYamlTags(*c.config, replaceMap, newContent, []string{})
		log.Info("result", "result", result)
		newContent, err := e.ProcessTmpl("write-hook", newContent, result, true)

		//if err != nil {
		//	fmt.Errorf(err.Error())
		//}
		//content, _ := u.ConvertToYAML(newContent)
		err = os.WriteFile(fileName, []byte(newContent), 0644)
		//content, _ := yaml.Marshal(result)
		//err := os.WriteFile(fileName, content, 0644)
		if err != nil {
			return err
		}
	}
	//for key, value := range c.Output. {
	//	outputKey, outputValue := c.getOutputValue(value)
	//
	//	err := c.writeOutput(hook, key, outputKey, outputValue)
	//	if err != nil {
	//		return err
	//	}
	//}
	return nil
}

// getOutputValue gets an output from terraform
func (c *WriteHook) getOutputValue(value string) (string, any) {
	outputKey := strings.TrimPrefix(value, ".")
	var outputValue any

	if strings.Index(value, ".") == 0 {
		outputValue = e.GetTerraformOutput(c.config, c.info.Stack, c.info.ComponentFromArg, outputKey, true)
	} else {
		outputValue = value
	}
	return outputKey, outputValue
}

// storeOutput puts the value of the output in the store
func (c *WriteHook) writeOutput(hook *Hook, key string, outputKey string, outputValue any) error {
	log.Debug("Writing Output Hook", "OutputFile", c.Output, "hook", hook)

	return nil
}

func (c *WriteHook) RunE(hook *Hook, event HookEvent, cmd *cobra.Command, args []string) error {
	return c.processWriteCommand(hook)
}
