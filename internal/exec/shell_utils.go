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
	"sync"
	"text/template"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/viper"
	xterm "golang.org/x/term"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/diagnostics"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	ioLayer "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	process "github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/shell"
	terminalpkg "github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const logFieldCommand = "command"

// ShellCommandOption is a functional option for ExecuteShellCommand.
type ShellCommandOption func(*shellCommandConfig)

// shellCommandConfig holds optional configuration for shell command execution.
type shellCommandConfig struct {
	stdoutCapture  io.Writer
	stderrCapture  io.Writer
	stdoutOverride io.Writer
	streams        *process.Streams
	ctx            context.Context
	// processEnv replaces os.Environ() as the process environment.
	// When set, ExecuteShellCommand uses this instead of re-reading os.Environ().
	// This is used when auth has already sanitized the environment (e.g., removed IRSA vars).
	processEnv []string
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

// WithProcessStreams provides the subprocess standard streams.
func WithProcessStreams(streams process.Streams) ShellCommandOption {
	return func(c *shellCommandConfig) {
		c.streams = &streams
	}
}

// WithProcessContext provides the context used for subprocess execution.
func WithProcessContext(ctx context.Context) ShellCommandOption {
	return func(c *shellCommandConfig) {
		c.ctx = ctx
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

	// Build environment: process env + global env (atmos.yaml) + command-specific env.
	// When auth has sanitized the environment, cfg.processEnv is used instead of
	// os.Environ() to avoid reintroducing problematic vars (e.g., IRSA credentials).
	baseEnv := os.Environ()
	if cfg.processEnv != nil {
		baseEnv = cfg.processEnv
	}
	cmdEnv := envpkg.MergeGlobalEnv(baseEnv, atmosConfig.Env)
	cmdEnv = append(cmdEnv, env...)
	cmdEnv = append(cmdEnv, fmt.Sprintf("ATMOS_SHLVL=%d", newShellLevel))

	// Propagate TTY state to subprocess.
	// MaskWriter wraps stderr as a pipe, so the subprocess's TTY detection (e.g., for SSO
	// device auth) will see a pipe instead of a terminal even when the user is interactive.
	// When the parent has a real TTY and ATMOS_FORCE_TTY is not already set, inject it so
	// subprocess commands that depend on TTY detection behave correctly.
	if xterm.IsTerminal(int(os.Stderr.Fd())) && !envKeyIsSet(cmdEnv, "ATMOS_FORCE_TTY") {
		cmdEnv = append(cmdEnv, "ATMOS_FORCE_TTY=true")
	}

	streams := process.OSStreams()
	if cfg.streams != nil {
		streams = *cfg.streams
		if streams.Stdin == nil {
			streams.Stdin = os.Stdin
		}
		if streams.Stdout == nil {
			streams.Stdout = os.Stdout
		}
		if streams.Stderr == nil {
			streams.Stderr = os.Stderr
		}
	}

	diagConfig := diagnostics.FromSchema(atmosConfig.Diagnostics)
	diagID := diagnostics.NewID("process")
	diagStartedAt := time.Now()

	var pacedClosers []io.Closer

	// Set up stdout: masked output to terminal, optionally tee'd to a capture writer.
	// When stdoutOverride is set, use it instead of os.Stdout (e.g., redirect to stderr
	// for workspace select so it doesn't pollute data-producing commands like output).
	var stdoutTarget io.Writer = streams.Stdout
	if cfg.stdoutOverride != nil {
		stdoutTarget = cfg.stdoutOverride
	}
	stdoutTarget = maybePaceTerminalWriter(stdoutTarget, atmosConfig.Settings.Terminal.Speed, &pacedClosers)
	maskedStdout := ioLayer.MaskWriter(stdoutTarget)
	stdoutWriters := []io.Writer{maskedStdout}
	if diagnostics.OutputEnabled(diagConfig) {
		stdoutWriters = append(stdoutWriters, diagnostics.NewOutputWriter(diagConfig, diagID, "stdout"))
	}
	if cfg.stdoutCapture != nil {
		stdoutWriters = append(stdoutWriters, cfg.stdoutCapture)
	}
	stdout := io.MultiWriter(stdoutWriters...)

	if runtime.GOOS == "windows" && redirectStdError == "/dev/null" {
		redirectStdError = "NUL"
	}
	if redirectStdError == "/dev/stdout" {
		stdout = &synchronizedWriter{w: stdout}
	}

	var stderr io.Writer
	if redirectStdError == "/dev/stderr" {
		stderrTarget := maybePaceTerminalWriter(streams.Stderr, atmosConfig.Settings.Terminal.Speed, &pacedClosers)
		maskedStderr := ioLayer.MaskWriter(stderrTarget)
		stderrWriters := []io.Writer{maskedStderr}
		if diagnostics.OutputEnabled(diagConfig) {
			stderrWriters = append(stderrWriters, diagnostics.NewOutputWriter(diagConfig, diagID, "stderr"))
		}
		if cfg.stderrCapture != nil {
			stderrWriters = append(stderrWriters, cfg.stderrCapture)
		}
		stderr = io.MultiWriter(stderrWriters...)
	} else if redirectStdError == "/dev/stdout" {
		maskedStderr := ioLayer.MaskWriter(stdout)
		if cfg.stderrCapture != nil {
			stderr = io.MultiWriter(maskedStderr, cfg.stderrCapture)
		} else {
			stderr = maskedStderr
		}
	} else if redirectStdError == "" {
		stderrTarget := maybePaceTerminalWriter(streams.Stderr, atmosConfig.Settings.Terminal.Speed, &pacedClosers)
		maskedStderr := ioLayer.MaskWriter(stderrTarget)
		stderrWriters := []io.Writer{maskedStderr}
		if diagnostics.OutputEnabled(diagConfig) {
			stderrWriters = append(stderrWriters, diagnostics.NewOutputWriter(diagConfig, diagID, "stderr"))
		}
		if cfg.stderrCapture != nil {
			stderrWriters = append(stderrWriters, cfg.stderrCapture)
		}
		stderr = io.MultiWriter(stderrWriters...)
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

		stderr = ioLayer.MaskWriter(f)
	}
	log.Debug("Executing", "command", command, "args", args)

	emitDiagnosticsEvent(diagConfig, &diagnostics.Event{
		Type:           "process.start",
		ID:             diagID,
		Level:          diagnostics.LevelDebug,
		Command:        command,
		Args:           append([]string{}, args...),
		CWD:            resolveDiagnosticsCWD(dir),
		DryRun:         diagnostics.Bool(dryRun),
		TTY:            diagnostics.Bool(streamsHaveTTY(streams)),
		StdinTTY:       diagnostics.Bool(isTTYReader(streams.Stdin)),
		StdoutTTY:      diagnostics.Bool(terminalpkg.IsTTYWriter(stdoutTarget)),
		StderrTTY:      diagnostics.Bool(terminalpkg.IsTTYWriter(streams.Stderr)),
		RedirectStderr: redirectStdError,
	})

	if dryRun {
		closeErr := closePacedWriters(pacedClosers)
		emitDiagnosticsEvent(diagConfig, &diagnostics.Event{
			Type:       "process.end",
			ID:         diagID,
			Level:      diagnostics.LevelDebug,
			Started:    diagnostics.Bool(false),
			Success:    diagnostics.Bool(closeErr == nil),
			ExitCode:   diagnostics.Int(0),
			DurationMS: diagnostics.Int64(time.Since(diagStartedAt).Milliseconds()),
			Error:      errorString(closeErr),
		})
		return closeErr
	}

	ctx := cfg.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	result := process.DefaultRunner{}.Run(ctx, process.TaskSpec{
		Command: command,
		Args:    args,
		Dir:     dir,
		Env:     cmdEnv,
		Streams: process.Streams{
			Stdin:  streams.Stdin,
			Stdout: stdout,
			Stderr: stderr,
		},
	})
	emitProcessEndDiagnostics(diagConfig, diagID, diagStartedAt, &result)
	if closeErr := closePacedWriters(pacedClosers); closeErr != nil && result.Err == nil {
		return closeErr
	}
	if result.Err == nil {
		return nil
	}

	if result.Canceled {
		return result.Err
	}

	if result.ExitCode >= 0 {
		log.Debug("Command exited with non-zero code", "code", result.ExitCode)
		return errUtils.ExitCodeError{Code: result.ExitCode}
	}
	return result.Err
}

func maybePaceTerminalWriter(w io.Writer, speed float64, closers *[]io.Closer) io.Writer {
	if !terminalpkg.IsSpeedLimited(speed) || !terminalpkg.IsTTYWriter(w) {
		return w
	}

	pacedWriter := terminalpkg.NewPacingWriter(w, speed)
	*closers = append(*closers, pacedWriter)
	return pacedWriter
}

func closePacedWriters(closers []io.Closer) error {
	var closeErr error
	for _, closer := range closers {
		if err := closer.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

func emitProcessEndDiagnostics(config diagnostics.Config, id string, fallbackStart time.Time, result *process.Result) {
	if result == nil {
		return
	}
	finishedAt := result.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now()
	}

	startedAt := result.StartedAt
	if startedAt.IsZero() {
		startedAt = fallbackStart
	}

	event := diagnostics.Event{
		Type:       "process.end",
		Time:       finishedAt.UTC(),
		ID:         id,
		Level:      diagnostics.LevelDebug,
		Started:    diagnostics.Bool(result.Started),
		Success:    diagnostics.Bool(result.Err == nil),
		Canceled:   diagnostics.Bool(result.Canceled),
		ExitCode:   diagnostics.Int(result.ExitCode),
		DurationMS: diagnostics.Int64(finishedAt.Sub(startedAt).Milliseconds()),
		Signaled:   diagnostics.Bool(result.Signaled),
		Signal:     result.Signal,
		Error:      errorString(result.Err),
	}
	if result.Signaled {
		event.SignalNumber = diagnostics.Int(result.SignalNumber)
	}
	emitDiagnosticsEvent(config, &event)
}

func emitDiagnosticsEvent(config diagnostics.Config, event *diagnostics.Event) {
	if err := diagnostics.EmitWithConfig(config, event); err != nil {
		log.Debug("Failed to write diagnostics event", "error", err)
	}
}

func resolveDiagnosticsCWD(dir string) string {
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return ""
		}
		return cwd
	}
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.Clean(dir)
	}
	return filepath.Clean(filepath.Join(cwd, dir))
}

func streamsHaveTTY(streams process.Streams) bool {
	return isTTYReader(streams.Stdin) || terminalpkg.IsTTYWriter(streams.Stdout) || terminalpkg.IsTTYWriter(streams.Stderr)
}

func isTTYReader(r io.Reader) bool {
	file, ok := r.(*os.File)
	if !ok || file == nil {
		return false
	}
	return xterm.IsTerminal(int(file.Fd())) //nolint:gosec // term.IsTerminal requires int file descriptors.
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

type synchronizedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *synchronizedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
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
	maskingEnabled := viper.GetBool("mask")
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
