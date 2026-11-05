package hooks

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/terminal"
)

const (
	// ArgsEnvVar holds the arguments Git passed to the hook, joined by single spaces. Shell steps
	// also receive them as positional parameters ($1, "$@"), which keeps arguments with spaces intact.
	ArgsEnvVar = "ATMOS_GIT_HOOK_ARGS"

	// StdinEnvVar holds the path of a file with the data Git piped to the hook on standard input.
	// It is set only for the hooks Git writes data to, for example the refs a pre-push hook is
	// about to update. The file is removed when the hook ends.
	StdinEnvVar = "ATMOS_GIT_HOOK_STDIN"

	stdinFilePattern = "atmos-git-hook-stdin-*"

	// Wraps a failure to save the captured input.
	saveStdinFormat = "saving the hook's standard input: %w"
	stdinFileMode   = 0o600
)

// hooksWithStdin are the Git hooks that receive data on standard input. Reading the input of any
// other hook could block on a terminal or on a pipe that Git never closes.
var hooksWithStdin = map[string]struct{}{
	"pre-push": {}, "pre-receive": {}, "post-receive": {}, "post-rewrite": {},
	"reference-transaction": {}, "proc-receive": {},
}

// hookInput is the standard input of a hook, captured once.
type hookInput struct {
	// stdin hands the captured data to the first shell step. It is nil when nothing was captured.
	stdin *step.HookStdin
	// path is the temporary file that holds the data, or "".
	path string
}

// cleanup removes the temporary file.
func (h hookInput) cleanup() {
	if h.path != "" {
		_ = os.Remove(h.path)
	}
}

// captureHookStdin reads the data Git piped to a hook once, writes it to a temporary file, and
// keeps it for the first shell step. Without this, the first step to read standard input would
// drain it and every later step, including embedded scripts, would see nothing. A hook that Git
// does not write to, or one run from a terminal, captures nothing.
func captureHookStdin(hookName string, supplied io.Reader) (hookInput, error) {
	defer perf.Track(nil, "hooks.captureHookStdin")()

	if _, ok := hooksWithStdin[hookName]; !ok {
		return hookInput{}, nil
	}
	reader := supplied
	if reader == nil {
		if stdinIsTerminal() {
			return hookInput{}, nil
		}
		reader = os.Stdin
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return hookInput{}, fmt.Errorf("reading the hook's standard input: %w", err)
	}
	file, err := os.CreateTemp("", stdinFilePattern)
	if err != nil {
		return hookInput{}, fmt.Errorf(saveStdinFormat, err)
	}
	path := file.Name()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return hookInput{}, fmt.Errorf(saveStdinFormat, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return hookInput{}, fmt.Errorf(saveStdinFormat, err)
	}
	if err := os.Chmod(path, stdinFileMode); err != nil {
		_ = os.Remove(path)
		return hookInput{}, fmt.Errorf(saveStdinFormat, err)
	}
	return hookInput{stdin: step.NewHookStdin(data), path: path}, nil
}

// stdinIsTerminal reports whether the process's standard input is a terminal. It is a variable so
// tests can stub it.
var stdinIsTerminal = func() bool {
	return terminal.New().IsTTY(terminal.Stdin)
}

// hookEnvironment returns the environment entries that describe the hook invocation.
func hookEnvironment(args []string, stdinPath string) []string {
	env := []string{ArgsEnvVar + "=" + strings.Join(args, " ")}
	if stdinPath != "" {
		env = append(env, StdinEnvVar+"="+stdinPath)
	}
	return env
}
