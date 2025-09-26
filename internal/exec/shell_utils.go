package exec

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"

	log "github.com/cloudposse/atmos/pkg/logger"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteShellCommand prints and executes the provided command with args and flags.
func ExecuteShellCommand(
	atmosConfig schema.AtmosConfiguration,
	command string,
	args []string,
	dir string,
	env []string,
	dryRun bool,
	redirectStdError string,
) error {
	newShellLevel, err := u.GetNextShellLevel()
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
			log.Warn(err.Error())
			return err
		}

		defer func(f *os.File) {
			err = f.Close()
			if err != nil {
				log.Warn(err.Error())
			}
		}(f)

		cmd.Stderr = f
	}
	log.Debug("Executing", "command", cmd.String())

	if dryRun {
		return nil
	}

	return cmd.Run()
}

// ExecuteShell runs a shell script
func ExecuteShell(
	command string,
	name string,
	dir string,
	env []string,
	dryRun bool,
) error {
	newShellLevel, err := u.GetNextShellLevel()
	if err != nil {
		return err
	}
	env = append(env, fmt.Sprintf("ATMOS_SHLVL=%d", newShellLevel))

	log.Debug("Executing", "command", command)

	if dryRun {
		return nil
	}

	return u.ShellRunner(command, name, dir, env, os.Stdout)
}

// execTerraformShellCommand executes `terraform shell` command by starting a new interactive shell
func execTerraformShellCommand(
	atmosConfig *schema.AtmosConfiguration,
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
			log.Warn("Failed to parse ATMOS_SHLVL", "error", err)
			return
		}
		// Prevent negative values
		newVal := val - 1
		if newVal < 0 {
			newVal = 0
		}
		if err := os.Setenv("ATMOS_SHLVL", fmt.Sprintf("%d", newVal)); err != nil {
			log.Warn("Failed to update ATMOS_SHLVL", "error", err)
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
	log.Debug("Starting a new interactive shell where you can execute all native Terraform commands (type 'exit' to go back)",
		"component", component,
		"stack", stack,
		"cwd", workingDir,
		"TerraformWorkspace", workspaceName)

	log.Debug("Setting the ENV vars in the shell")

	// Merge env vars, ensuring componentEnvList takes precedence
	mergedEnv := mergeEnvVars(componentEnvList)

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
		// Test if the environment variable GEODESIC_SHELL is set to "true" (or set at all).
		if !hasCustomShellPrompt {
			shellCommand = shellCommand + " -l"
		}

		if shellName == "zsh" && hasCustomShellPrompt {
			shellCommand = shellCommand + " -d -f -i"
		}
	}
	log.Debug("Starting process", "command", shellCommand)

	args := strings.Fields(shellCommand)

	proc, err := os.StartProcess(args[0], args[1:], &pa)
	if err != nil {
		return err
	}

	// Wait until the user exits the shell
	state, err := proc.Wait()
	if err != nil {
		return err
	}
	log.Debug("Exited shell", "state", state.String())
	return nil
}

// mergeEnvVars adds a list of environment variables to the system environment variables.
//
// This is necessary because:
//  1. We need to preserve existing system environment variables (PATH, HOME, etc.)
//  2. Atmos-specific variables (TF_CLI_ARGS, ATMOS_* vars) must take precedence
//  3. For conflicts, such as TF_CLI_ARGS_*, we need special handling to ensure proper merging rather than simple overwriting
func mergeEnvVars(componentEnvList []string) []string {
	envMap := make(map[string]string)

	// Parse system environment variables
	for _, env := range os.Environ() {
		if parts := strings.SplitN(env, "=", 2); len(parts) == 2 {
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
				// For all other environment variables, just override any existing value
				envMap[parts[0]] = parts[1]
			}
		}
	}

	// Convert back to slice
	merged := make([]string, 0, len(envMap))
	for k, v := range envMap {
		log.Debug("Setting ENV var", "key", k, "value", v)
		merged = append(merged, k+"="+v)
	}
	return merged
}
