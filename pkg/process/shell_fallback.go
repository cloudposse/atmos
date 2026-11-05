package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"syscall"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

const (
	exitStatusCannotExecute = 126
	exitStatusShellSyntax   = 2
)

func shellScriptFallback(err error) bool {
	return runtime.GOOS != "windows" && errors.Is(err, syscall.ENOEXEC)
}

// runShellScriptFallback preserves mvdan's Unix ENOEXEC handling: executable text without a
// shebang runs in a fresh shell with only exported variables. Its external commands still use
// the grouped runner, so restoring script compatibility does not undo cancellation cleanup.
func runShellScriptFallback(ctx context.Context, handler *interp.HandlerContext, path string, args []string, runner Runner) error {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(handler.Stderr, err)
		return interp.ExitStatus(exitStatusCannotExecute)
	}
	line, _, _ := bytes.Cut(data, []byte("\n"))
	if bytes.IndexByte(line, 0) >= 0 {
		fmt.Fprintf(handler.Stderr, "%s: cannot execute binary file\n", args[0])
		return interp.ExitStatus(exitStatusCannotExecute)
	}
	file, err := syntax.NewParser().Parse(bytes.NewReader(data), args[0])
	if err != nil {
		fmt.Fprintln(handler.Stderr, err)
		return interp.ExitStatus(exitStatusShellSyntax)
	}
	shell, err := interp.New(
		interp.Dir(handler.Dir),
		interp.Env(expand.ListEnviron(exportedEnvironment(handler.Env)...)),
		interp.StdIO(handler.Stdin, handler.Stdout, handler.Stderr),
		interp.ExecHandlers(func(_ interp.ExecHandlerFunc) interp.ExecHandlerFunc { return groupedExecHandler(runner) }),
	)
	if err != nil {
		return err
	}
	shell.Params = args[1:]
	return shell.Run(ctx, file)
}
