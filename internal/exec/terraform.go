package exec

import (
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"path"
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
	componentPathExists, err := utils.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return errors.New(fmt.Sprintf("Component '%s' does not exixt in %s",
			finalComponent,
			path.Join(c.ProcessedConfig.TerraformDirAbsolutePath, info.ComponentFolderPrefix),
		))
	}

	varFile := fmt.Sprintf("%s-%s.terraform.tfvars.json", info.ContextPrefix, info.Component)
	planFile := fmt.Sprintf("%s-%s.planfile", info.ContextPrefix, info.Component)

	if info.SubCommand == "clean" {
		fmt.Println("Deleting '.terraform' folder")
		_ = os.RemoveAll(path.Join(componentPath, ".terraform"))

		fmt.Println("Deleting '.terraform.lock.hcl' file")
		_ = os.Remove(path.Join(componentPath, ".terraform.lock.hcl"))

		fmt.Println(fmt.Sprintf("Deleting terraform varfile: %s", varFile))
		_ = os.Remove(path.Join(componentPath, varFile))

		fmt.Println(fmt.Sprintf("Deleting terraform planfile: %s", planFile))
		_ = os.Remove(path.Join(componentPath, planFile))

		tfDataDir := os.Getenv("TF_DATA_DIR")
		if len(tfDataDir) > 0 && tfDataDir != "." && tfDataDir != "/" && tfDataDir != "./" {
			color.Cyan("Found ENV var TF_DATA_DIR=%s", tfDataDir)
			var userAnswer string
			fmt.Println(fmt.Sprintf("Do you want to delete the folder '%s'? (only 'yes' will be accepted to approve)", tfDataDir))
			fmt.Print("Enter a value: ")
			count, err := fmt.Scanln(&userAnswer)
			if count > 0 && err != nil {
				return err
			}
			if userAnswer == "yes" {
				fmt.Println(fmt.Sprintf("Deleting folder '%s'", tfDataDir))
				_ = os.RemoveAll(tfDataDir)
			}
		}

		fmt.Println()
		return nil
	}

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
			varFileName = path.Join(
				c.Config.Components.Terraform.BasePath,
				finalComponent,
				varFile,
			)
		} else {
			varFileName = path.Join(
				c.Config.Components.Terraform.BasePath,
				info.ComponentFolderPrefix,
				finalComponent,
				varFile,
			)
		}
	}

	color.Cyan("Writing variables to file:")
	fmt.Println(varFileName)
	err = utils.WriteToFileAsJSON(varFileName, info.ComponentVarsSection, 0644)
	if err != nil {
		return err
	}

	// Handle `terraform varfile` and `terraform write varfile` custom commands
	if info.SubCommand == "varfile" || info.SubCommand == "write varfile" {
		fmt.Println()
		return nil
	}

	// Auto generate backend file
	var backendFileName string
	if c.Config.Components.Terraform.AutoGenerateBackendFile == true {
		fmt.Println()
		if len(info.ComponentFolderPrefix) == 0 {
			backendFileName = fmt.Sprintf("%s/%s/backend.tf.json",
				c.Config.Components.Terraform.BasePath,
				finalComponent,
			)
		} else {
			backendFileName = fmt.Sprintf("%s/%s/%s/backend.tf.json",
				c.Config.Components.Terraform.BasePath,
				info.ComponentFolderPrefix,
				finalComponent,
			)
		}
		color.Cyan("Writing backend config to file:")
		fmt.Println(backendFileName)
		var componentBackendConfig = generateComponentBackendConfig(info.ComponentBackendType, info.ComponentBackendSection)
		err = utils.WriteToFileAsJSON(backendFileName, componentBackendConfig, 0644)
		if err != nil {
			return err
		}
	}

	// Run `terraform init`
	runTerraformInit := true
	if info.SubCommand == "init" ||
		info.SubCommand == "workspace" ||
		info.SubCommand == "clean" ||
		(info.SubCommand == "deploy" && c.Config.Components.Terraform.DeployRunInit == false) {
		runTerraformInit = false
	}
	if runTerraformInit == true {
		err = execCommand(info.Command, []string{"init"}, componentPath, nil)
		if err != nil {
			return err
		}
	}

	// Handle `terraform deploy` custom command
	if info.SubCommand == "deploy" {
		info.SubCommand = "apply"
		if info.UseTerraformPlan == false && !utils.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
			info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, autoApproveFlag)
		}
	}

	// Handle Config.Components.Terraform.ApplyAutoApprove flag
	if info.SubCommand == "apply" && c.Config.Components.Terraform.ApplyAutoApprove == true && info.UseTerraformPlan == false {
		if !utils.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
			info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, autoApproveFlag)
		}
	}

	// Print command info
	color.Cyan("\nCommand info:")
	fmt.Println("Terraform binary: " + info.Command)
	fmt.Println("Terraform command: " + info.SubCommand)
	fmt.Println(fmt.Sprintf("Arguments and flags: %v", info.AdditionalArgsAndFlags))
	fmt.Println("Component: " + info.ComponentFromArg)
	if len(info.BaseComponentPath) > 0 {
		fmt.Println("Base component: " + info.BaseComponentPath)
	}
	fmt.Println("Stack: " + info.Stack)

	var workingDir string
	if len(info.ComponentFolderPrefix) == 0 {
		workingDir = fmt.Sprintf("%s/%s", c.Config.Components.Terraform.BasePath, finalComponent)
	} else {
		workingDir = fmt.Sprintf("%s/%s/%s", c.Config.Components.Terraform.BasePath, info.ComponentFolderPrefix, finalComponent)
	}
	fmt.Println(fmt.Sprintf(fmt.Sprintf("Working dir: %s", workingDir)))

	var workspaceName string
	if len(info.BaseComponent) > 0 {
		workspaceName = fmt.Sprintf("%s-%s", info.ContextPrefix, info.Component)
	} else {
		workspaceName = info.ContextPrefix
	}

	allArgsAndFlags := []string{info.SubCommand}

	switch info.SubCommand {
	case "plan":
		allArgsAndFlags = append(allArgsAndFlags, []string{"-var-file", varFile, "-out", planFile}...)
		break
	case "destroy":
		allArgsAndFlags = append(allArgsAndFlags, []string{"-var-file", varFile}...)
		break
	case "import":
		allArgsAndFlags = append(allArgsAndFlags, []string{"-var-file", varFile}...)
		break
	case "apply":
		if info.UseTerraformPlan == true {
			allArgsAndFlags = append(allArgsAndFlags, []string{planFile}...)
		} else {
			allArgsAndFlags = append(allArgsAndFlags, []string{"-var-file", varFile}...)
		}
		break
	}

	allArgsAndFlags = append(allArgsAndFlags, info.AdditionalArgsAndFlags...)

	// Run `terraform workspace`
	err = execCommand(info.Command, []string{"workspace", "select", workspaceName}, componentPath, nil)
	if err != nil {
		err = execCommand(info.Command, []string{"workspace", "new", workspaceName}, componentPath, nil)
		if err != nil {
			return err
		}
	}

	// Check if the terraform command requires a user interaction,
	// but it's running in a scripted environment (where a `tty` is not attached or `stdin` is not attached)
	if os.Stdin == nil && !utils.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
		errorMessage := ""
		if info.SubCommand == "apply" {
			errorMessage = "'terraform apply' requires a user interaction, but it's running without `tty` or `stdin` attached." +
				"\nUse 'terraform apply -auto-approve' or 'terraform deploy' instead."
		} else if info.SubCommand == "destroy" {
			errorMessage = "'terraform destroy' requires a user interaction, but it's running without `tty` or `stdin` attached." +
				"\nUse 'terraform destroy -auto-approve' if you need to destroy resources without asking the user for confirmation."
		}
		if errorMessage != "" {
			return errors.New(errorMessage)
		}
	}

	// Execute the command
	if info.SubCommand != "workspace" {
		err = execCommand(info.Command, allArgsAndFlags, componentPath, nil)
		if err != nil {
			return err
		}
	}

	// Clean up
	if info.SubCommand != "plan" {
		planFilePath := fmt.Sprintf("%s/%s", workingDir, planFile)
		_ = os.Remove(planFilePath)
	}

	err = os.Remove(varFileName)
	if err != nil {
		color.Yellow("Error deleting terraform varfile: %s\n", err)
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
