package exec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	u "github.com/cloudposse/atmos/pkg/utils"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// ExecuteShellCommand prints and executes the provided command with args and flags
func ExecuteShellCommand(
	command string,
	args []string,
	dir string,
	env []string,
	dryRun bool,
	verbose bool,
) error {
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if verbose {
		u.PrintInfo("\nExecuting command:")
		fmt.Println(cmd.String())
	}

	if dryRun {
		return nil
	}

	return cmd.Run()
}

// ExecuteShell runs a shell script
func ExecuteShell(command string, name string, dir string, env []string, dryRun bool, verbose bool) error {
	if verbose {
		u.PrintInfo("\nExecuting command:")
		fmt.Println(command)
	}

	if dryRun {
		return nil
	}

	return shellRunner(command, name, dir, env, os.Stdout)
}

// ExecuteShellAndReturnOutput runs a shell script and capture its standard output
func ExecuteShellAndReturnOutput(command string, name string, dir string, env []string, dryRun bool, verbose bool) (string, error) {
	var b bytes.Buffer

	if verbose {
		u.PrintInfo("\nExecuting command:")
		fmt.Println(command)
	}

	if dryRun {
		return "", nil
	}

	err := shellRunner(command, name, dir, env, &b)
	if err != nil {
		return "", err
	}

	return b.String(), nil
}

// shellRunner uses mvdan.cc/sh/v3's parser and interpreter to run a shell script and divert its stdout
func shellRunner(command string, name string, dir string, env []string, out io.Writer) error {
	parser, err := syntax.NewParser().Parse(strings.NewReader(command), name)
	if err != nil {
		return err
	}

	environ := append(os.Environ(), env...)
	listEnviron := expand.ListEnviron(environ...)
	runner, err := interp.New(
		interp.Dir(dir),
		interp.Env(listEnviron),
		interp.StdIO(os.Stdin, out, os.Stderr),
	)
	if err != nil {
		return err
	}

	return runner.Run(context.TODO(), parser)
}

// ExecuteShellCommandAndReturnOutput prints and executes the provided command with args and flags and returns the command output
func ExecuteShellCommandAndReturnOutput(command string, args []string, dir string, env []string, dryRun bool, verbose bool) (string, error) {
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	if verbose {
		u.PrintInfo("\nExecuting command:")
		fmt.Println(cmd.String())
	}

	if dryRun {
		return "", nil
	}

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(output), nil
}

// ExecuteShellCommands sequentially executes the provided list of commands
func ExecuteShellCommands(commands []string, dir string, env []string, dryRun bool, verbose bool) error {
	for _, command := range commands {
		args := strings.Fields(command)
		if len(args) > 0 {
			if err := ExecuteShellCommand(args[0], args[1:], dir, env, dryRun, verbose); err != nil {
				return err
			}
		}
	}
	return nil
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
	componentEnvList = append(componentEnvList, fmt.Sprintf("TF_CLI_ARGS_console=-var-file=%s", varFile))

	fmt.Println()
	u.PrintInfo("Starting a new interactive shell where you can execute all native Terraform commands (type 'exit' to go back)")
	fmt.Printf("Component: %s\n", component)
	fmt.Printf("Stack: %s\n", stack)
	fmt.Printf("Working directory: %s\n", workingDir)
	fmt.Printf("Terraform workspace: %s\n", workspaceName)
	fmt.Println()
	u.PrintInfo("Setting the ENV vars in the shell:\n")
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

	u.PrintInfo(fmt.Sprintf("Starting process: %s", shellCommand))
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
