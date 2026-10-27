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

	"github.com/charmbracelet/lipgloss"

	cfg "github.com/cloudposse/atmos/pkg/config"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/shell"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

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

	// Merge env vars, ensuring componentEnvList takes precedence.
	// Include global env from atmos.yaml (lowest priority after system env).
	mergedEnv := envpkg.MergeSystemEnvWithGlobal(componentEnvList, atmosConfig.Env)

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
	log.Debug("Starting process", logFieldCommand, shellCommand)

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

// ExecAuthShellCommand starts a new interactive shell with the provided authentication environment variables.
// It increments ATMOS_SHLVL for the session, sets ATMOS_IDENTITY plus the supplied auth env vars into the shell environment, prints enter/exit messages, and launches the resolved shell command; returns an error if no suitable shell is found or if the shell process fails.
//
// The sanitizedEnv parameter should be a complete, pre-sanitized environment from PrepareShellEnvironment.
// It is used directly without re-reading os.Environ(), ensuring problematic vars (e.g., IRSA credentials)
// that were removed during auth preparation are not reintroduced.
func ExecAuthShellCommand(
	atmosConfig *schema.AtmosConfiguration,
	identityName string,
	providerName string,
	sanitizedEnv []string,
	shellOverride string,
	shellArgs []string,
) error {
	defer perf.Track(atmosConfig, "exec.ExecAuthShellCommand")()

	atmosShellVal := shell.Level() + 1
	if err := shell.SetLevel(atmosShellVal); err != nil {
		return err
	}

	// Decrement the value after exiting the shell.
	defer shell.DecrementLevel()

	// Append shell-specific env vars to the sanitized environment.
	// The sanitizedEnv already includes os.Environ() (sanitized) + auth vars.
	// Use UpdateEnvVar to replace-or-append each key, because os.StartProcess
	// does not deduplicate — the first occurrence wins if duplicates exist.
	shellEnv := append([]string{}, sanitizedEnv...)
	shellEnv = envpkg.UpdateEnvVar(shellEnv, "ATMOS_IDENTITY", identityName)
	shellEnv = envpkg.UpdateEnvVar(shellEnv, shell.LevelEnvVar, strconv.Itoa(atmosShellVal))

	// Append global env from atmos.yaml.
	for k, v := range atmosConfig.Env {
		shellEnv = envpkg.UpdateEnvVar(shellEnv, k, v)
	}

	log.Debug("Starting a new interactive shell with authentication environment variables (type 'exit' to go back)",
		"identity", identityName)

	log.Debug("Setting the ENV vars in the shell")

	// Warn about masking limitations in interactive TTY sessions.
	maskingEnabled := cfg.GlobalViper().GetBool("mask")
	if maskingEnabled {
		log.Debug("Interactive TTY session - output masking is not available due to TTY limitations")
	}

	// Print user-facing message about entering the shell.
	printShellEnterMessage(identityName, providerName)

	// Use the sanitized environment directly (no re-reading os.Environ()).
	mergedEnv := shellEnv

	// Determine shell command and args.
	shellCommand, shellCommandArgs := shell.Determine(shellOverride, shellArgs)

	log.Debug("Starting process", logFieldCommand, shellCommand, "args", shellCommandArgs)

	// Execute the shell and wait for it to exit.
	err := shell.StartInteractive(shellCommand, shellCommandArgs, mergedEnv)

	// Print user-facing message about exiting the shell.
	printShellExitMessage(identityName, providerName)

	return err
}

// printShellEnterMessage prints a user-facing message when entering an Atmos-managed shell.
func printShellEnterMessage(identityName, providerName string) {
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.GetCurrentColorScheme().Success)).
		Bold(true)

	identityStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.GetCurrentColorScheme().Link))

	providerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.GetCurrentColorScheme().TextMuted))

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.GetCurrentColorScheme().TextMuted))

	// Build identity display with provider name in parentheses.
	identityDisplay := identityName
	if providerName != "" {
		identityDisplay = fmt.Sprintf("%s %s", identityName, providerStyle.Render(fmt.Sprintf("(%s)", providerName)))
	}

	ui.Writef("\n%s %s\n",
		headerStyle.Render("→ Entering Atmos shell with identity:"),
		identityStyle.Render(identityDisplay))

	ui.Writef("%s\n\n",
		hintStyle.Render("  Type 'exit' to return to your normal shell"))
}

// printShellExitMessage prints a user-facing message when exiting an Atmos-managed shell.
func printShellExitMessage(identityName, providerName string) {
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.GetCurrentColorScheme().TextMuted))

	identityStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.GetCurrentColorScheme().TextMuted))

	// Build identity display with provider name in parentheses.
	identityDisplay := identityName
	if providerName != "" {
		identityDisplay = fmt.Sprintf("%s (%s)", identityName, providerName)
	}

	ui.Writef("\n%s %s\n\n",
		headerStyle.Render("← Exited Atmos shell for identity:"),
		identityStyle.Render(identityDisplay))
}

const logFieldCommand = "command"
