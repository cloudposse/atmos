package output

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// initSubprocessWaitDelay bounds how long to wait for the init subprocess's I/O to drain after
// the context is cancelled. It mirrors the delay terraform-exec applies to its own commands.
const initSubprocessWaitDelay = 60 * time.Second

// InitWithVarsRequest describes one `terraform init` (or `tofu init`) invocation that must run
// with TF_VAR_* in its environment.
type InitWithVarsRequest struct {
	// Dir is the working directory init runs in (the component directory).
	Dir string
	// Executable is the resolved terraform/tofu binary.
	Executable string
	// Env is the complete environment for the subprocess, including TF_VAR_* entries.
	Env map[string]string
	// Stderr optionally receives a copy of the subprocess's stderr (the quiet-mode capture buffer).
	// Nil when the caller does not capture stderr.
	Stderr io.Writer
	// Reconfigure adds -reconfigure to the init arguments.
	Reconfigure bool
	// Upgrade adds -upgrade to the init arguments.
	Upgrade bool
}

// InitWithVarsFunc runs `terraform init` with the environment in the request. Errors must include
// the subprocess's stderr text so autoinit.Classify can inspect the diagnostic.
type InitWithVarsFunc func(ctx context.Context, req *InitWithVarsRequest) error

// WithInitWithVars overrides how `terraform init` runs when init.pass_vars is enabled (for testing).
//
// Terraform-exec refuses TF_VAR_* in Terraform.SetEnv and has no InitOption for -var/-var-file, so init-time
// variable dependencies (issue #1412) cannot travel through the terraform-exec runner. In that mode
// init is executed directly by this function with the full environment instead.
func WithInitWithVars(fn InitWithVarsFunc) ExecutorOption {
	defer perf.Track(nil, "output.WithInitWithVars")()

	return func(e *Executor) {
		e.initWithVars = fn
	}
}

// flagInitRunner is implemented by runners that run init themselves from plain reconfigure/upgrade
// flags rather than through terraform-exec's opaque InitOption values.
type flagInitRunner interface {
	InitWithFlags(ctx context.Context, reconfigure, upgrade bool) error
}

// runTerraformInit runs init through the runner, using the flag-based path when the runner offers it.
func runTerraformInit(ctx context.Context, runner TerraformRunner, reconfigure, upgrade bool) error {
	if r, ok := runner.(flagInitRunner); ok {
		return r.InitWithFlags(ctx, reconfigure, upgrade)
	}
	return runner.Init(ctx, buildInitOptions(reconfigure, upgrade)...)
}

// varsInitRunner decorates a TerraformRunner so `init` runs with TF_VAR_* in its environment while
// every other command (workspace, output) keeps using the wrapped runner and its TF_VAR-free env.
type varsInitRunner struct {
	TerraformRunner
	initWithVars  InitWithVarsFunc
	dir           string
	executable    string
	env           map[string]string
	stderrCapture *quietModeWriter
}

// InitWithFlags runs init as a subprocess with the full environment (including TF_VAR_*).
func (r *varsInitRunner) InitWithFlags(ctx context.Context, reconfigure, upgrade bool) error {
	defer perf.Track(nil, "output.varsInitRunner.InitWithFlags")()

	req := &InitWithVarsRequest{
		Dir:         r.dir,
		Executable:  r.executable,
		Env:         r.env,
		Reconfigure: reconfigure,
		Upgrade:     upgrade,
	}
	// Avoid wrapping a nil *quietModeWriter in a non-nil io.Writer.
	if r.stderrCapture != nil {
		req.Stderr = r.stderrCapture
	}
	return r.initWithVars(ctx, req)
}

// withVarsInit returns runner unchanged unless init.pass_vars exported component vars as TF_VAR_*
// (issue #1412), in which case it returns a runner whose init receives the full environment.
func (e *Executor) withVarsInit(
	runner TerraformRunner,
	config *ComponentConfig,
	environMap map[string]string,
	stderrCapture *quietModeWriter,
) TerraformRunner {
	if !config.PassVars || len(config.Vars) == 0 {
		return runner
	}
	initFn := e.initWithVars
	if initFn == nil {
		initFn = runInitSubprocess
	}
	return &varsInitRunner{
		TerraformRunner: runner,
		initWithVars:    initFn,
		dir:             config.ComponentPath,
		executable:      config.Executable,
		env:             environMap,
		stderrCapture:   stderrCapture,
	}
}

// withoutTerraformVarEnv returns a copy of env with every TF_VAR_* entry removed.
// Terraform-exec rejects those keys in SetEnv, so they must never reach the runner; they are delivered
// to the init subprocess by varsInitRunner instead. The input map is not modified.
//
// Known limitation: a user-defined TF_VAR_* in the component `env:` section is also stripped from
// the non-init runner environment. Tfexec already rejected such keys before (with an error), and
// `terraform output` / `workspace` commands do not consume variables.
func withoutTerraformVarEnv(env map[string]string) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		if strings.HasPrefix(k, tfVarEnvPrefix) {
			continue
		}
		out[k] = v
	}
	return out
}

// setRunnerEnv hands the runner its environment, minus TF_VAR_* which tfexec refuses.
func setRunnerEnv(runner TerraformRunner, environMap map[string]string) error {
	if len(environMap) == 0 {
		return nil
	}
	return runner.SetEnv(withoutTerraformVarEnv(environMap))
}

// runInitSubprocess is the default InitWithVarsFunc. It runs `<executable> init -input=false
// -no-color [-upgrade] [-reconfigure]` in req.Dir with req.Env, mirroring what terraform-exec runs
// (stdout is discarded like an unset tfexec stdout; stderr is captured and appended to the returned
// error, as tfexec does, and copied to req.Stderr when set).
func runInitSubprocess(ctx context.Context, req *InitWithVarsRequest) error {
	defer perf.Track(nil, "output.runInitSubprocess")()

	// The executable is the toolchain-resolved terraform/tofu binary from atmos configuration, not user input.
	cmd := exec.CommandContext(ctx, req.Executable, buildInitSubprocessArgs(req.Reconfigure, req.Upgrade)...) //nolint:gosec // G204: see above.
	cmd.Dir = req.Dir
	cmd.Env = buildInitSubprocessEnv(req.Env)
	if runtime.GOOS != "windows" {
		// Windows does not support SIGINT, so graceful cancellation is unavailable there.
		cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
		cmd.WaitDelay = initSubprocessWaitDelay
	}

	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if req.Stderr != nil {
		cmd.Stderr = io.MultiWriter(&stderr, req.Stderr)
	}

	log.Debug("Executing terraform init with TF_VAR_* environment", "dir", req.Dir, "executable", req.Executable)
	if err := cmd.Run(); err != nil {
		if text := strings.TrimSpace(stderr.String()); text != "" {
			return fmt.Errorf("%w\n%s", err, text)
		}
		return err
	}
	return nil
}

// buildInitSubprocessArgs renders the init command line.
func buildInitSubprocessArgs(reconfigure, upgrade bool) []string {
	args := []string{"init", "-input=false", "-no-color"}
	if upgrade {
		args = append(args, "-upgrade")
	}
	if reconfigure {
		args = append(args, "-reconfigure")
	}
	return args
}

// buildInitSubprocessEnv renders env as a sorted KEY=VALUE list and applies the automation
// overrides terraform-exec always applies to its own commands, so init behaves the same whether it
// runs through tfexec or directly.
func buildInitSubprocessEnv(env map[string]string) []string {
	merged := make(map[string]string, len(env)+5)
	maps.Copy(merged, env)

	// Keep logging from polluting captured stderr.
	for _, key := range []string{"TF_LOG", "TF_LOG_CORE", "TF_LOG_PATH", "TF_LOG_PROVIDER"} {
		merged[key] = ""
	}
	merged["TF_IN_AUTOMATION"] = "1"
	// Workspaces are selected explicitly, never through the environment.
	delete(merged, "TF_WORKSPACE")

	list := make([]string, 0, len(merged))
	for k, v := range merged {
		list = append(list, k+"="+v)
	}
	sort.Strings(list)
	return list
}
