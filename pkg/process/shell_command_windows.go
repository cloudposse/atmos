//go:build windows

package process

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/cloudposse/atmos/pkg/perf"
)

// NewShellCommand builds the system-shell invocation for a session command
// string on Windows.
//
// cmd.exe does not follow the C quoting rules Go applies when it converts an
// argv into a process command line (`\"` escapes), so passing the command as
// a regular argument mangles anything containing quotes. Build the command
// line verbatim with `/S /C "<command>"`: /S makes cmd strip exactly the
// outer quotes and run everything inside literally.
func NewShellCommand(ctx context.Context, command string) *exec.Cmd {
	defer perf.Track(nil, "process.NewShellCommand")()

	shell, _ := sessionShell()
	cmd := exec.CommandContext(ctx, shell)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: fmt.Sprintf(`"%s" /S /C "%s"`, shell, command),
	}
	return cmd
}

// applyWindowsCmdExeQuoting rewrites cmd to invoke a `cmd.exe /C <command>`
// spec (see pkg/workflow/control_executor.go's controlShellInvocationForOS,
// used for non-interactive `type: shell` steps nested in a `type:
// parallel`/`type: matrix` group) the same verbatim-command-line way
// NewShellCommand already does for session-attached commands, instead of
// os/exec's default per-argument Windows escaping. That default escaping
// re-quotes and backslash-escapes the whole command string because it
// contains spaces, corrupting any quote characters already inside it (e.g. a
// quoted executable path) before cmd.exe ever sees it.
func applyWindowsCmdExeQuoting(cmd *exec.Cmd, program string, args []string) {
	if len(args) != 2 || args[0] != "/C" {
		return
	}
	if !strings.EqualFold(filepath.Base(program), "cmd.exe") {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: fmt.Sprintf(`"%s" /S /C "%s"`, program, args[1]),
	}
}
