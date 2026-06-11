package cmd

import (
	"context"

	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/signals"
)

// runShellSessionFn is a seam for tests to intercept terminal-attached steps.
var runShellSessionFn = process.RunShellSession

// replaceShellSessionFn is a seam for tests to intercept exec steps.
var replaceShellSessionFn = process.ReplaceShellSession

// executeShellStep runs a custom command shell step.
//
// Steps with `tty: true` attach to the user's terminal (PTY when supported).
// Steps with `interactive: true` let the child process own Ctrl-C: the Atmos
// SIGINT-exit handler is suspended while the step runs, so the terminal's
// SIGINT interrupts the step (which already has stdin attached), not Atmos.
// Plain steps keep the existing masked shell-interpreter behavior.
func executeShellStep(step *schema.Task, commandToRun, commandName, workDir string, env []string) error {
	if step.Tty {
		return runShellSessionFn(context.Background(), &process.ShellSessionSpec{
			Command:       commandToRun,
			Name:          commandName,
			Dir:           workDir,
			Env:           env,
			TTY:           true,
			Interactive:   step.Interactive,
			Masker:        iolib.GetContext().Masker(),
			EnableMasking: viper.GetBool("mask"),
		})
	}

	if step.Interactive {
		release := signals.SuspendInterruptExit()
		defer release()
	}
	return e.ExecuteShell(commandToRun, commandName, workDir, env, false)
}

// executeExecStep runs a custom command exec step: the command replaces the
// Atmos process entirely (shell exec semantics). Validated earlier to be the
// final step, so on Unix this never returns on success; on Windows the
// child's exit code is propagated.
func executeExecStep(commandToRun, commandName, workDir string, env []string) error {
	return replaceShellSessionFn(&process.ExecSpec{
		Command: commandToRun,
		Name:    commandName,
		Dir:     workDir,
		Env:     env,
	})
}
