package exec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// MaxShellDepth is the maximum number of nested shell commands that can be executed
const MaxShellDepth = 10

// getNextShellLevel increments the ATMOS_SHLVL and returns the new value or an error if maximum depth is exceeded
func getNextShellLevel() (int, error) {
	atmosShellLvl := os.Getenv("ATMOS_SHLVL")
	shellVal := 0
	if atmosShellLvl != "" {
		val, err := strconv.Atoi(atmosShellLvl)
		if err != nil {
			return 0, fmt.Errorf("invalid ATMOS_SHLVL value: %s", atmosShellLvl)
		}
		shellVal = val
	}

	shellVal++

	if shellVal > MaxShellDepth {
		return 0, fmt.Errorf("ATMOS_SHLVL (%d) exceeds maximum allowed depth (%d). Infinite recursion?",
			shellVal, MaxShellDepth)
	}
	return shellVal, nil
}

// ExecuteShellCommandWithPipe prints and executes the provided command with args and flags
func ExecuteShellCommandWithPipe(
	atmosConfig schema.AtmosConfiguration,
	command string,
	args []string,
	dir string,
	env []string,
	dryRun bool,
	redirectStdError string,
	stdin io.Reader,
	stdout io.Writer,
) (func() error, error) {
	newShellLevel, err := getNextShellLevel()
	if err != nil {
		return nil, err
	}
	updatedEnv := append(env, fmt.Sprintf("ATMOS_SHLVL=%d", newShellLevel))

	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), updatedEnv...)
	cmd.Dir = dir
	if stdin != nil {
		cmd.Stdin = stdin
	} else {
		cmd.Stdin = os.Stdin
	}
	if stdout != nil {
		cmd.Stdout = stdout
	} else {
		cmd.Stdout = os.Stdout
	}

	if runtime.GOOS == "windows" && redirectStdError == "/dev/null" {
		redirectStdError = "NUL"
	}

	if redirectStdError == "/dev/stderr" {
		cmd.Stderr = os.Stderr
	} else if redirectStdError == "/dev/stdout" {
		cmd.Stderr = os.Stdout
	} else if redirectStdError == "" {
		cmd.Stderr = os.Stderr
	} else {
		f, err := os.OpenFile(redirectStdError, os.O_WRONLY|os.O_CREATE, 0o644)
		if err != nil {
			u.LogWarning(atmosConfig, err.Error())
			return nil, err
		}

		defer func(f *os.File) {
			err = f.Close()
			if err != nil {
				u.LogWarning(atmosConfig, err.Error())
			}
		}(f)

		cmd.Stderr = f
	}

	u.LogDebug(atmosConfig, "\nExecuting command:")
	u.LogDebug(atmosConfig, cmd.String())

	if dryRun {
		return nil, nil
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Wait, nil
}

// ExecuteShellCommand prints and executes the provided command with args and flags
func ExecuteShellCommand(
	atmosConfig schema.AtmosConfiguration,
	command string,
	args []string,
	dir string,
	env []string,
	dryRun bool,
	redirectStdError string,
) error {
	newShellLevel, err := getNextShellLevel()
	if err != nil {
		return err
	}
	updatedEnv := append(env, fmt.Sprintf("ATMOS_SHLVL=%d", newShellLevel))

	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), updatedEnv...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	if runtime.GOOS == "windows" && redirectStdError == "/dev/null" {
		redirectStdError = "NUL"
	}

	if redirectStdError == "/dev/stderr" {
		cmd.Stderr = os.Stderr
	} else if redirectStdError == "/dev/stdout" {
		cmd.Stderr = os.Stdout
	} else if redirectStdError == "" {
		cmd.Stderr = os.Stderr
	} else {
		f, err := os.OpenFile(redirectStdError, os.O_WRONLY|os.O_CREATE, 0o644)
		if err != nil {
			u.LogWarning(err.Error())
			return err
		}

		defer func(f *os.File) {
			err = f.Close()
			if err != nil {
				u.LogWarning(err.Error())
			}
		}(f)

		cmd.Stderr = f
	}

	u.LogDebug("\nExecuting command:")
	u.LogDebug(cmd.String())

	if dryRun {
		return nil
	}

	return cmd.Run()
}

// ExecuteShell runs a shell script
func ExecuteShell(
	atmosConfig schema.AtmosConfiguration,
	command string,
	name string,
	dir string,
	env []string,
	dryRun bool,
) error {
	newShellLevel, err := getNextShellLevel()
	if err != nil {
		return err
	}
	updatedEnv := append(env, fmt.Sprintf("ATMOS_SHLVL=%d", newShellLevel))

	u.LogDebug("\nExecuting command:")
	u.LogDebug(command)

	if dryRun {
		return nil
	}

	return shellRunner(command, name, dir, updatedEnv, os.Stdout)
}

// ExecuteShellAndReturnOutput runs a shell script and capture its standard output
func ExecuteShellAndReturnOutput(
	atmosConfig schema.AtmosConfiguration,
	command string,
	name string,
	dir string,
	env []string,
	dryRun bool,
) (string, error) {
	var b bytes.Buffer

	newShellLevel, err := getNextShellLevel()
	if err != nil {
		return "", err
	}
	updatedEnv := append(env, fmt.Sprintf("ATMOS_SHLVL=%d", newShellLevel))

	u.LogDebug("\nExecuting command:")
	u.LogDebug(command)

	if dryRun {
		return "", nil
	}

	err = shellRunner(command, name, dir, updatedEnv, &b)
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

// execTerraformShellCommand executes `terraform shell` command by starting a new interactive shell
func execTerraformShellCommand(
	atmosConfig schema.AtmosConfiguration,
	component string,
	stack string,
	componentEnvList []string,
	varFile string,
	workingDir string,
	workspaceName string,
	componentPath string,
) error {
	atmosShellLvl := os.Getenv("ATMOS_SHLVL")
	atmosShellVal := 1
	if atmosShellLvl != "" {
		val, err := strconv.Atoi(atmosShellLvl)
		if err != nil {
			return err
		}
		atmosShellVal = val + 1
	}
	if err := os.Setenv("ATMOS_SHLVL", fmt.Sprintf("%d", atmosShellVal)); err != nil {
		return err
	}

	// decrement the value after exiting the shell
	defer func() {
		atmosShellLvl := os.Getenv("ATMOS_SHLVL")
		if atmosShellLvl == "" {
			return
		}
		val, err := strconv.Atoi(atmosShellLvl)
		if err != nil {
			u.LogWarning(fmt.Sprintf("Failed to parse ATMOS_SHLVL: %v", err))
			return
		}
		// Prevent negative values
		newVal := val - 1
		if newVal < 0 {
			newVal = 0
		}
		if err := os.Setenv("ATMOS_SHLVL", fmt.Sprintf("%d", newVal)); err != nil {
			u.LogWarning(fmt.Sprintf("Failed to update ATMOS_SHLVL: %v", err))
		}
	}()

	// Define the Terraform commands that may use var-file configuration
	tfCommands := []string{"plan", "apply", "refresh", "import", "destroy", "console"}
	for _, cmd := range tfCommands {
		componentEnvList = append(componentEnvList, fmt.Sprintf("TF_CLI_ARGS_%s=-var-file=%s", cmd, varFile))
	}

	// Set environment variables to indicate the details of the Atmos shell configuration
	componentEnvList = append(componentEnvList, fmt.Sprintf("ATMOS_STACK=%s", stack))
	componentEnvList = append(componentEnvList, fmt.Sprintf("ATMOS_COMPONENT=%s", component))
	componentEnvList = append(componentEnvList, fmt.Sprintf("ATMOS_SHELL_WORKING_DIR=%s", workingDir))
	componentEnvList = append(componentEnvList, fmt.Sprintf("ATMOS_TERRAFORM_WORKSPACE=%s", workspaceName))

	hasCustomShellPrompt := atmosConfig.Components.Terraform.Shell.Prompt != ""
	if hasCustomShellPrompt {
		// Template for the custom shell prompt
		tmpl := atmosConfig.Components.Terraform.Shell.Prompt

		// Data for the template
		data := struct {
			Component string
			Stack     string
		}{
			Component: component,
			Stack:     stack,
		}

		// Parse and execute the template
		var result bytes.Buffer
		t := template.Must(template.New("shellPrompt").Parse(tmpl))
		if err := t.Execute(&result, data); err == nil {
			componentEnvList = append(componentEnvList, fmt.Sprintf("PS1=%s", result.String()))
		}
	}

	u.LogDebug("\nStarting a new interactive shell where you can execute all native Terraform commands (type 'exit' to go back)")
	u.LogDebug(fmt.Sprintf("Component: %s\n", component))
	u.LogDebug(fmt.Sprintf("Stack: %s\n", stack))
	u.LogDebug(fmt.Sprintf("Working directory: %s\n", workingDir))
	u.LogDebug(fmt.Sprintf("Terraform workspace: %s\n", workspaceName))
	u.LogDebug("\nSetting the ENV vars in the shell:\n")

	// Merge env vars, ensuring componentEnvList takes precedence
	mergedEnv := mergeEnvVars(atmosConfig, componentEnvList)

	// Transfer stdin, stdout, and stderr to the new process and also set the target directory for the shell to start in
	pa := os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Dir:   componentPath,
		Env:   mergedEnv,
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
				// Try fallback to sh if bash is not available
				shPath, shErr := exec.LookPath("sh")
				if shErr != nil {
					return fmt.Errorf("no suitable shell found: %v", shErr)
				}
				shellCommand = shPath
			} else {
				shellCommand = bashPath
			}
		}

		shellName := filepath.Base(shellCommand)

		// This means you cannot have a custom shell prompt inside Geodesic (Geodesic requires "-l").
		// Perhaps we should have special detection for Geodesic?
		// We could test if env var GEODESIC_SHELL is set to "true" (or set at all).
		if !hasCustomShellPrompt {
			shellCommand = shellCommand + " -l"
		}

		if shellName == "zsh" && hasCustomShellPrompt {
			shellCommand = shellCommand + " -d -f -i"
		}
	}

	u.LogDebug(fmt.Sprintf("Starting process: %s\n", shellCommand))

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

	u.LogDebug(fmt.Sprintf("Exited shell: %s\n", state.String()))
	return nil
}

// mergeEnvVars adds a list of environment variables to the system environment variables
//
// This is necessary because:
//  1. We need to preserve existing system environment variables (PATH, HOME, etc.)
//  2. Atmos-specific variables (TF_CLI_ARGS, ATMOS_* vars) must take precedence
//  3. For conflicts, such as TF_CLI_ARGS_*, we need special handling to ensure proper merging rather than simple overwriting
func mergeEnvVars(atmosConfig schema.AtmosConfiguration, componentEnvList []string) []string {
	envMap := make(map[string]string)

	// Parse system environment variables
	for _, env := range os.Environ() {
		if parts := strings.SplitN(env, "=", 2); len(parts) == 2 {
			if strings.HasPrefix(parts[0], "TF_") {
				u.LogWarning(fmt.Sprintf("detected '%s' set in the environment; this may interfere with Atmos's control of Terraform.", parts[0]))
			}
			envMap[parts[0]] = parts[1]
		}
	}

	// Merge with new, Atmos defined environment variables
	for _, env := range componentEnvList {
		if parts := strings.SplitN(env, "=", 2); len(parts) == 2 {
			// Special handling for Terraform CLI arguments environment variables
			if strings.HasPrefix(parts[0], "TF_CLI_ARGS_") {
				// For TF_CLI_ARGS_* variables, we need to append new values to any existing values
				if existing, exists := envMap[parts[0]]; exists {
					// Put the new, Atmos defined value first so it takes precedence
					envMap[parts[0]] = parts[1] + " " + existing
				} else {
					// No existing value, just set the new value
					envMap[parts[0]] = parts[1]
				}
			} else {
				// For all other environment variables, simply override any existing value
				envMap[parts[0]] = parts[1]
			}
		}
	}

	// Convert back to slice
	merged := make([]string, 0, len(envMap))
	for k, v := range envMap {
		u.LogDebug(fmt.Sprintf("%s=%s", k, v))
		merged = append(merged, k+"="+v)
	}
	return merged
}
