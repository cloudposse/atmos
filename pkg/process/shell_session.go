package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/signals"
	"github.com/cloudposse/atmos/pkg/terminal/pty"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ShellSessionSpec describes a shell step that attaches to the user's
// terminal (steps declared with `tty: true` and/or `interactive: true`).
type ShellSessionSpec struct {
	// Command is the raw shell command string, run via the system shell.
	Command string
	// Name is a logical name used for logging.
	Name string
	// Dir is the working directory.
	Dir string
	// Env is the fully merged environment. When empty, os.Environ() is used.
	Env []string
	// TTY allocates a pseudo-terminal when the platform supports it.
	TTY bool
	// Interactive forwards host stdin to the session and suspends the Atmos
	// SIGINT-exit handler so Ctrl-C is handled by the child process.
	Interactive bool
	// DryRun logs the command without executing it.
	DryRun bool
	// Masker applies secret masking to PTY output. When nil, it defaults to
	// iolib.GetContext().Masker() with EnableMasking from the `mask` setting,
	// so call sites don't need masking wiring.
	Masker iolib.Masker
	// EnableMasking enables secret masking of session output where supported.
	EnableMasking bool
}

// RunShellStep routes a shell-family step: steps that request a terminal
// (tty and/or interactive) run as terminal-attached sessions; plain steps
// run via the caller-provided fallback (each execution path keeps its own
// plain-step behavior: masked interpreter, command runner, output modes).
func RunShellStep(ctx context.Context, spec *ShellSessionSpec, plain func() error) error {
	defer perf.Track(nil, "process.RunShellStep")()

	if spec.TTY || spec.Interactive {
		return RunShellSession(ctx, spec)
	}
	return plain()
}

// RunShellSession executes a shell command attached to the user's terminal.
//
// When TTY is set and the platform supports it, the command runs under a
// pseudo-terminal: output masking is preserved, and (with Interactive) the
// host terminal switches to raw mode so Ctrl-C reaches the child instead of
// Atmos. Otherwise the command inherits the real stdin/stdout/stderr file
// descriptors directly; masking is unavailable in that mode.
//
// Non-zero exits are returned as errUtils.ExitCodeError.
func RunShellSession(ctx context.Context, spec *ShellSessionSpec) error {
	defer perf.Track(nil, "process.RunShellSession")()

	if ctx == nil {
		ctx = context.Background()
	}

	// Default the masking wiring so call sites don't need to know about it.
	if spec.Masker == nil {
		spec.Masker = iolib.GetContext().Masker()
		spec.EnableMasking = viper.GetBool("mask")
	}

	log.Debug("Executing shell session",
		"name", spec.Name, "command", spec.Command, "tty", spec.TTY, "interactive", spec.Interactive)

	if spec.DryRun {
		return nil
	}

	shellLevel, err := u.GetNextShellLevel()
	if err != nil {
		return err
	}

	if spec.Interactive {
		// The foreground child owns Ctrl-C for the duration of the session.
		release := signals.SuspendInterruptExit()
		defer release()
	}

	env := spec.Env
	if len(env) == 0 {
		env = os.Environ()
	}
	env = append(append([]string{}, env...), fmt.Sprintf("ATMOS_SHLVL=%d", shellLevel))

	cmd := newShellCommand(ctx, spec.Command)
	cmd.Dir = spec.Dir
	cmd.Env = env

	if spec.TTY && pty.IsSupported() {
		return sessionExitError(runSessionPTY(ctx, cmd, spec))
	}
	return sessionExitError(runSessionAttached(cmd, spec))
}

// sessionShell returns the system shell and its command flag used to
// interpret session command strings.
func sessionShell() (string, string) {
	if runtime.GOOS == "windows" {
		if comspec := os.Getenv("COMSPEC"); comspec != "" { //nolint:forbidigo // COMSPEC is a Windows system variable, not Atmos configuration.
			return comspec, "/C"
		}
		return "cmd.exe", "/C"
	}
	return "sh", "-c"
}

// runSessionPTY runs the command under a pseudo-terminal with optional
// output masking. Host stdin is only forwarded for interactive sessions.
func runSessionPTY(ctx context.Context, cmd *exec.Cmd, spec *ShellSessionSpec) error {
	return pty.ExecWithPTY(ctx, cmd, &pty.Options{
		Masker:              spec.Masker,
		EnableMasking:       spec.EnableMasking,
		DisableStdinForward: !spec.Interactive,
	})
}

// runSessionAttached runs the command with the real standard streams
// inherited directly (no PTY). Used on platforms without PTY support.
// Masking cannot be applied to inherited file descriptors.
func runSessionAttached(cmd *exec.Cmd, spec *ShellSessionSpec) error {
	if spec.EnableMasking {
		if spec.TTY {
			// A PTY was requested but is unavailable on this platform - the
			// loss of masking is unexpected, so warn visibly.
			ui.Warning("Output masking is not supported for tty steps on this platform; secrets will not be masked")
		} else {
			// Plain interactive sessions always attach the real streams;
			// the loss of masking is inherent, so don't warn on every step.
			log.Debug("Output masking does not apply to interactive sessions", "name", spec.Name)
		}
	}

	// Attach the real *os.File streams so the child's own TTY detection works.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if spec.Interactive {
		cmd.Stdin = os.Stdin
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrProcessStartFailed, err)
	}
	return cmd.Wait()
}

// sessionExitError normalizes session errors: non-zero exits become
// errUtils.ExitCodeError so callers propagate the child's exit code.
// Signal-killed children report shell-convention codes (128+signal).
func sessionExitError(err error) error {
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return errUtils.ExitCodeError{Code: exitStatusCode(exitErr)}
	}
	return err
}
