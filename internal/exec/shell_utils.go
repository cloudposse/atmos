package exec

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/viper"
	xterm "golang.org/x/term"

	errUtils "github.com/cloudposse/atmos/errors"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	ioLayer "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	atmosShellLevelEnvVar = "ATMOS_SHLVL"
	logFieldCommand       = "command"
	osWindows             = "windows"
)

// ShellCommandOption is a functional option for ExecuteShellCommand.
type ShellCommandOption func(*shellCommandConfig)

// shellCommandConfig holds optional configuration for shell command execution.
type shellCommandConfig struct {
	stdoutCapture  io.Writer
	stderrCapture  io.Writer
	stdoutOverride io.Writer
	// processEnv replaces os.Environ() as the process environment.
	// When set, ExecuteShellCommand uses this instead of re-reading os.Environ().
	// This is used when auth has already sanitized the environment (e.g., removed IRSA vars).
	processEnv []string
	// sanitizeTerraformSetupEnv, when true, filters the base process environment
	// through sanitizeTerraformWorkspaceEnv before merging.  This strips env
	// vars (TF_CLI_ARGS, TF_CLI_ARGS_workspace) that would cause OpenTofu to
	// inject flags into atmos-internal terraform/tofu setup subprocesses
	// (`workspace select`, `workspace new`, the auto-`init` pre-step) that
	// those subcommands do not accept.
	// See docs/fixes/2026-04-27-tf-cli-args-breaks-workspace-select.md.
	sanitizeTerraformSetupEnv bool
}

// WithStdoutCapture returns a ShellCommandOption that tees stdout to the provided writer.
// The captured output includes secret masking (post-MaskWriter).
func WithStdoutCapture(w io.Writer) ShellCommandOption {
	return func(c *shellCommandConfig) {
		c.stdoutCapture = w
	}
}

// WithStderrCapture returns a ShellCommandOption that tees stderr to the provided writer.
// The captured output includes secret masking (post-MaskWriter).
func WithStderrCapture(w io.Writer) ShellCommandOption {
	return func(c *shellCommandConfig) {
		c.stderrCapture = w
	}
}

// WithStdoutOverride returns a ShellCommandOption that replaces the default stdout
// (os.Stdout) with a different writer. Used to redirect noisy commands (e.g.,
// workspace select) to stderr so they don't pollute data-producing commands like output.
func WithStdoutOverride(w io.Writer) ShellCommandOption {
	return func(c *shellCommandConfig) {
		c.stdoutOverride = w
	}
}

// WithEnvironment provides a pre-sanitized process environment for subprocess execution.
// When provided, ExecuteShellCommand uses this instead of re-reading os.Environ().
// Pass nil to fall back to the default os.Environ() behavior.
func WithEnvironment(env []string) ShellCommandOption {
	defer perf.Track(nil, "exec.WithEnvironment")()

	return func(c *shellCommandConfig) {
		c.processEnv = env
	}
}

// WithSanitizedTerraformSetupEnv returns a ShellCommandOption that strips env
// vars known to break atmos-internal terraform/tofu setup subprocesses.
// Specifically: `TF_CLI_ARGS` and `TF_CLI_ARGS_workspace`.
//
// Atmos invokes several terraform/tofu subcommands as setup steps that the
// user did not write:
//   - `tofu workspace select` / `tofu workspace new` (workspace setup before
//     plan/apply).
//   - `tofu init` as an auto-init pre-step before plan/apply (when
//     SubCommand ≠ "init" and --skip-init is not set).
//
// OpenTofu prepends `TF_CLI_ARGS` to the argv of every subcommand. Users
// commonly put plan/apply-only flags in `TF_CLI_ARGS` (e.g.
// `-lock-timeout=10m` for CI lock retries, `-parallelism=4` for performance,
// `-refresh=false` for plan workflows). When Atmos's setup subcommands
// inherit those flags, they crash with "flag provided but not defined: …"
// before the user's target subcommand can run.
//
// This option only affects atmos-invoked SETUP subprocesses; the user's
// primary subcommand (`plan`, `apply`, `init` when invoked as the target,
// `version`, …) continues to receive `TF_CLI_ARGS` unchanged so that
// intentional configuration still reaches its target command. Per-subcommand
// variants (`TF_CLI_ARGS_plan`, `TF_CLI_ARGS_apply`, `TF_CLI_ARGS_init`, …)
// are always preserved — OpenTofu only applies them to their named
// subcommand by design, so they cannot interfere with setup commands.
//
// See docs/fixes/2026-04-27-tf-cli-args-breaks-workspace-select.md.
func WithSanitizedTerraformSetupEnv() ShellCommandOption {
	defer perf.Track(nil, "exec.WithSanitizedTerraformSetupEnv")()

	return func(c *shellCommandConfig) {
		c.sanitizeTerraformSetupEnv = true
	}
}

// ExecuteShellCommand prints and executes the provided command with args and flags.
func ExecuteShellCommand(
	atmosConfig schema.AtmosConfiguration,
	command string,
	args []string,
	dir string,
	env []string,
	dryRun bool,
	redirectStdError string,
	opts ...ShellCommandOption,
) error {
	defer perf.Track(&atmosConfig, "exec.ExecuteShellCommand")()

	// Apply functional options.
	var cfg shellCommandConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	newShellLevel, err := u.GetNextShellLevel()
	if err != nil {
		return err
	}

	cmd := exec.Command(command, args...)
	// Build environment: process env + global env (atmos.yaml) + command-specific env.
	// When auth has sanitized the environment, cfg.processEnv is used instead of
	// os.Environ() to avoid reintroducing problematic vars (e.g., IRSA credentials).
	baseEnv := os.Environ()
	if cfg.processEnv != nil {
		baseEnv = cfg.processEnv
	}
	cmdEnv := envpkg.MergeGlobalEnv(baseEnv, atmosConfig.Env)
	cmdEnv = append(cmdEnv, env...)
	// When the caller requested terraform-setup-env sanitization (atmos-internal
	// `tofu workspace select` / `tofu workspace new` / auto-`tofu init` pre-step),
	// filter out env vars that would cause OpenTofu to inject flags those
	// subcommands do not accept.  Applied AFTER the merge so blocked vars cannot
	// be reintroduced by atmosConfig.Env (atmos.yaml top-level env:) or by the
	// per-command env slice (info.ComponentEnvList → stack env: + auth hooks).
	// Applied BEFORE the ATMOS_SHLVL append so we don't strip our own bookkeeping.
	if cfg.sanitizeTerraformSetupEnv {
		cmdEnv = sanitizeTerraformWorkspaceEnv(cmdEnv)
	}
	cmdEnv = append(cmdEnv, fmt.Sprintf("ATMOS_SHLVL=%d", newShellLevel))

	// Propagate TTY state to subprocess.
	// MaskWriter wraps stderr as a pipe, so the subprocess's TTY detection (e.g., for SSO
	// device auth) will see a pipe instead of a terminal even when the user is interactive.
	// When the parent has a real TTY and ATMOS_FORCE_TTY is not already set, inject it so
	// subprocess commands that depend on TTY detection behave correctly.
	if xterm.IsTerminal(int(os.Stderr.Fd())) && !envKeyIsSet(cmdEnv, "ATMOS_FORCE_TTY") {
		cmdEnv = append(cmdEnv, "ATMOS_FORCE_TTY=true")
	}

	cmd.Env = cmdEnv
	cmd.Dir = dir
	cmd.Stdin = os.Stdin

	// Set up stdout: masked output to terminal, optionally tee'd to a capture writer.
	// When stdoutOverride is set, use it instead of os.Stdout (e.g., redirect to stderr
	// for workspace select so it doesn't pollute data-producing commands like output).
	var stdoutTarget io.Writer = os.Stdout
	if cfg.stdoutOverride != nil {
		stdoutTarget = cfg.stdoutOverride
	}
	maskedStdout := ioLayer.MaskWriter(stdoutTarget)
	if cfg.stdoutCapture != nil {
		cmd.Stdout = io.MultiWriter(maskedStdout, cfg.stdoutCapture)
	} else {
		cmd.Stdout = maskedStdout
	}

	if runtime.GOOS == "windows" && redirectStdError == "/dev/null" {
		redirectStdError = "NUL"
	}

	if redirectStdError == "/dev/stderr" {
		maskedStderr := ioLayer.MaskWriter(os.Stderr)
		if cfg.stderrCapture != nil {
			cmd.Stderr = io.MultiWriter(maskedStderr, cfg.stderrCapture)
		} else {
			cmd.Stderr = maskedStderr
		}
	} else if redirectStdError == "/dev/stdout" {
		maskedStderr := ioLayer.MaskWriter(os.Stdout)
		if cfg.stderrCapture != nil {
			cmd.Stderr = io.MultiWriter(maskedStderr, cfg.stderrCapture)
		} else {
			cmd.Stderr = maskedStderr
		}
	} else if redirectStdError == "" {
		maskedStderr := ioLayer.MaskWriter(os.Stderr)
		if cfg.stderrCapture != nil {
			cmd.Stderr = io.MultiWriter(maskedStderr, cfg.stderrCapture)
		} else {
			cmd.Stderr = maskedStderr
		}
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

		cmd.Stderr = ioLayer.MaskWriter(f)
	}
	log.Debug("Executing", "command", cmd.String())

	if dryRun {
		return nil
	}

	err = cmd.Run()
	if err != nil {
		// Extract exit code from error to preserve it.
		// This is critical for commands like `terraform plan -detailed-exitcode`
		// which use exit code 2 to indicate changes detected.
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode := exitError.ExitCode()
			log.Debug("Command exited with non-zero code", "code", exitCode)

			// Return a typed error that preserves the exit code.
			// main.go will check for this type and exit with the correct code.
			return errUtils.ExitCodeError{Code: exitCode}
		}
		// If we can't extract exit code, return the original error.
		return err
	}
	return nil
}

// ExecuteShell runs a shell script.
func ExecuteShell(
	command string,
	name string,
	dir string,
	envVars []string,
	dryRun bool,
) error {
	defer perf.Track(nil, "exec.ExecuteShell")()

	newShellLevel, err := u.GetNextShellLevel()
	if err != nil {
		return err
	}

	// Always start with the current process environment to ensure PATH and other
	// system variables are available. Custom env vars passed in will override
	// any existing values with the same key via UpdateEnvVar semantics.
	// This matches the behavior before commit 9fd7d156a where the environment
	// was merged rather than replaced.
	mergedEnv := os.Environ()
	for _, envVar := range envVars {
		mergedEnv = envpkg.UpdateEnvVar(mergedEnv, parseEnvVarKey(envVar), parseEnvVarValue(envVar))
	}

	mergedEnv = append(mergedEnv, fmt.Sprintf("ATMOS_SHLVL=%d", newShellLevel))

	log.Debug("Executing", "command", command)

	if dryRun {
		return nil
	}

	return u.ShellRunner(command, name, dir, mergedEnv, ioLayer.MaskWriter(os.Stdout))
}

// parseEnvVarKey extracts the key from an environment variable string (KEY=value).
func parseEnvVarKey(envVar string) string {
	if idx := strings.IndexByte(envVar, '='); idx >= 0 {
		return envVar[:idx]
	}
	return envVar
}

// parseEnvVarValue extracts the value from an environment variable string (KEY=value).
func parseEnvVarValue(envVar string) string {
	if idx := strings.IndexByte(envVar, '='); idx >= 0 {
		return envVar[idx+1:]
	}
	return ""
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

	atmosShellVal := getAtmosShellLevel() + 1
	if err := setAtmosShellLevel(atmosShellVal); err != nil {
		return err
	}

	// Decrement the value after exiting the shell.
	defer decrementAtmosShellLevel()

	// Append shell-specific env vars to the sanitized environment.
	// The sanitizedEnv already includes os.Environ() (sanitized) + auth vars.
	// Use UpdateEnvVar to replace-or-append each key, because os.StartProcess
	// does not deduplicate — the first occurrence wins if duplicates exist.
	shellEnv := append([]string{}, sanitizedEnv...)
	shellEnv = envpkg.UpdateEnvVar(shellEnv, "ATMOS_IDENTITY", identityName)
	shellEnv = envpkg.UpdateEnvVar(shellEnv, atmosShellLevelEnvVar, strconv.Itoa(atmosShellVal))

	// Append global env from atmos.yaml.
	for k, v := range atmosConfig.Env {
		shellEnv = envpkg.UpdateEnvVar(shellEnv, k, v)
	}

	log.Debug("Starting a new interactive shell with authentication environment variables (type 'exit' to go back)",
		"identity", identityName)

	log.Debug("Setting the ENV vars in the shell")

	// Warn about masking limitations in interactive TTY sessions.
	maskingEnabled := viper.GetBool("mask")
	if maskingEnabled {
		log.Debug("Interactive TTY session - output masking is not available due to TTY limitations")
	}

	// Print user-facing message about entering the shell.
	printShellEnterMessage(identityName, providerName)

	// Use the sanitized environment directly (no re-reading os.Environ()).
	mergedEnv := shellEnv

	// Determine shell command and args.
	shellCommand, shellCommandArgs := determineShell(shellOverride, shellArgs)
	if shellCommand == "" {
		return errors.Join(errUtils.ErrNoSuitableShell, fmt.Errorf("bash and sh not found in PATH"))
	}

	log.Debug("Starting process", logFieldCommand, shellCommand, "args", shellCommandArgs)

	// Execute the shell and wait for it to exit.
	err := executeShellProcess(shellCommand, shellCommandArgs, mergedEnv)

	// Print user-facing message about exiting the shell.
	printShellExitMessage(identityName, providerName)

	return err
}

// executeShellProcess starts a shell process and waits for it to exit, propagating the exit code.
func executeShellProcess(shellCommand string, shellArgs []string, env []string) error {
	// Resolve shell command to absolute path if necessary.
	// os.StartProcess doesn't search PATH, so we need to resolve relative commands.
	resolvedCommand := shellCommand
	if !filepath.IsAbs(resolvedCommand) {
		lookup, err := exec.LookPath(resolvedCommand)
		if err != nil {
			return errors.Join(errUtils.ErrNoSuitableShell, fmt.Errorf("failed to resolve shell %q", resolvedCommand))
		}
		resolvedCommand = lookup
	}

	// Build full args array: [shellCommand, arg1, arg2, ...].
	fullArgs := append([]string{shellCommand}, shellArgs...)

	// Transfer stdin, stdout, and stderr to the new process.
	pa := os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Dir:   "",
		Env:   env,
	}

	proc, err := os.StartProcess(resolvedCommand, fullArgs, &pa)
	if err != nil {
		return err
	}

	// Wait until the user exits the shell.
	state, err := proc.Wait()
	if err != nil {
		return err
	}

	exitCode := state.ExitCode()
	log.Debug("Exited shell", "state", state.String(), "exitCode", exitCode)

	// Propagate the shell's exit code.
	if exitCode != 0 {
		return errUtils.ExitCodeError{Code: exitCode}
	}

	return nil
}

// getAtmosShellLevel retrieves the current ATMOS_SHLVL value.
func getAtmosShellLevel() int {
	atmosShellLvl := os.Getenv(atmosShellLevelEnvVar) //nolint:forbidigo // ATMOS_SHLVL is a runtime variable that changes during shell execution, not a config variable.
	if atmosShellLvl == "" {
		return 0
	}
	val, err := strconv.Atoi(atmosShellLvl)
	if err != nil {
		return 0
	}
	return val
}

// setAtmosShellLevel sets the ATMOS_SHLVL environment variable.
func setAtmosShellLevel(level int) error {
	return os.Setenv(atmosShellLevelEnvVar, fmt.Sprintf("%d", level))
}

// decrementAtmosShellLevel decrements the ATMOS_SHLVL environment variable.
func decrementAtmosShellLevel() {
	currentLevel := getAtmosShellLevel()
	if currentLevel <= 0 {
		return
	}
	newLevel := currentLevel - 1
	if err := setAtmosShellLevel(newLevel); err != nil {
		log.Warn("Failed to update ATMOS_SHLVL", "error", err)
	}
}

// determineShell determines which shell to use and what arguments to pass.
func determineShell(shellOverride string, shellArgs []string) (string, []string) {
	// Determine shell command from override, environment, or fallback.
	shellCommand := shellOverride
	if shellCommand == "" {
		shellCommand = viper.GetString("shell")
	}
	if shellCommand == "" {
		if runtime.GOOS == osWindows {
			shellCommand = "cmd.exe"
		} else {
			shellCommand = findAvailableShell()
		}
	}

	// If no custom shell args provided, use login shell by default (Unix only).
	shellCommandArgs := shellArgs
	if len(shellCommandArgs) == 0 && runtime.GOOS != osWindows {
		shellCommandArgs = []string{"-l"}
	}

	return shellCommand, shellCommandArgs
}

// findAvailableShell finds an available shell on the system.
func findAvailableShell() string {
	// Try bash first.
	if bashPath, err := exec.LookPath("bash"); err == nil {
		return bashPath
	}

	// Fallback to sh.
	if shPath, err := exec.LookPath("sh"); err == nil {
		return shPath
	}

	// If nothing found, return empty (will cause error later).
	return ""
}

// printShellEnterMessage prints a user-facing message when entering an Atmos-managed shell.
func printShellEnterMessage(identityName, providerName string) {
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorGreen)).
		Bold(true)

	identityStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorCyan))

	providerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorGray))

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorGray))

	// Build identity display with provider name in parentheses.
	identityDisplay := identityName
	if providerName != "" {
		identityDisplay = fmt.Sprintf("%s %s", identityName, providerStyle.Render(fmt.Sprintf("(%s)", providerName)))
	}

	fmt.Fprintf(ioLayer.MaskWriter(os.Stderr), "\n%s %s\n",
		headerStyle.Render("→ Entering Atmos shell with identity:"),
		identityStyle.Render(identityDisplay))

	fmt.Fprintf(ioLayer.MaskWriter(os.Stderr), "%s\n\n",
		hintStyle.Render("  Type 'exit' to return to your normal shell"))
}

// printShellExitMessage prints a user-facing message when exiting an Atmos-managed shell.
func printShellExitMessage(identityName, providerName string) {
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorGray))

	identityStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorGray))

	// Build identity display with provider name in parentheses.
	identityDisplay := identityName
	if providerName != "" {
		identityDisplay = fmt.Sprintf("%s (%s)", identityName, providerName)
	}

	fmt.Fprintf(ioLayer.MaskWriter(os.Stderr), "\n%s %s\n\n",
		headerStyle.Render("← Exited Atmos shell for identity:"),
		identityStyle.Render(identityDisplay))
}

// envKeyIsSet returns true if any entry in env starts with "KEY=".
func envKeyIsSet(env []string, key string) bool {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	return false
}

// envVarFromList returns the value of the last "KEY=value" entry in env, or ""
// if the key is not present.  The last entry wins, matching how exec.Cmd.Env
// and os.Environ() resolve duplicates.
func envVarFromList(env []string, key string) string {
	prefix := key + "="
	result := ""
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			result = e[len(prefix):]
		}
	}
	return result
}
