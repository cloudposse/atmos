package process

import (
	"os"

	log "github.com/cloudposse/atmos/pkg/logger"
)

// ExecSpec describes an exec step: a command that replaces the Atmos process
// entirely (shell exec semantics). Exec steps must be the final step — nothing
// runs after the replacement, and supervisor features (masking, output
// capture, retry, timeout) do not apply.
type ExecSpec struct {
	// Command is the raw shell command string, run via the system shell.
	Command string
	// Name is a logical name used for logging.
	Name string
	// Dir is the working directory to change to before the replacement ("" keeps the current directory).
	Dir string
	// Env is the fully merged environment, used verbatim. ATMOS_SHLVL is NOT
	// incremented: the session replaces Atmos rather than nesting under it.
	Env []string
	// DryRun logs the command without executing it.
	DryRun bool
}

// prepareExec performs the platform-independent part of an exec step: logging,
// dry-run short-circuit, environment defaulting, and working directory change.
// It reports whether execution should proceed.
func prepareExec(spec *ExecSpec) (env []string, proceed bool, err error) {
	// Masking cannot apply once the process is replaced; this is inherent to
	// exec steps (debug, not a warning - masking is enabled by default and a
	// warning would fire on every invocation).
	log.Debug("Replacing Atmos process with exec step (output masking does not apply)",
		"name", spec.Name, "command", spec.Command)

	if spec.DryRun {
		return nil, false, nil
	}

	env = spec.Env
	if len(env) == 0 {
		env = os.Environ()
	}

	if spec.Dir != "" {
		if err := os.Chdir(spec.Dir); err != nil {
			return nil, false, err
		}
	}
	return env, true, nil
}
