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
	stack, componentFromArg, componentFolderPrefix, componentNamePrefix, component, baseComponentPath, baseComponent,
		command, subCommand, componentVarsSection, additionalArgsAndFlags, _,
		err := processConfigAndStacks("terraform", cmd, args)
	if err != nil {
		return err
	}

	if len(stack) < 1 {
		return errors.New("the specified stack does not exist")
	}

	err = checkTerraformConfig()
	if err != nil {
		return err
	}

	var finalComponent string

	if len(baseComponent) > 0 {
		finalComponent = baseComponent
	} else {
		finalComponent = component
	}

	// Check if the component exists
	componentPath := path.Join(c.ProcessedConfig.TerraformDirAbsolutePath, componentFolderPrefix, finalComponent)
	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return errors.New(fmt.Sprintf("Component '%s' does not exixt in %s",
			finalComponent,
			path.Join(c.ProcessedConfig.TerraformDirAbsolutePath, componentFolderPrefix),
		))
	}

	stackNameFormatted := strings.Replace(stack, "/", "-", -1)

	// Write variables to a file
	var varFileName, varFileNameFromArg string

	// Handle `terraform varfile` custom command
	if subCommand == "varfile" {
		if len(additionalArgsAndFlags) == 2 {
			fileFlag := additionalArgsAndFlags[0]
			if fileFlag == "-f" || fileFlag == "--file" {
				varFileNameFromArg = additionalArgsAndFlags[1]
			}
		}
	}

	if len(varFileNameFromArg) > 0 {
		varFileName = varFileNameFromArg
	} else {
		if len(componentFolderPrefix) == 0 {
			varFileName = fmt.Sprintf("%s/%s/%s-%s.terraform.tfvars.json",
				c.Config.Components.Terraform.BasePath,
				finalComponent,
				stackNameFormatted,
				component,
			)
		} else {
			varFileName = fmt.Sprintf("%s/%s/%s/%s-%s.terraform.tfvars.json",
				c.Config.Components.Terraform.BasePath,
				componentFolderPrefix,
				finalComponent,
				stackNameFormatted,
				component,
			)
		}
	}

	color.Cyan("Writing variables to file:")
	fmt.Println(varFileName)
	err = u.WriteToFileAsJSON(varFileName, componentVarsSection, 0644)
	if err != nil {
		return err
	}

	// Handle `terraform varfile` custom command
	if subCommand == "varfile" {
		fmt.Println()
		return nil
	}

	// Handle `terraform deploy` custom command
	if subCommand == "deploy" {
		subCommand = "apply"
		if !u.SliceContainsString(additionalArgsAndFlags, autoApproveFlag) {
			additionalArgsAndFlags = append(additionalArgsAndFlags, autoApproveFlag)
		}
	}

	// Handle Config.Components.Terraform.ApplyAutoApprove flag
	if subCommand == "apply" && c.Config.Components.Terraform.ApplyAutoApprove == true {
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
	if len(baseComponentPath) > 0 {
		color.Green("Base component: " + baseComponentPath)
	}
	color.Green("Stack: " + stack)

	var workingDir string
	if len(componentNamePrefix) == 0 {
		workingDir = fmt.Sprintf("%s/%s", c.Config.Components.Terraform.BasePath, component)
	} else {
		workingDir = fmt.Sprintf("%s/%s/%s", c.Config.Components.Terraform.BasePath, componentNamePrefix, component)
	}
	color.Green(fmt.Sprintf("Working dir: %s", workingDir))
	fmt.Println()

	var planFile, varFile string
	if len(componentNamePrefix) == 0 {
		planFile = fmt.Sprintf("%s-%s.planfile", stackNameFormatted, component)
		varFile = fmt.Sprintf("%s-%s.terraform.tfvars.json", stackNameFormatted, component)
	} else {
		planFile = fmt.Sprintf("%s-%s-%s.planfile", stackNameFormatted, componentNamePrefix, component)
		varFile = fmt.Sprintf("%s-%s-%s.terraform.tfvars.json", stackNameFormatted, componentNamePrefix, component)
	}

	var workspaceName string
	if len(componentNamePrefix) == 0 {
		if len(baseComponent) > 0 {
			workspaceName = fmt.Sprintf("%s-%s", stackNameFormatted, finalComponent)
		} else {
			workspaceName = stackNameFormatted
		}
	} else {
		if len(baseComponent) > 0 {
			workspaceName = fmt.Sprintf("%s-%s-%s", componentNamePrefix, stackNameFormatted, finalComponent)
		} else {
			workspaceName = fmt.Sprintf("%s-%s", componentNamePrefix, stackNameFormatted)
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

	// Run `terraform init`
	if subCommand != "init" {
		err = execCommand(command, []string{"init"}, componentPath, nil)
		if err != nil {
			return err
		}
	}

	// Run `terraform workspace`
	err = execCommand(command, []string{"workspace", "select", workspaceName}, componentPath, nil)
	if err != nil {
		err = execCommand(command, []string{"workspace", "new", workspaceName}, componentPath, nil)
		if err != nil {
			return err
		}
	}

	if subCommand != "workspace" {
		// Execute the command
		err = execCommand(command, allArgsAndFlags, componentPath, nil)
		if err != nil {
			return err
		}
	}

	if cleanUp == true {
		planFilePath := fmt.Sprintf("%s/%s/%s", c.ProcessedConfig.TerraformDirAbsolutePath, component, planFile)
		_ = os.Remove(planFilePath)

		varFilePath := fmt.Sprintf("%s/%s/%s", c.ProcessedConfig.TerraformDirAbsolutePath, component, varFile)
		err = os.Remove(varFilePath)
		if err != nil {
			color.Yellow("Error deleting terraform var file: %s\n", err)
		}
	}

	return nil
}

func checkTerraformConfig() error {
	if len(c.Config.Components.Terraform.BasePath) < 1 {
		return errors.New("Base path to terraform components must be provided in 'components.terraform.base_path' config or " +
			"'ATMOS_COMPONENTS_TERRAFORM_BASE_PATH' ENV variable")
	}

	return nil
}
