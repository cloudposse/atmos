package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func init() {
	// Every shell script Atmos runs through pkg/utils (custom commands, workflows, !exec, hook
	// commands) starts its commands through the process runner, so they get process-group cleanup.
	u.SetShellExecMiddleware(groupedMiddleware)
}

// groupedMiddleware replaces the interpreter's default command handler with one that starts each
// command through the process runner.
func groupedMiddleware(_ interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return groupedExecHandler(NewDefaultRunner())
}

// exitStatusNotFound is the shell exit status for a command that could not be started.
const exitStatusNotFound = 127

// exitStatusMask keeps an exit code within the 8 bits of a shell exit status.
const exitStatusMask = 0xff

// exportedEnvCapacity is the initial capacity of the exported-variable list.
const exportedEnvCapacity = 64

// exitStatusSignalBase is added to a signal number for the shell exit status of a signalled child.
const exitStatusSignalBase = 128

// ShellInterpreterSpec describes a shell script run by the in-process mvdan/sh interpreter.
type ShellInterpreterSpec struct {
	// Command is the shell script.
	Command string
	// Name labels the script in parse errors.
	Name string
	// Dir is the working directory.
	Dir string
	// Env is the environment. When empty, os.Environ() is used.
	Env []string
	// Params are the script's positional parameters ($1, $2, "$@"). Git hook arguments reach
	// shell steps this way.
	Params []string
	// Stdin is the script's standard input. When nil, os.Stdin is used.
	Stdin io.Reader
	// Stdout and Stderr receive the script's output.
	Stdout io.Writer
	Stderr io.Writer
}

// RunShellInterpreter runs a shell script in-process with mvdan.cc/sh and starts every external
// command it runs through the process runner. Each command is therefore the leader of its own
// process group (unless it inherits a terminal), and cancelling ctx, for example when a step
// `timeout:` expires, sends SIGTERM to the whole group and SIGKILL to survivors after a grace
// period. The interpreter's default command handler signals only the direct child, which leaves
// the grandchildren of `sh -c 'worker & wait'` running. On Windows only the direct child is
// terminated, as everywhere else in this package.
//
// A non-zero exit becomes errUtils.ExitCodeError so callers can propagate the child's exit code.
func RunShellInterpreter(ctx context.Context, spec *ShellInterpreterSpec) error {
	defer perf.Track(nil, "process.RunShellInterpreter")()

	file, err := syntax.NewParser().Parse(strings.NewReader(spec.Command), spec.Name)
	if err != nil {
		return err
	}
	// Use the provided environment directly to preserve PATH modifications.
	environ := spec.Env
	if len(environ) == 0 {
		environ = os.Environ()
	}
	var stdin io.Reader = os.Stdin
	if spec.Stdin != nil {
		stdin = spec.Stdin
	}
	options := []interp.RunnerOption{
		interp.Dir(spec.Dir),
		interp.Env(expand.ListEnviron(environ...)),
		interp.StdIO(stdin, spec.Stdout, spec.Stderr),
		interp.ExecHandlers(groupedMiddleware),
	}
	if len(spec.Params) > 0 {
		// "--" keeps a parameter such as -e from being read as a shell option.
		options = append(options, interp.Params(append([]string{"--"}, spec.Params...)...))
	}
	runner, err := interp.New(options...)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := runner.Run(ctx, file); err != nil {
		// Preserve the exit code. main.go exits with the code of this typed error.
		var exitStatus interp.ExitStatus
		if errors.As(err, &exitStatus) {
			return errUtils.ExitCodeError{Code: int(exitStatus)}
		}
		return err
	}
	return nil
}

// groupedExecHandler returns the interpreter command handler that runs external commands through
// runner. It mirrors the interpreter's default handler (exit status 127 for a command that cannot
// be started, 128 plus the signal number for a signalled child, the context error for a cancelled
// run) so scripts observe the same statuses with and without the process group.
func groupedExecHandler(runner Runner) interp.ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		handler := interp.HandlerCtx(ctx)
		if _, err := interp.LookPathDir(handler.Dir, handler.Env, args[0]); err != nil {
			fmt.Fprintln(handler.Stderr, err)
			return interp.ExitStatus(exitStatusNotFound)
		}
		result := runner.Run(ctx, TaskSpec{
			Command: args[0],
			Args:    args[1:],
			Dir:     handler.Dir,
			Env:     exportedEnvironment(handler.Env),
			Streams: Streams{Stdin: handler.Stdin, Stdout: handler.Stdout, Stderr: handler.Stderr},
			// The step reports its own result. Resource usage of shell commands was never part of
			// the end-of-invocation summary.
			SkipMetrics: true,
		})
		return execHandlerError(ctx, &handler, &result)
	}
}

// execHandlerError converts a finished process result into the error the interpreter expects.
func execHandlerError(ctx context.Context, handler *interp.HandlerContext, result *Result) error {
	if result.Err == nil {
		return nil
	}
	if !result.Started {
		var execErr *exec.Error
		if errors.As(result.Err, &execErr) {
			fmt.Fprintf(handler.Stderr, "%v\n", execErr)
			return interp.ExitStatus(exitStatusNotFound)
		}
		return result.Err
	}
	if result.Canceled || ctx.Err() != nil {
		return ctx.Err()
	}
	if result.Signaled {
		return shellExitStatus(exitStatusSignalBase + result.SignalNumber)
	}
	if result.ExitCode > 0 {
		return shellExitStatus(result.ExitCode)
	}
	return result.Err
}

// shellExitStatus converts an exit code to the interpreter's 8-bit exit status.
func shellExitStatus(code int) error {
	return interp.ExitStatus(uint8(code & exitStatusMask))
}

// exportedEnvironment lists the interpreter's exported variables as KEY=value pairs, the way the
// interpreter's own command handler does.
func exportedEnvironment(env expand.Environ) []string {
	list := make([]string, 0, exportedEnvCapacity)
	for name, variable := range env.Each {
		if !variable.IsSet() {
			// A variable set globally but unset in the interpreter must not reach the child, so
			// blank any earlier entry for it.
			for i, entry := range list {
				if strings.HasPrefix(entry, name+"=") {
					list[i] = ""
				}
			}
		}
		if variable.Exported && variable.Kind == expand.String {
			list = append(list, name+"="+variable.String())
		}
	}
	return slices.DeleteFunc(list, func(entry string) bool { return entry == "" })
}
