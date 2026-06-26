package tape

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ExecuteCommandDirect runs a command with stdout/stderr connected directly to the terminal.
// No buffering, no capturing - just raw execution. Returns only success/failure and duration.
// When connected to a real TTY, we let the subprocess detect it naturally for proper color support.
func ExecuteCommandDirect(ctx context.Context, cmd Command, workdir string, env []string) ExecutionResult {
	defer perf.Track(nil, "tape.ExecuteCommandDirect")()

	start := time.Now()

	shellCmd := exec.CommandContext(ctx, "bash", "-c", cmd.Text)
	shellCmd.Dir = workdir
	shellCmd.Stdin = os.Stdin
	shellCmd.Stdout = os.Stdout
	shellCmd.Stderr = os.Stderr

	// Set environment - inherit from parent process.
	// Do NOT set ATMOS_FORCE_TTY/ATMOS_FORCE_COLOR here - let the subprocess
	// detect the real TTY naturally for proper color profile detection.
	// The force flags are only for piping scenarios where you want color despite no TTY.
	shellCmd.Env = os.Environ()
	shellCmd.Env = append(shellCmd.Env, env...)

	err := shellCmd.Run()
	duration := time.Since(start)

	result := ExecutionResult{
		Command:  cmd,
		Duration: duration,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.Error = err
			result.ExitCode = -1
		}
	} else {
		result.Success = true
	}

	return result
}

// ExecutionResult contains the result of executing a command.
type ExecutionResult struct {
	Command  Command       // The command that was executed
	Success  bool          // True if exit code was 0
	ExitCode int           // Exit code from the command
	Stdout   string        // Standard output
	Stderr   string        // Standard error
	Duration time.Duration // How long the command took
	Error    error         // Any execution error (not exit code errors)
}

// ExecuteCommands runs the given commands in sequence and returns results.
// Commands are executed in the specified workdir with the given environment.
// Each command is run through the shell to support pipes and redirects.
func ExecuteCommands(ctx context.Context, commands []Command, workdir string, env []string) []ExecutionResult {
	defer perf.Track(nil, "tape.ExecuteCommands")()

	results := make([]ExecutionResult, 0, len(commands))

	for _, cmd := range commands {
		result := ExecuteCommand(ctx, cmd, workdir, env)
		results = append(results, result)
	}

	return results
}

// ExecuteCommand runs a single command and returns the result.
// Output is buffered and returned in the result.
func ExecuteCommand(ctx context.Context, cmd Command, workdir string, env []string) ExecutionResult {
	return executeCommandInternal(ctx, cmd, workdir, env, nil, nil)
}

// ExecuteCommandStreaming runs a command with output streamed to the provided writers.
// If stdout/stderr writers are provided, output is streamed directly to them.
// The result still contains exit code and duration but not the buffered output.
func ExecuteCommandStreaming(ctx context.Context, cmd Command, workdir string, env []string, stdout, stderr io.Writer) ExecutionResult {
	defer perf.Track(nil, "tape.ExecuteCommandStreaming")()

	return executeCommandInternal(ctx, cmd, workdir, env, stdout, stderr)
}

// executeCommandInternal is the shared implementation for command execution.
func executeCommandInternal(ctx context.Context, cmd Command, workdir string, env []string, stdoutWriter, stderrWriter io.Writer) ExecutionResult {
	start := time.Now()

	// Use shell to execute to support pipes, redirects, etc.
	shellCmd := exec.CommandContext(ctx, "bash", "-c", cmd.Text)
	shellCmd.Dir = workdir

	// Set environment - start with current env then add extras.
	shellCmd.Env = os.Environ()
	// Force TTY and color mode for atmos commands to get colored output.
	// This simulates terminal behavior without needing a real PTY.
	shellCmd.Env = append(shellCmd.Env, "ATMOS_FORCE_TTY=true", "ATMOS_FORCE_COLOR=true")
	shellCmd.Env = append(shellCmd.Env, env...)

	var stdoutBuf, stderrBuf bytes.Buffer

	// If streaming writers provided, use MultiWriter to both stream and capture.
	// Otherwise just capture to buffer.
	if stdoutWriter != nil {
		shellCmd.Stdout = io.MultiWriter(stdoutWriter, &stdoutBuf)
	} else {
		shellCmd.Stdout = &stdoutBuf
	}

	if stderrWriter != nil {
		shellCmd.Stderr = io.MultiWriter(stderrWriter, &stderrBuf)
	} else {
		shellCmd.Stderr = &stderrBuf
	}

	err := shellCmd.Run()
	duration := time.Since(start)

	result := ExecutionResult{
		Command:  cmd,
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		Duration: duration,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			result.Success = false
		} else {
			result.Error = err
			result.ExitCode = -1
			result.Success = false
		}
	} else {
		result.ExitCode = 0
		result.Success = true
	}

	return result
}

// ExecutionSummary provides a summary of command execution results.
type ExecutionSummary struct {
	Total   int
	Passed  int
	Failed  int
	Results []ExecutionResult
}

// Summarize creates a summary from execution results.
func Summarize(results []ExecutionResult) ExecutionSummary {
	summary := ExecutionSummary{
		Total:   len(results),
		Results: results,
	}

	for _, r := range results {
		if r.Success {
			summary.Passed++
		} else {
			summary.Failed++
		}
	}

	return summary
}
