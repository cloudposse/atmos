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

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteShellCommand prints and executes the provided command with args and flags
func ExecuteShellCommand(
	cliConfig schema.CliConfiguration,
	command string,
	args []string,
	dir string,
	env []string,
	dryRun bool,
	verbose bool,
	redirectStdError string,
) error {
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	if redirectStdError == "" {
		cmd.Stderr = os.Stderr
	} else {
		f, err := os.OpenFile(redirectStdError, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			return err
		}

		defer func(f *os.File) {
			err = f.Close()
			if err != nil {
				u.LogWarning(cliConfig, err.Error())
			}
		}(f)

		cmd.Stderr = f
	}

	if verbose {
		u.LogInfo(cliConfig, "\nExecuting command:")
		u.LogInfo(cliConfig, cmd.String())
	}

	if dryRun {
		return nil
	}

	return cmd.Run()
}

// ExecuteShell runs a shell script
func ExecuteShell(
	cliConfig schema.CliConfiguration,
	command string,
	name string,
	dir string,
	env []string,
	dryRun bool,
	verbose bool,
) error {
	if verbose {
		u.LogInfo(cliConfig, "\nExecuting command:")
		u.LogInfo(cliConfig, command)
	}

	if dryRun {
		return nil
	}

	return shellRunner(command, name, dir, env, os.Stdout)
}

// ExecuteShellAndReturnOutput runs a shell script and capture its standard output
func ExecuteShellAndReturnOutput(
	cliConfig schema.CliConfiguration,
	command string,
	name string,
	dir string,
	env []string,
	dryRun bool,
	verbose bool,
) (string, error) {
	var b bytes.Buffer

	if verbose {
		u.LogInfo(cliConfig, "\nExecuting command:")
		u.LogInfo(cliConfig, command)
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
func ExecuteShellCommandAndReturnOutput(
	cliConfig schema.CliConfiguration,
	command string,
	args []string,
	dir string,
	env []string,
	dryRun bool,
	verbose bool,
	redirectStdError string,
) (string, error) {
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin

	if redirectStdError == "" {
		cmd.Stderr = os.Stderr
	} else {
		f, err := os.OpenFile(redirectStdError, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			return "", err
		}

		defer func(f *os.File) {
			err = f.Close()
			if err != nil {
				u.LogWarning(cliConfig, err.Error())
			}
		}(f)

		cmd.Stderr = f
	}

	if verbose {
		u.LogInfo(cliConfig, "\nExecuting command:")
		u.LogInfo(cliConfig, cmd.String())
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
func ExecuteShellCommands(
	cliConfig schema.CliConfiguration,
	commands []string,
	dir string,
	env []string,
	dryRun bool,
	verbose bool,
	redirectStdError string,
) error {
	for _, command := range commands {
		args := strings.Fields(command)
		if len(args) > 0 {
			if err := ExecuteShellCommand(cliConfig, args[0], args[1:], dir, env, dryRun, verbose, redirectStdError); err != nil {
				return err
			}
		}
	}
	return nil
}

// execTerraformShellCommand executes `terraform shell` command by starting a new interactive shell
func execTerraformShellCommand(
	cliConfig schema.CliConfiguration,
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

	u.LogInfo(cliConfig, "\nStarting a new interactive shell where you can execute all native Terraform commands (type 'exit' to go back)")
	u.LogInfo(cliConfig, fmt.Sprintf("Component: %s\n", component))
	u.LogInfo(cliConfig, fmt.Sprintf("Stack: %s\n", stack))
	u.LogInfo(cliConfig, fmt.Sprintf("Working directory: %s\n", workingDir))
	u.LogInfo(cliConfig, fmt.Sprintf("Terraform workspace: %s\n", workspaceName))
	u.LogInfo(cliConfig, "\nSetting the ENV vars in the shell:\n")
	for _, v := range componentEnvList {
		u.LogInfo(cliConfig, v)
	}

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

	u.LogInfo(cliConfig, fmt.Sprintf("Starting process: %s\n", shellCommand))

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

	u.LogInfo(cliConfig, fmt.Sprintf("Exited shell: %s\n", state.String()))
	return nil
}
