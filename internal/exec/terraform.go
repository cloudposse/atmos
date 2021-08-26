package exec

import (
	c "atmos/internal/config"
	s "atmos/internal/stack"
	u "atmos/internal/utils"
	"fmt"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"path"
	"strings"
)

const (
	autoApproveFlag = "-auto-approve"
)

// ExecuteTerraform executes terraform commands
func ExecuteTerraform(cmd *cobra.Command, args []string) error {
	if len(args) < 3 {
		return errors.New("Invalid number of arguments")
	}

	cmd.DisableFlagParsing = false

	err := cmd.ParseFlags(args)
	if err != nil {
		return err
	}
	flags := cmd.Flags()

	// Get stack
	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	// Process and merge CLI configurations
	err = c.InitConfig(stack)
	if err != nil {
		return err
	}

	// Process CLI arguments and flags
	additionalArgsAndFlags := removeCommonArgsAndFlags(args)
	subCommand := args[0]

	// Process stack config file(s)
	_, stacksMap, err := s.ProcessYAMLConfigFiles(
		c.ProcessedConfig.StacksBaseAbsolutePath,
		c.ProcessedConfig.StackConfigFilesAbsolutePaths,
		false,
		true)

	if err != nil {
		return err
	}

	// Check if component was provided
	componentFromArg := args[1]
	if len(componentFromArg) < 1 {
		return errors.New("'component' is required")
	}

	// Print the stack config files
	fmt.Println()
	var msg string
	if c.ProcessedConfig.StackType == "Directory" {
		msg = "Found the config file for the provided stack:"
	} else {
		msg = "Found config files:"
	}
	color.Cyan(msg)
	err = u.PrintAsYAML(c.ProcessedConfig.StackConfigFilesRelativePaths)
	if err != nil {
		return err
	}

	// Check and process stacks
	var componentVarsSection map[interface{}]interface{}
	var baseComponent string
	var command string

	if c.ProcessedConfig.StackType == "Directory" {
		componentVarsSection, baseComponent, command, err = checkStackConfig(stack, stacksMap, "terraform", componentFromArg)
		if err != nil {
			return err
		}
	} else {
		color.Cyan("Searching for stack config where the component '%s' is defined\n", componentFromArg)

		if len(c.Config.Stacks.NamePattern) < 1 {
			return errors.New("Stack name pattern must be provided in 'stacks.name_pattern' config or 'ATMOS_STACKS_NAME_PATTERN' ENV variable")
		}

		stackParts := strings.Split(stack, "-")
		stackNamePatternParts := strings.Split(c.Config.Stacks.NamePattern, "-")

		var tenant string
		var environment string
		var stage string
		var tenantFound bool
		var environmentFound bool
		var stageFound bool

		for i, part := range stackNamePatternParts {
			if part == "{tenant}" {
				tenant = stackParts[i]
			} else if part == "{environment}" {
				environment = stackParts[i]
			} else if part == "{stage}" {
				stage = stackParts[i]
			}
		}

		for stackName := range stacksMap {
			componentVarsSection, baseComponent, command, err = checkStackConfig(stackName, stacksMap, "terraform", componentFromArg)
			if err != nil {
				continue
			}

			tenantFound = true
			environmentFound = true
			stageFound = true

			// Search for tenant in stack
			if len(tenant) > 0 {
				if tenantInStack, ok := componentVarsSection["tenant"].(string); !ok || tenantInStack != tenant {
					tenantFound = false
				}
			}

			// Search for environment in stack
			if len(environment) > 0 {
				if environmentInStack, ok := componentVarsSection["environment"].(string); !ok || environmentInStack != environment {
					environmentFound = false
				}
			}

			// Search for stage in stack
			if len(stage) > 0 {
				if stageInStack, ok := componentVarsSection["stage"].(string); !ok || stageInStack != stage {
					stageFound = false
				}
			}

			if tenantFound == true && environmentFound == true && stageFound == true {
				color.Green("Found stack config for component '%s' in stack '%s'\n\n", componentFromArg, stackName)
				stack = stackName
				break
			}
		}

		if tenantFound == false || environmentFound == false || stageFound == false {
			return errors.New(fmt.Sprintf("\nCould not find config for component '%s' for stack '%s'.\n"+
				"Check that all attributes in the stack name pattern '%s' are defined in stack config files.\n"+
				"Did you forget an import?",
				componentFromArg,
				stack,
				c.Config.Stacks.NamePattern,
			))
		}
	}

	if len(command) > 0 {
		color.Cyan("Found 'command: %s' for component '%s' in stack '%s'\n\n", command, componentFromArg, stack)
	} else {
		command = "terraform"
	}

	color.Cyan("Variables for component '%s' in stack '%s':", componentFromArg, stack)
	err = u.PrintAsYAML(componentVarsSection)
	if err != nil {
		return err
	}

	// Check component (and its base component)
	component := componentFromArg
	if len(baseComponent) > 0 {
		component = baseComponent
	}

	componentPath := path.Join(c.ProcessedConfig.TerraformDirAbsolutePath, component)
	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return errors.New(fmt.Sprintf("Component '%s' does not exixt in %s", component, c.ProcessedConfig.TerraformDirAbsolutePath))
	}

	// Write variables to a file
	stackNameFormatted := strings.Replace(stack, "/", "-", -1)
	varFileName := fmt.Sprintf("%s/%s/%s-%s.terraform.tfvars.json", c.Config.Components.Terraform.BasePath, component, stackNameFormatted, componentFromArg)
	color.Cyan("Writing variables to file:")
	fmt.Println(varFileName)
	err = u.WriteToFileAsJSON(varFileName, componentVarsSection, 0644)
	if err != nil {
		return err
	}

	// Handle `terraform deploy` custom command
	if subCommand == "deploy" {
		subCommand = "apply"
		if !u.SliceContainsString(additionalArgsAndFlags, autoApproveFlag) {
			additionalArgsAndFlags = append(additionalArgsAndFlags, autoApproveFlag)
		}
	}

	// Print command info
	color.Cyan("\nCommand info:")
	color.Green("Terraform binary: " + command)
	color.Green("Terraform command: " + subCommand)
	color.Green("Arguments and flags: %v", additionalArgsAndFlags)
	color.Green("Component: " + componentFromArg)
	if len(baseComponent) > 0 {
		color.Green("Base component: " + baseComponent)
	}
	color.Green("Stack: " + stack)
	fmt.Println()

	// Execute command
	emoji, err := u.UnquoteCodePoint("\\U+1F680")
	if err != nil {
		return err
	}

	color.Cyan(fmt.Sprintf("\nExecuting command  %v", emoji))
	color.Green(fmt.Sprintf("Command: %s %s %s",
		command,
		subCommand,
		u.SliceOfStringsToSpaceSeparatedString(additionalArgsAndFlags),
	))

	workingDir := fmt.Sprintf("%s/%s", c.Config.Components.Terraform.BasePath, component)
	color.Green(fmt.Sprintf("Working dir: %s", workingDir))
	fmt.Println(strings.Repeat("\n", 2))

	planFile := fmt.Sprintf("%s-%s.planfile", stackNameFormatted, componentFromArg)
	varFile := fmt.Sprintf("%s-%s.terraform.tfvars.json", stackNameFormatted, componentFromArg)

	var workspaceName string
	if len(baseComponent) > 0 {
		workspaceName = fmt.Sprintf("%s-%s", stackNameFormatted, componentFromArg)
	} else {
		workspaceName = stackNameFormatted
	}

	// Run `terraform init`
	err = execCommand(command, []string{"init"}, componentPath)
	if err != nil {
		return err
	}

	// Run `terraform workspace`
	err = execCommand(command, []string{"workspace", "select", workspaceName}, componentPath)
	if err != nil {
		err = execCommand(command, []string{"workspace", "new", workspaceName}, componentPath)
		if err != nil {
			return err
		}
	}

	cleanUp := false
	allArgsAndFlags := append([]string{subCommand}, additionalArgsAndFlags...)

	switch subCommand {
	case "plan":
		allArgsAndFlags = append(allArgsAndFlags, []string{"-var-file", varFile, "-out", planFile}...)
		break
	case "destroy":
		allArgsAndFlags = append(allArgsAndFlags, []string{"-var-file", varFile}...)
		cleanUp = true
		break
	case "apply":
		// Use the planfile if `-auto-approve` flag is specified
		// Use the varfile if `-auto-approve` is specified
		if !u.SliceContainsString(allArgsAndFlags, autoApproveFlag) {
			allArgsAndFlags = append(allArgsAndFlags, []string{planFile}...)
		} else {
			allArgsAndFlags = append(allArgsAndFlags, []string{"-var-file", varFile}...)
		}
		cleanUp = true
		break
	}

	// Execute the command
	err = execCommand(command, allArgsAndFlags, componentPath)
	if err != nil {
		return err
	}

	if cleanUp == true {
		planFilePath := fmt.Sprintf("%s/%s/%s", c.ProcessedConfig.TerraformDirAbsolutePath, component, planFile)
		_ = os.Remove(planFilePath)

		varFilePath := fmt.Sprintf("%s/%s/%s", c.ProcessedConfig.TerraformDirAbsolutePath, component, varFile)
		err = os.Remove(varFilePath)
		if err != nil {
			color.Red("Error deleting terraform var file: %s\n", err)
		}
	}

	return nil
}
