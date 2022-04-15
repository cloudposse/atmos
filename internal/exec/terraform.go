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
	"strings"
)

const (
	autoApproveFlag = "-auto-approve"
)

// ExecuteTerraform executes terraform commands
func ExecuteTerraform(cmd *cobra.Command, args []string) error {
	info, err := processArgsConfigAndStacks("terraform", cmd, args)
	if err != nil {
		return err
	}

	if info.NeedHelp == true {
		return nil
	}

	if len(info.Stack) < 1 {
		return errors.New("stack must be specified")
	}

	err = checkTerraformConfig()
	if err != nil {
		return err
	}

	// Check if the component (or base component) exists as Terraform component
	componentPath := path.Join(c.ProcessedConfig.TerraformDirAbsolutePath, info.ComponentFolderPrefix, info.FinalComponent)
	componentPathExists, err := utils.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return errors.New(fmt.Sprintf("Component '%s' does not exist in '%s'",
			info.FinalComponent,
			path.Join(c.ProcessedConfig.TerraformDirAbsolutePath, info.ComponentFolderPrefix),
		))
	}

	// Check if the component is allowed to be provisioned (`metadata.type` attribute)
	if (info.SubCommand == "plan" || info.SubCommand == "apply" || info.SubCommand == "deploy" || info.SubCommand == "workspace") && info.ComponentIsAbstract {
		return errors.New(fmt.Sprintf("Abstract component '%s' cannot be provisioned since it's explicitly prohibited from being deployed "+
			"by 'metadata.type: abstract' attribute", path.Join(info.ComponentFolderPrefix, info.Component)))
	}

	varFile := constructTerraformComponentVarfileName(info)
	planFile := constructTerraformComponentPlanfileName(info)

	if info.SubCommand == "clean" {
		fmt.Println("Deleting '.terraform' folder")
		_ = os.RemoveAll(path.Join(componentPath, ".terraform"))

		fmt.Println("Deleting '.terraform.lock.hcl' file")
		_ = os.Remove(path.Join(componentPath, ".terraform.lock.hcl"))

		fmt.Println(fmt.Sprintf("Deleting terraform varfile: %s", varFile))
		_ = os.Remove(path.Join(componentPath, varFile))

		fmt.Println(fmt.Sprintf("Deleting terraform planfile: %s", planFile))
		_ = os.Remove(path.Join(componentPath, planFile))

		// If `auto_generate_backend_file` is `true` (we are auto-generating backend files), remove `backend.tf.json`
		if c.Config.Components.Terraform.AutoGenerateBackendFile {
			fmt.Println("Deleting 'backend.tf.json' file")
			_ = os.Remove(path.Join(componentPath, "backend.tf.json"))
		}

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

	// Print component variables
	color.Cyan("\nVariables for the component '%s' in the stack '%s':\n\n", info.ComponentFromArg, info.Stack)
	err = utils.PrintAsYAML(info.ComponentVarsSection)
	if err != nil {
		return err
	}

	// Write variables to a file
	var varFilePath, varFileNameFromArg string

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
		varFilePath = varFileNameFromArg
	} else {
		varFilePath = constructTerraformComponentVarfilePath(info)
	}

	color.Cyan("Writing the variables to file:")
	fmt.Println(varFilePath)

	if !info.DryRun {
		err = utils.WriteToFileAsJSON(varFilePath, info.ComponentVarsSection, 0644)
		if err != nil {
			return err
		}
	}

	// Handle `terraform varfile` and `terraform write varfile` custom commands
	if info.SubCommand == "varfile" || info.SubCommand == "write varfile" {
		fmt.Println()
		return nil
	}

	// Auto generate backend file
	if c.Config.Components.Terraform.AutoGenerateBackendFile == true {
		backendFileName := path.Join(
			constructTerraformComponentWorkingDir(info),
			"backend.tf.json",
		)

		fmt.Println()
		color.Cyan("Writing the backend config to file:")
		fmt.Println(backendFileName)

		if !info.DryRun {
			var componentBackendConfig = generateComponentBackendConfig(info.ComponentBackendType, info.ComponentBackendSection)
			err = utils.WriteToFileAsJSON(backendFileName, componentBackendConfig, 0644)
			if err != nil {
				return err
			}
		}
	}

	// Run `terraform init`
	runTerraformInit := true
	if info.SubCommand == "init" ||
		info.SubCommand == "clean" ||
		(info.SubCommand == "deploy" && c.Config.Components.Terraform.DeployRunInit == false) {
		runTerraformInit = false
	}
	if runTerraformInit == true {
		initCommandWithArguments := []string{"init"}
		if info.SubCommand == "workspace" || c.Config.Components.Terraform.InitRunReconfigure == true {
			initCommandWithArguments = []string{"init", "-reconfigure"}
		}
		err = execCommand(info.Command, initCommandWithArguments, componentPath, info.ComponentEnvList, info.DryRun)
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
		fmt.Println("Terraform component: " + info.BaseComponentPath)
	}
	if len(info.ComponentInheritanceChain) > 0 {
		fmt.Println("Inheritance: " + info.ComponentFromArg + " -> " + strings.Join(info.ComponentInheritanceChain, " -> "))
	}

	if info.Stack == info.StackFromArg {
		fmt.Println("Stack: " + info.StackFromArg)
	} else {
		fmt.Println("Stack: " + info.StackFromArg)
		fmt.Println("Stack path: " + path.Join(c.Config.BasePath, c.Config.Stacks.BasePath, info.Stack))
	}

	workingDir := constructTerraformComponentWorkingDir(info)
	fmt.Println(fmt.Sprintf(fmt.Sprintf("Working dir: %s", workingDir)))

	// Print ENV vars if they are found in the component's stack config
	if len(info.ComponentEnvList) > 0 {
		fmt.Println()
		color.Cyan("Using ENV vars:\n")
		for _, v := range info.ComponentEnvList {
			fmt.Println(v)
		}
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
	case "refresh":
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
	if info.SubCommand != "init" {
		err = execCommand(info.Command, []string{"workspace", "select", info.TerraformWorkspace}, componentPath, info.ComponentEnvList, info.DryRun)
		if err != nil {
			err = execCommand(info.Command, []string{"workspace", "new", info.TerraformWorkspace}, componentPath, info.ComponentEnvList, info.DryRun)
			if err != nil {
				return err
			}
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

	// Check `region` for `terraform import`
	if info.SubCommand == "import" {
		if region, regionExist := info.ComponentVarsSection["region"].(string); regionExist {
			info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("AWS_REGION=%s", region))
		}
	}

	// Execute `terraform shell` command
	if info.SubCommand == "shell" {
		err = execTerraformShellCommand(
			info.ComponentFromArg,
			info.Stack,
			info.ComponentEnvList,
			varFile,
			workingDir,
			info.TerraformWorkspace,
			componentPath,
		)
		if err != nil {
			return err
		}
		return nil
	}

	// Execute the provided command
	if info.SubCommand != "workspace" {
		err = execCommand(info.Command, allArgsAndFlags, componentPath, info.ComponentEnvList, info.DryRun)
		if err != nil {
			return err
		}
	}

	// Clean up
	if info.SubCommand != "plan" {
		planFilePath := constructTerraformComponentPlanfilePath(info)
		_ = os.Remove(planFilePath)
	}

	return nil
}
