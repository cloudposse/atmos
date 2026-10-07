package hooks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/automation"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// RunOption configures a Git hook invocation.
type RunOption func(*runOptions)

type runOptions struct {
	ctx            context.Context
	config         *schema.AtmosConfiguration
	stdout, stderr io.Writer
}

// WithContext supplies cancellation and deadlines for step execution.
func WithContext(ctx context.Context) RunOption {
	defer perf.Track(nil, "hooks.WithContext")()

	return func(o *runOptions) { o.ctx = ctx }
}

// WithAtmosConfig supplies project configuration to step handlers.
func WithAtmosConfig(config *schema.AtmosConfiguration) RunOption {
	defer perf.Track(nil, "hooks.WithAtmosConfig")()

	return func(o *runOptions) { o.config = config }
}

// WithOutputWriters routes step output to the host streams.
func WithOutputWriters(stdout, stderr io.Writer) RunOption {
	defer perf.Track(nil, "hooks.WithOutputWriters")()

	return func(o *runOptions) { o.stdout, o.stderr = stdout, stderr }
}

func runSteps(name string, entry schema.GitHookEntry, args []string, opts []RunOption) error {
	defer perf.Track(nil, "hooks.runSteps")()

	options := runOptions{ctx: context.Background(), stdout: os.Stdout, stderr: os.Stderr}
	for _, opt := range opts {
		opt(&options)
	}
	vars := step.NewVariables()
	vars.SetAtmosConfig(options.config)
	vars.ScriptArgs = slices.Clone(args)
	dir, err := resolveWorkingDir()
	if err != nil {
		return wrapHookError(name, err)
	}
	// Seed the step environment from the full process environment (plus the atmos.yaml global
	// env), exactly as custom commands do. Without it, the first env step would leave later
	// children with only the exported keys and no PATH.
	var globalEnv map[string]string
	if options.config != nil {
		globalEnv = options.config.Env
	}
	err = step.NewAutomationLibrary(vars, nil).RunSteps(options.ctx, entry.Steps, &automation.StepCall{
		WorkingDirectory: dir,
		ProcessEnv:       envpkg.MergeGlobalEnv(os.Environ(), globalEnv),
		Stdout:           options.stdout,
		Stderr:           options.stderr,
	})
	if err != nil {
		return wrapHookError(name, markStepTimeout(options.ctx, entry.Steps, err))
	}
	return nil
}

// resolveWorkingDir returns the directory hook steps and commands run in. Git itself runs hooks
// from the repository root, so a direct `atmos git hooks run` from a subdirectory must behave the
// same way. Outside a repository it falls back to the current directory.
func resolveWorkingDir() (string, error) {
	root, err := atmosgit.GetRoot()
	if err == nil && root != "" {
		return root, nil
	}
	cwd, cwdErr := os.Getwd()
	if cwdErr != nil {
		return "", cwdErr
	}
	log.Debug("Not inside a Git repository; running Git hook from the current directory", "dir", cwd, "error", err)
	return cwd, nil
}

// markStepTimeout reports a step's own `timeout:` expiry as a step timeout, the way custom
// commands do, instead of a bare "context deadline exceeded". The caller's cancellation is left
// untouched. RunSteps names the failing step in its error, which identifies the step here.
func markStepTimeout(ctx context.Context, tasks schema.Tasks, err error) error {
	if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errUtils.ErrStepTimeout) || ctx.Err() != nil {
		return err
	}
	for i := range tasks {
		name := tasks[i].Name
		if name == "" {
			name = fmt.Sprintf("step_%d", i+1)
		}
		if tasks[i].Timeout == "" || !strings.Contains(err.Error(), fmt.Sprintf("step %q:", name)) {
			continue
		}
		return errUtils.Build(errUtils.ErrStepTimeout).
			WithCause(err).
			WithExplanationf("Step '%s' did not finish within its timeout of %s and was canceled.", name, tasks[i].Timeout).
			WithHint("Raise the step's `timeout:` or make the work faster.").
			WithContext("step", name).
			WithContext("timeout", tasks[i].Timeout).
			Err()
	}
	return err
}
