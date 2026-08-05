//go:build windows

package process

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
)

// newShellCommand builds the system-shell invocation for a session command
// string on Windows.
//
// cmd.exe does not follow the C quoting rules Go applies when it converts an
// argv into a process command line (`\"` escapes), so passing the command as
// a regular argument mangles anything containing quotes. Build the command
// line verbatim with `/S /C "<command>"`: /S makes cmd strip exactly the
// outer quotes and run everything inside literally.
func newShellCommand(ctx context.Context, command string) *exec.Cmd {
	shell, _ := sessionShell()
	cmd := exec.CommandContext(ctx, shell)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: fmt.Sprintf(`"%s" /S /C "%s"`, shell, command),
	}
	return cmd
}
