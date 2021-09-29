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
	info, err := processConfigAndStacks("terraform", cmd, args)
	if err != nil {
		return err
	}

	if len(info.Stack) < 1 {
		return errors.New("the specified stack does not exist")
	}

	err = checkTerraformConfig()
	if err != nil {
		return err
	}

	var finalComponent string

	if len(info.BaseComponent) > 0 {
		finalComponent = info.BaseComponent
	} else {
		finalComponent = info.Component
	}

	// Check if the component exists
	componentPath := path.Join(c.ProcessedConfig.TerraformDirAbsolutePath, info.ComponentFolderPrefix, finalComponent)
	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return errors.New(fmt.Sprintf("Component '%s' does not exixt in %s",
			finalComponent,
			path.Join(c.ProcessedConfig.TerraformDirAbsolutePath, info.ComponentFolderPrefix),
		))
	}

	stackNameFormatted := strings.Replace(info.Stack, "/", "-", -1)

	// Write variables to a file
	var varFileName, varFileNameFromArg string

	// Handle `terraform varfile` and `terraform write varfile` custom commands
	if info.SubCommand == "varfile" || info.SubCommand == "write varfile" {
		if len(info.AdditionalArgsAndFlags) == 2 {
			fileFlag := info.AdditionalArgsAndFlags[0]
			if fileFlag == "-f" || fileFlag == "--file" {
				varFileNameFromArg = info.AdditionalArgsAndFlags[1]
			}
		}
	}

	if len(varFileNameFromArg) > 0 {
		varFileName = varFileNameFromArg
	} else {
		if len(info.ComponentFolderPrefix) == 0 {
			varFileName = fmt.Sprintf("%s/%s/%s-%s.terraform.tfvars.json",
				c.Config.Components.Terraform.BasePath,
				finalComponent,
				stackNameFormatted,
				info.Component,
			)
		} else {
			varFileName = fmt.Sprintf("%s/%s/%s/%s-%s.terraform.tfvars.json",
				c.Config.Components.Terraform.BasePath,
				info.ComponentFolderPrefix,
				finalComponent,
				stackNameFormatted,
				info.Component,
			)
		}
	}

	color.Cyan("Writing variables to file:")
	fmt.Println(varFileName)
	err = u.WriteToFileAsJSON(varFileName, info.ComponentVarsSection, 0644)
	if err != nil {
		return err
	}

	// Handle `terraform varfile` and `terraform write varfile` custom commands
	if info.SubCommand == "varfile" || info.SubCommand == "write varfile" {
		fmt.Println()
		return nil
	}

	// Handle `terraform deploy` custom command
	if info.SubCommand == "deploy" {
		info.SubCommand = "apply"
		if !u.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
			info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, autoApproveFlag)
		}
	}

	// Handle Config.Components.Terraform.ApplyAutoApprove flag
	if info.SubCommand == "apply" && c.Config.Components.Terraform.ApplyAutoApprove == true {
		if !u.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
			info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, autoApproveFlag)
		}
	}

	// Print command info
	color.Cyan("\nCommand info:")
	color.Green("Terraform binary: " + info.Command)
	color.Green("Terraform command: " + info.SubCommand)
	color.Green("Arguments and flags: %v", info.AdditionalArgsAndFlags)
	color.Green("Component: " + info.ComponentFromArg)
	if len(info.BaseComponentPath) > 0 {
		color.Green("Base component: " + info.BaseComponentPath)
	}
	color.Green("Stack: " + info.Stack)

	var workingDir string
	if len(info.ComponentNamePrefix) == 0 {
		workingDir = fmt.Sprintf("%s/%s", c.Config.Components.Terraform.BasePath, info.Component)
	} else {
		workingDir = fmt.Sprintf("%s/%s/%s", c.Config.Components.Terraform.BasePath, info.ComponentNamePrefix, info.Component)
	}
	color.Green(fmt.Sprintf("Working dir: %s", workingDir))
	fmt.Println()

	var planFile, varFile string
	if len(info.ComponentNamePrefix) == 0 {
		planFile = fmt.Sprintf("%s-%s.planfile", stackNameFormatted, info.Component)
		varFile = fmt.Sprintf("%s-%s.terraform.tfvars.json", stackNameFormatted, info.Component)
	} else {
		planFile = fmt.Sprintf("%s-%s-%s.planfile", stackNameFormatted, info.ComponentNamePrefix, info.Component)
		varFile = fmt.Sprintf("%s-%s-%s.terraform.tfvars.json", stackNameFormatted, info.ComponentNamePrefix, info.Component)
	}

	var workspaceName string
	if len(info.ComponentNamePrefix) == 0 {
		if len(info.BaseComponent) > 0 {
			workspaceName = fmt.Sprintf("%s-%s", stackNameFormatted, info.Component)
		} else {
			workspaceName = stackNameFormatted
		}
	} else {
		if len(info.BaseComponent) > 0 {
			workspaceName = fmt.Sprintf("%s-%s-%s", info.ComponentNamePrefix, stackNameFormatted, info.Component)
		} else {
			workspaceName = fmt.Sprintf("%s-%s", info.ComponentNamePrefix, stackNameFormatted)
		}
	}

	cleanUp := false
	allArgsAndFlags := append([]string{info.SubCommand}, info.AdditionalArgsAndFlags...)

	switch info.SubCommand {
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
	if info.SubCommand != "init" {
		err = execCommand(info.Command, []string{"init"}, componentPath, nil)
		if err != nil {
			return err
		}
	}

	// Run `terraform workspace`
	err = execCommand(info.Command, []string{"workspace", "select", workspaceName}, componentPath, nil)
	if err != nil {
		err = execCommand(info.Command, []string{"workspace", "new", workspaceName}, componentPath, nil)
		if err != nil {
			return err
		}
	}

	if info.SubCommand != "workspace" {
		// Execute the command
		err = execCommand(info.Command, allArgsAndFlags, componentPath, nil)
		if err != nil {
			return err
		}
	}

	if cleanUp == true {
		planFilePath := fmt.Sprintf("%s/%s/%s", c.ProcessedConfig.TerraformDirAbsolutePath, info.Component, planFile)
		_ = os.Remove(planFilePath)

		varFilePath := fmt.Sprintf("%s/%s/%s", c.ProcessedConfig.TerraformDirAbsolutePath, info.Component, varFile)
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
