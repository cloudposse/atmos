package exec

import (
	"fmt"
	"github.com/fatih/color"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// execCommand prints and executes the provided command with args and flags
func execCommand(command string, args []string, dir string, env []string, dryRun bool) error {
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println()
	color.Cyan("Executing command:\n")
	fmt.Println(cmd.String())

	if dryRun {
		return nil
	}

	return cmd.Run()
}

// execTerraformShellCommand executes `terraform shell` command by starting a new interactive shell
func execTerraformShellCommand(
	component string,
	stack string,
	componentEnvList []string,
	varFile string,
	workingDir string,
	workspaceName string,
	componentPath string) error {

	componentEnvList = append(componentEnvList, fmt.Sprintf("TF_CLI_ARGS_plan=-var-file=%s", varFile))
	componentEnvList = append(componentEnvList, fmt.Sprintf("TF_CLI_ARGS_apply=-var-file=%s", varFile))
	componentEnvList = append(componentEnvList, fmt.Sprintf("TF_CLI_ARGS_refresh=-var-file=%s", varFile))
	componentEnvList = append(componentEnvList, fmt.Sprintf("TF_CLI_ARGS_import=-var-file=%s", varFile))
	componentEnvList = append(componentEnvList, fmt.Sprintf("TF_CLI_ARGS_destroy=-var-file=%s", varFile))

	fmt.Println()
	color.Cyan("Starting a new interactive shell where you can execute all native Terraform commands (type 'exit' to go back)")
	fmt.Println(fmt.Sprintf("Component: %s", component))
	fmt.Println(fmt.Sprintf("Stack: %s", stack))
	fmt.Println(fmt.Sprintf("Working directory: %s", workingDir))
	fmt.Println(fmt.Sprintf("Terraform workspace: %s", workspaceName))
	fmt.Println()
	color.Cyan("Setting the ENV vars in the shell:\n")
	for _, v := range componentEnvList {
		fmt.Println(v)
	}
	fmt.Println()

	// Transfer stdin, stdout, and stderr to the new process and also set the target directory for the shell to start in
	pa := os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Dir:   componentPath,
		Env:   append(os.Environ(), componentEnvList...),
	}

	// Start a new shell
	var shellCommand string

	if runtime.GOOS == "windows" {
		shellCommand = "cmd.exe"
	} else {
		// If 'SHELL' ENV var is not defined, use 'bash' shell
		shellCommand = os.Getenv("SHELL")
		if len(shellCommand) == 0 {
			bashPath, err := exec.LookPath("bash")
			if err != nil {
				return err
			}
			shellCommand = bashPath
		}
		shellCommand = shellCommand + " -l"
	}

	color.Cyan(fmt.Sprintf("Starting process: %s", shellCommand))
	fmt.Println()

	args := strings.Fields(shellCommand)

	proc, err := os.StartProcess(args[0], args[1:], &pa)
	if err != nil {
		return err
	}

	// Wait until user exits the shell
	state, err := proc.Wait()
	if err != nil {
		return err
	}

	fmt.Printf("Exited shell: %s\n", state.String())
	return nil
}
