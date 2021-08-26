package exec

import (
	c "atmos/internal/config"
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
	stack, componentFromArg, component, baseComponent, command, subCommand, componentVarsSection, additionalArgsAndFlags,
		err := processConfigAndStacks("terraform", cmd, args)

	// Check if the component exists
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
	err = execCommand(command, []string{"init"}, componentPath, nil)
	if err != nil {
		return err
	}

	// Run `terraform workspace`
	err = execCommand(command, []string{"workspace", "select", workspaceName}, componentPath, nil)
	if err != nil {
		err = execCommand(command, []string{"workspace", "new", workspaceName}, componentPath, nil)
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
		// Use the planfile if `-auto-approve` flag is not specified
		// Use the varfile if `-auto-approve` flag is specified
		if !u.SliceContainsString(allArgsAndFlags, autoApproveFlag) {
			allArgsAndFlags = append(allArgsAndFlags, []string{planFile}...)
		} else {
			allArgsAndFlags = append(allArgsAndFlags, []string{"-var-file", varFile}...)
		}
		cleanUp = true
		break
	}

	// Execute the command
	err = execCommand(command, allArgsAndFlags, componentPath, nil)
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
