package shell

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const osWindows = "windows"

// Determine resolves which shell binary to launch and which arguments to pass.
//
// The shell is taken from (in order): the explicit override, the "shell" Viper
// key, then a platform default (cmd.exe on Windows; bash, falling back to sh,
// elsewhere). When no shell args are supplied, a login shell ("-l") is used by
// default on Unix.
func Determine(shellOverride string, shellArgs []string) (string, []string) {
	defer perf.Track(nil, "shell.Determine")()

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

// StartInteractive launches an interactive shell process with the supplied
// environment, transferring stdin/stdout/stderr to it, and waits for the user
// to exit. The shell's exit code is propagated as errUtils.ExitCodeError.
//
// Internally, os.StartProcess is used (rather than exec.Command) so the TTY is
// passed through directly for interactive sessions.
func StartInteractive(shellCommand string, shellArgs []string, env []string) error {
	defer perf.Track(nil, "shell.StartInteractive")()

	if shellCommand == "" {
		return errors.Join(errUtils.ErrNoSuitableShell, fmt.Errorf("bash and sh not found in PATH"))
	}

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
