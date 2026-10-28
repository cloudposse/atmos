package step

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	osexec "os/exec"
	"strings"
	"unicode"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	cfg "github.com/cloudposse/atmos/pkg/config"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
	_ "github.com/cloudposse/atmos/pkg/script/starlark" // Register the embedded interpreter.
	"github.com/cloudposse/atmos/pkg/template/delimiters"
)

// ScriptHandler executes inline scripts with an explicit interpreter.
type ScriptHandler struct {
	BaseHandler
}

type scriptInvocation struct {
	interpreter string
	script      string
	workDir     string
	// sourcePath is the absolute path of the file the script body came from, or empty for an
	// inline script that did not come from an include.
	sourcePath string
}

func init() {
	Register(&ScriptHandler{
		BaseHandler: NewBaseHandler(schema.TaskTypeScript, CategoryCommand, false),
	})
}

// Validate checks that the script step has required fields and does not use command.
func (h *ScriptHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.ScriptHandler.Validate")()

	if err := h.ValidateOutput(step); err != nil {
		return err
	}
	if step.Command != "" {
		return errUtils.Build(schema.ErrScriptStepInvalidField).
			WithContext("step", step.Name).
			WithContext("type", step.Type).
			WithContext("field", "command").
			Err()
	}
	source, _, err := validateScriptBody(step)
	if err != nil {
		return err
	}
	interpreter, err := h.validateInterpreter(step, source)
	if err != nil {
		return err
	}
	if _, embedded := script.Get(interpreter); embedded && step.Container.IsEnabled() {
		return fmt.Errorf("%w: embedded %s requires container: false", errUtils.ErrScript, strings.TrimSpace(interpreter))
	}
	return nil
}

// validateInterpreter returns the interpreter a script step will use, inferred from the script
// file when none is set. A templated interpreter or source is returned as written (or empty), so
// the final check happens at execution, once the templates are rendered.
func (h *ScriptHandler) validateInterpreter(step *schema.WorkflowStep, source string) (string, error) {
	if strings.TrimSpace(step.Interpreter) != "" {
		return step.Interpreter, nil
	}
	path := scriptInterpreterPath(step, source)
	if path == "" {
		return "", h.ValidateRequired(step, "interpreter", step.Interpreter)
	}
	interpreter, err := resolveScriptInterpreter(step, "", path)
	if err != nil && scriptPathMayBeTemplated(path) {
		// The step template delimiters are not known at validation time, so a path that could
		// still be a template is checked at execution, once it is rendered.
		return "", nil
	}
	return interpreter, err
}

// scriptPathMayBeTemplated reports whether path could hold a template expression under any
// configured delimiters. A plain path only uses letters, digits, and common path punctuation, so
// anything else (spaces, braces, brackets, angle brackets, or other symbols) is treated as a
// possible template, mirroring the conservative output-mode check.
func scriptPathMayBeTemplated(path string) bool {
	if delimiters.Default().Contains(path) {
		return true
	}
	for _, r := range path {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
		case strings.ContainsRune("._-/\\:~+@", r):
		default:
			return true
		}
	}
	return false
}

// Execute runs the script.
func (h *ScriptHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.ScriptHandler.Execute")()

	return h.execute(ctx, step, vars, nil)
}

// ExecuteWithWorkflow runs the script with workflow-level output defaults.
func (h *ScriptHandler) ExecuteWithWorkflow(ctx context.Context, step *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) (*StepResult, error) {
	defer perf.Track(nil, "step.ScriptHandler.ExecuteWithWorkflow")()

	return h.execute(ctx, step, vars, workflow)
}

func (h *ScriptHandler) execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) (*StepResult, error) {
	invocation, err := h.resolveInvocation(step, vars)
	if err != nil {
		return nil, err
	}
	// The raw interpreter may be a template, so Validate cannot tell whether it is embedded.
	// Check the rendered interpreter here, where the container opt-in is still visible.
	if _, embedded := script.Get(invocation.interpreter); embedded && step.Container.IsEnabled() {
		return nil, fmt.Errorf("%w: embedded %s requires container: false", errUtils.ErrScript, strings.TrimSpace(invocation.interpreter))
	}
	env, err := h.resolveEnv(step, vars)
	if err != nil {
		return nil, err
	}

	// Render a templated `output:` before anything runs, so an unknown mode fails the step.
	step, err = resolveOutputStep(step, vars)
	if err != nil {
		return nil, err
	}

	// Enforce the step's timeout by running the interpreter under a context deadline. The embedded
	// engine cancels its thread and subprocesses when the context ends.
	deadline, err := StartStepDeadline(ctx, step, vars)
	if err != nil {
		return nil, err
	}
	defer deadline.Stop()
	ctx = deadline.Context()

	// Script steps share the command-step output defaults: raw mode and no step labels unless
	// the step or workflow opts in through output or show.labels.
	writer := NewCommandOutputWriter(step, workflow)
	writer.writers = vars.OutputWriters
	if _, embedded := script.Get(invocation.interpreter); !embedded {
		// External scripts receive source text or no input, never the parent's terminal.
		writer = writer.WithStdin(nil)
	}
	var embedded script.Result
	stdout, stderr, err := writer.ExecuteWithIO(func(stdout, stderr io.Writer) error {
		if engine, ok := script.Get(invocation.interpreter); ok {
			inputs := step.Env
			if step.ScriptEnv != nil {
				inputs = step.ScriptEnv
			}
			resolved, resolveErr := vars.ResolveStepEnvMap(step, inputs)
			if resolveErr != nil {
				return resolveErr
			}
			var runErr error
			embedded, runErr = engine.Execute(ctx, script.Spec{
				Steps:        NewAutomationLibrary(vars, workflow),
				Parallel:     vars.automationParallel,
				OwnsTerminal: scriptOwnsTerminal(writer, vars),
				InstallTools: ScriptToolInstaller(vars.AtmosConfig),
				CI:           ci.NewReporter(vars.AtmosConfig),
				Name:         step.Name, Source: invocation.script, WorkingDirectory: invocation.workDir,
				SourcePath: invocation.sourcePath, ProjectRoot: scriptProjectRoot(vars),
				Env: resolved, ProcessEnv: env, Stdout: stdout, Stderr: stderr, DryRun: step.DryRun,
				Flags: vars.ScriptFlags(), Arguments: vars.ScriptArguments(),
				Component: ScriptComponentRef(vars), ResolveComponent: ScriptComponentResolver(vars),
				ProcessOverrides: step.ScriptProcessOverrides,
				Hook:             vars.ScriptHook,
				Args:             vars.ScriptArgs,
				Viewport:         scriptViewport,
			})
			return runErr
		}
		return process.RunScript(ctx, &process.ScriptSpec{
			Interpreter: invocation.interpreter,
			Script:      invocation.script,
			Name:        step.Name,
			Dir:         invocation.workDir,
			Env:         env,
			DryRun:      step.DryRun,
		}, stdout, stderr)
	})
	if err != nil {
		err = deadline.Wrap(err)
		err = WrapScriptInterpreterError(invocation.interpreter, err)
		return NewStepResult(stdout).
			WithError(stderr).
			WithMetadata("stdout", stdout).
			WithMetadata("stderr", stderr).
			WithMetadata(exitCodeMetadata, getExitCode(err)), err
	}

	value := stdout
	if embedded.HasOutput {
		value = embedded.Value
	}
	return NewStepResult(value).
		WithMetadata("stdout", stdout).
		WithMetadata("stderr", stderr).
		WithMetadata(exitCodeMetadata, 0), nil
}

// scriptOwnsTerminal carries host ownership through masking and capture wrappers.
func scriptOwnsTerminal(writer *OutputModeWriter, vars *Variables) bool {
	if writer.mode != OutputModeRaw || vars.automationParallel {
		return false
	}
	stderr := vars.OutputWriters.Stderr
	return stderr == nil || stderr == os.Stderr || stderr == iolib.GetContext().UI()
}

// WrapScriptInterpreterError adds a naming hint when an external interpreter could not be
// started because its name differs only by case from a registered embedded interpreter
// (for example `Starlark`). It does nothing on the success path and for any other failure,
// so the lookup runs only after an interpreter was already not found.
func WrapScriptInterpreterError(interpreter string, err error) error {
	defer perf.Track(nil, "step.WrapScriptInterpreterError")()

	if err == nil || !errors.Is(err, osexec.ErrNotFound) {
		return err
	}
	name := strings.TrimSpace(interpreter)
	lower := strings.ToLower(name)
	if lower == name {
		return err
	}
	if _, embedded := script.Get(lower); !embedded {
		return err
	}
	return errUtils.Build(err).
		WithHintf("Did you mean `%s`? Embedded interpreter names are case-sensitive.", lower).
		Err()
}

func (h *ScriptHandler) resolveInvocation(step *schema.WorkflowStep, vars *Variables) (scriptInvocation, error) {
	source, hasSource, err := validateScriptBody(step)
	if err != nil {
		return scriptInvocation{}, err
	}
	interpreter, err := vars.ResolveStepField(step, "interpreter", step.Interpreter)
	if err != nil {
		return scriptInvocation{}, vars.TemplateFieldError(step, "interpreter", err)
	}
	workDir := step.WorkingDirectory
	if workDir != "" {
		workDir, err = vars.ResolveStepField(step, "working_directory", workDir)
		if err != nil {
			return scriptInvocation{}, vars.TemplateFieldError(step, "working_directory", err)
		}
	}
	body, sourcePath, err := h.resolveScriptBody(step, vars, source, hasSource, workDir)
	if err != nil {
		return scriptInvocation{}, err
	}
	interpreter, err = resolveScriptInterpreter(step, interpreter, scriptInterpreterPath(step, sourcePath))
	if err != nil {
		return scriptInvocation{}, err
	}
	return scriptInvocation{interpreter: interpreter, script: body, workDir: workDir, sourcePath: sourcePath}, nil
}

// resolveScriptBody returns the script text and the absolute path it came from. An inline
// script is rendered as a template and keeps the path of the include it came from, if any. A
// `source` file is read at execution time and used as written, so a file never needs escaping.
func (h *ScriptHandler) resolveScriptBody(step *schema.WorkflowStep, vars *Variables, source string, hasSource bool, workDir string) (body, sourcePath string, err error) {
	if !hasSource {
		body, err = vars.ResolveStepField(step, "script", step.Script)
		if err != nil {
			return "", "", vars.TemplateFieldError(step, "script", err)
		}
		return body, step.ScriptSource, nil
	}
	source, err = vars.ResolveStepField(step, "source", source)
	if err != nil {
		return "", "", vars.TemplateFieldError(step, "source", err)
	}
	if err := rejectRemoteScriptSource(step, source); err != nil {
		return "", "", err
	}
	return scriptSourceFile(step, source, workDir)
}

func (h *ScriptHandler) resolveEnv(step *schema.WorkflowStep, vars *Variables) ([]string, error) {
	env := vars.EnvSlice()
	if len(env) == 0 {
		env = os.Environ()
	}
	// Global --profile/--identity selections live only in this process; forward them so a nested
	// atmos command the script starts (atmos.* or exec.run) runs with the same ones, as it does
	// for a standalone script. The step's own env is applied afterwards and so wins.
	env = ProcessEnvironmentWithSelection(env)
	if len(step.Env) == 0 {
		return env, nil
	}
	resolvedEnv, err := vars.ResolveStepEnvMap(step, step.Env)
	if err != nil {
		return nil, vars.TemplateFieldError(step, "env", err)
	}
	for key, value := range resolvedEnv {
		env = envpkg.UpdateEnvVar(env, key, value)
	}
	return env, nil
}

// scriptViewport shows a subprocess's live output in the step output viewport, the same one
// `output: viewport` gives a shell step, so exec.run(output="viewport") works in custom command,
// workflow, and hook script steps and not only in standalone scripts.
func scriptViewport(title string, run func(stdout, stderr io.Writer) error) error {
	_, _, err := NewOutputModeWriter(OutputModeViewport, title, nil).ExecuteWithIO(run)
	return err
}

// ProcessEnvironmentWithSelection adds the profile and identity chosen with global flags to env, so
// a child Atmos process selects the same ones.
func ProcessEnvironmentWithSelection(env []string) []string {
	defer perf.Track(nil, "step.ProcessEnvironmentWithSelection")()

	return script.ProcessEnvironment(env, script.SelectionEnv(cfg.GetActiveProfiles(nil), cfg.GlobalViper().GetString(cfg.IdentityFlagName)))
}

// ScriptFlags snapshots parsed flags without rendering user-provided strings.
// Custom commands preserve native types in their template roots; workflows use
// the string-valued flag map. Neither path needs to materialize prior step output.
func (v *Variables) ScriptFlags() map[string]any {
	defer perf.Track(nil, "step.Variables.ScriptFlags")()

	if v == nil {
		return nil
	}

	for _, key := range []string{"Flags", "flags"} {
		if value, exists := v.templateRoots[key]; exists {
			return scriptInputMap(value)
		}
	}
	return scriptInputMap(v.Flags)
}

// ScriptArguments snapshots named command arguments for embedded scripts.
func (v *Variables) ScriptArguments() map[string]any {
	defer perf.Track(nil, "step.Variables.ScriptArguments")()

	if v == nil {
		return nil
	}

	return scriptInputMap(v.templateRoots["Arguments"])
}

func scriptInputMap(value any) map[string]any {
	defer perf.Track(nil, "step.scriptInputMap")()

	switch input := value.(type) {
	case map[string]any:
		return maps.Clone(input)
	case map[string]string:
		result := make(map[string]any, len(input))
		for key, item := range input {
			result[key] = item
		}
		return result
	default:
		return nil
	}
}
