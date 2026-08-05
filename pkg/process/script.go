package process

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ScriptSpec describes an inline script executed by a named interpreter.
type ScriptSpec struct {
	Interpreter string
	Script      string
	Name        string
	Dir         string
	Env         []string
	DryRun      bool
}

// ScriptInvocation returns the argv and optional stdin needed to execute an
// inline script directly with the selected interpreter.
func ScriptInvocation(interpreter, script string) ([]string, io.Reader) {
	defer perf.Track(nil, "process.ScriptInvocation")()

	interp := strings.TrimSpace(interpreter)
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(interp)), ".exe")

	switch name {
	case "python", "python2", "python3":
		return []string{interp, "-"}, strings.NewReader(script)
	case "node", "nodejs":
		return []string{interp, "-e", script}, nil
	case "bash", "dash", "ksh", "sh", "zsh":
		return []string{interp, "-c", script}, nil
	case "pwsh", "powershell":
		return []string{interp, "-NoProfile", "-NonInteractive", "-Command", "-"}, strings.NewReader(script)
	case "cmd":
		return []string{interp, "/S", "/C", script}, nil
	default:
		return []string{interp, "-"}, strings.NewReader(script)
	}
}

// NewScriptCommand builds an exec.Cmd for a script invocation.
func NewScriptCommand(ctx context.Context, spec *ScriptSpec) *exec.Cmd {
	defer perf.Track(nil, "process.NewScriptCommand")()

	if ctx == nil {
		ctx = context.Background()
	}
	argv, stdin := ScriptInvocation(spec.Interpreter, spec.Script)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // Interpreter is user-configured workflow input.
	cmd.Stdin = stdin
	cmd.Dir = spec.Dir
	if len(spec.Env) > 0 {
		cmd.Env = spec.Env
	} else {
		cmd.Env = os.Environ()
	}
	return cmd
}

// RunScript executes an inline script with attached output streams.
func RunScript(ctx context.Context, spec *ScriptSpec, stdout, stderr io.Writer) error {
	defer perf.Track(nil, "process.RunScript")()

	if spec.DryRun {
		return nil
	}
	cmd := NewScriptCommand(ctx, spec)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrProcessWaitFailed, err)
	}
	return nil
}

// FormatScriptDisplay returns a readable command preview for show.command.
func FormatScriptDisplay(interpreter, script string) string {
	defer perf.Track(nil, "process.FormatScriptDisplay")()

	if strings.TrimSpace(interpreter) == "" || strings.TrimSpace(script) == "" {
		return ""
	}
	return strings.TrimSpace(interpreter) + " <<'SCRIPT'\n" + script + "\nSCRIPT"
}
