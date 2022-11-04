package exec

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"path"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	autoApproveFlag = "-auto-approve"
)

// ExecuteTerraformCmd executes terraform commands
func ExecuteTerraformCmd(cmd *cobra.Command, args []string) error {
	cliConfig, err := cfg.InitCliConfig(cfg.ConfigAndStacksInfo{}, true)
	if err != nil {
		u.PrintErrorToStdError(err)
		return err
	}

	info, err := processCommandLineArgs(cliConfig, "terraform", cmd, args)
	if err != nil {
		return err
	}

	info, err = ProcessStacks(cliConfig, info, true)
	if err != nil {
		return err
	}

	if info.NeedHelp {
		return nil
	}

	if len(info.Stack) < 1 {
		return errors.New("stack must be specified")
	}

	err = checkTerraformConfig(cliConfig)
	if err != nil {
		return err
	}

	// Check if the component (or base component) exists as Terraform component
	componentPath := path.Join(cliConfig.TerraformDirAbsolutePath, info.ComponentFolderPrefix, info.FinalComponent)
	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return fmt.Errorf("'%s' points to the Terraform component '%s', but it does not exist in '%s'",
			info.ComponentFromArg,
			info.FinalComponent,
			path.Join(cliConfig.Components.Terraform.BasePath, info.ComponentFolderPrefix),
		)
	}

	// Check if the component is allowed to be provisioned (`metadata.type` attribute is not set to `abstract`)
	if (info.SubCommand == "plan" || info.SubCommand == "apply" || info.SubCommand == "deploy" || info.SubCommand == "workspace") && info.ComponentIsAbstract {
		return fmt.Errorf("abstract component '%s' cannot be provisioned since it's explicitly prohibited from being deployed "+
			"by 'metadata.type: abstract' attribute", path.Join(info.ComponentFolderPrefix, info.Component))
	}

	varFile := constructTerraformComponentVarfileName(info)
	planFile := constructTerraformComponentPlanfileName(info)

	if info.SubCommand == "clean" {
		fmt.Println("Deleting '.terraform' folder")
		err = os.RemoveAll(path.Join(componentPath, ".terraform"))
		if err != nil {
			u.PrintError(err)
		}

		fmt.Println("Deleting '.terraform.lock.hcl' file")
		_ = os.Remove(path.Join(componentPath, ".terraform.lock.hcl"))

		fmt.Printf("Deleting terraform varfile: %s\n", varFile)
		_ = os.Remove(path.Join(componentPath, varFile))

		fmt.Printf("Deleting terraform planfile: %s\n", planFile)
		_ = os.Remove(path.Join(componentPath, planFile))

		// If `auto_generate_backend_file` is `true` (we are auto-generating backend files), remove `backend.tf.json`
		if cliConfig.Components.Terraform.AutoGenerateBackendFile {
			fmt.Println("Deleting 'backend.tf.json' file")
			_ = os.Remove(path.Join(componentPath, "backend.tf.json"))
		}

		tfDataDir := os.Getenv("TF_DATA_DIR")
		if len(tfDataDir) > 0 && tfDataDir != "." && tfDataDir != "/" && tfDataDir != "./" {
			u.PrintInfo(fmt.Sprintf("Found ENV var TF_DATA_DIR=%s", tfDataDir))
			var userAnswer string
			fmt.Printf("Do you want to delete the folder '%s'? (only 'yes' will be accepted to approve)\n", tfDataDir)
			fmt.Print("Enter a value: ")
			count, err := fmt.Scanln(&userAnswer)
			if count > 0 && err != nil {
				return err
			}
			if userAnswer == "yes" {
				fmt.Printf("Deleting folder '%s'\n", tfDataDir)
				err = os.RemoveAll(path.Join(componentPath, tfDataDir))
				if err != nil {
					u.PrintError(err)
				}
			}
		}

		fmt.Println()
		return nil
	}

	// Print component variables and write to file
	// Don't process variables when executing `terraform workspace` commands
	if info.SubCommand != "workspace" {
		u.PrintInfo(fmt.Sprintf("\nVariables for the component '%s' in the stack '%s':\n", info.ComponentFromArg, info.Stack))
		err = u.PrintAsYAML(info.ComponentVarsSection)
		if err != nil {
			return err
		}

		// Write variables to a file
		var varFilePath, varFileNameFromArg string

		// Handle `terraform varfile` and `terraform write varfile` legacy commands
		if info.SubCommand == "varfile" || (info.SubCommand == "write" && info.SubCommand2 == "varfile") {
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
			varFilePath = constructTerraformComponentVarfilePath(cliConfig, info)
		}

		u.PrintInfo("Writing the variables to file:")
		fmt.Println(varFilePath)

		if !info.DryRun {
			err = u.WriteToFileAsJSON(varFilePath, info.ComponentVarsSection, 0644)
			if err != nil {
				return err
			}
		}
	}

	// Handle `terraform varfile` and `terraform write varfile` legacy commands
	if info.SubCommand == "varfile" || (info.SubCommand == "write" && info.SubCommand2 == "varfile") {
		fmt.Println()
		return nil
	}

	// Check if component 'settings.validation' section is specified and validate the component
	valid, err := ValidateComponent(cliConfig, info.ComponentFromArg, info.ComponentSection, "", "")
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("\nComponent '%s' did not pass the validation policies.\n", info.ComponentFromArg)
	}

	// Auto generate backend file
	if cliConfig.Components.Terraform.AutoGenerateBackendFile {
		backendFileName := path.Join(
			constructTerraformComponentWorkingDir(cliConfig, info),
			"backend.tf.json",
		)

		fmt.Println()
		u.PrintInfo("Writing the backend config to file:")
		fmt.Println(backendFileName)

		if !info.DryRun {
			var componentBackendConfig = generateComponentBackendConfig(info.ComponentBackendType, info.ComponentBackendSection)
			err = u.WriteToFileAsJSON(backendFileName, componentBackendConfig, 0644)
			if err != nil {
				return err
			}
		}
	}

	// Run `terraform init` before running other commands
	runTerraformInit := true
	if info.SubCommand == "init" ||
		info.SubCommand == "clean" ||
		(info.SubCommand == "deploy" && !cliConfig.Components.Terraform.DeployRunInit) {
		runTerraformInit = false
	}

	if info.SkipInit {
		fmt.Println()
		u.PrintInfo("Skipping over 'terraform init' due to '--skip-init' flag being passed")
		runTerraformInit = false
	}

	if runTerraformInit {
		initCommandWithArguments := []string{"init"}
		if info.SubCommand == "workspace" || cliConfig.Components.Terraform.InitRunReconfigure {
			initCommandWithArguments = []string{"init", "-reconfigure"}
		}
		err = ExecuteShellCommand(info.Command, initCommandWithArguments, componentPath, info.ComponentEnvList, info.DryRun, true)
		if err != nil {
			return err
		}
	}

	// Handle `terraform deploy` custom command
	if info.SubCommand == "deploy" {
		info.SubCommand = "apply"
		if !info.UseTerraformPlan && !u.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
			info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, autoApproveFlag)
		}
	}

	// Handle cliConfig.Components.Terraform.ApplyAutoApprove flag
	if info.SubCommand == "apply" && cliConfig.Components.Terraform.ApplyAutoApprove && !info.UseTerraformPlan {
		if !u.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
			info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, autoApproveFlag)
		}
	}

	// Print command info
	u.PrintInfo("\nCommand info:")
	fmt.Println("Terraform binary: " + info.Command)
	if info.SubCommand2 == "" {
		fmt.Printf("Terraform command: %s\n", info.SubCommand)
	} else {
		fmt.Printf("Terraform command: %s %s\n", info.SubCommand, info.SubCommand2)
	}
	fmt.Printf("Arguments and flags: %v\n", info.AdditionalArgsAndFlags)
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
		fmt.Println("Stack path: " + path.Join(cliConfig.BasePath, cliConfig.Stacks.BasePath, info.Stack))
	}

	workingDir := constructTerraformComponentWorkingDir(cliConfig, info)
	fmt.Printf("Working dir: %s\n", workingDir)

	// Print ENV vars if they are found in the component's stack config
	if len(info.ComponentEnvList) > 0 {
		fmt.Println()
		u.PrintInfo("Using ENV vars:")
		for _, v := range info.ComponentEnvList {
			fmt.Println(v)
		}
	}

	allArgsAndFlags := strings.Fields(info.SubCommand)

	switch info.SubCommand {
	case "plan":
		allArgsAndFlags = append(allArgsAndFlags, []string{"-var-file", varFile, "-out", planFile}...)
	case "destroy":
		allArgsAndFlags = append(allArgsAndFlags, []string{"-var-file", varFile}...)
	case "import":
		allArgsAndFlags = append(allArgsAndFlags, []string{"-var-file", varFile}...)
	case "refresh":
		allArgsAndFlags = append(allArgsAndFlags, []string{"-var-file", varFile}...)
	case "apply":
		if info.UseTerraformPlan {
			allArgsAndFlags = append(allArgsAndFlags, []string{planFile}...)
		} else {
			allArgsAndFlags = append(allArgsAndFlags, []string{"-var-file", varFile}...)
		}
	case "init":
		if cliConfig.Components.Terraform.InitRunReconfigure {
			allArgsAndFlags = append(allArgsAndFlags, []string{"-reconfigure"}...)
		}
	case "workspace":
		if info.SubCommand2 == "list" || info.SubCommand2 == "show" {
			allArgsAndFlags = append(allArgsAndFlags, []string{info.SubCommand2}...)
		} else if info.SubCommand2 != "" {
			allArgsAndFlags = append(allArgsAndFlags, []string{info.SubCommand2, info.TerraformWorkspace}...)
		}
	}

	allArgsAndFlags = append(allArgsAndFlags, info.AdditionalArgsAndFlags...)

	// Run `terraform workspace` before executing other terraform commands
	if info.SubCommand != "init" && !(info.SubCommand == "workspace" && info.SubCommand2 != "") {
		err = ExecuteShellCommand(info.Command, []string{"workspace", "select", info.TerraformWorkspace}, componentPath, info.ComponentEnvList, info.DryRun, true)
		if err != nil {
			err = ExecuteShellCommand(info.Command, []string{"workspace", "new", info.TerraformWorkspace}, componentPath, info.ComponentEnvList, info.DryRun, true)
			if err != nil {
				return err
			}
		}
	}

	// Check if the terraform command requires a user interaction,
	// but it's running in a scripted environment (where a `tty` is not attached or `stdin` is not attached)
	if os.Stdin == nil && !u.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
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

	// Execute the provided command (except for `terraform workspace` which was executed above)
	if !(info.SubCommand == "workspace" && info.SubCommand2 == "") {
		err = ExecuteShellCommand(info.Command, allArgsAndFlags, componentPath, info.ComponentEnvList, info.DryRun, true)
		if err != nil {
			return err
		}
	}

	// Clean up
	if info.SubCommand != "plan" {
		planFilePath := constructTerraformComponentPlanfilePath(cliConfig, info)
		_ = os.Remove(planFilePath)
	}
	if info.SubCommand == "apply" {
		varFilePath := constructTerraformComponentVarfilePath(cliConfig, info)
		_ = os.Remove(varFilePath)
	}

	return nil
}
