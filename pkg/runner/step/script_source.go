package step

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// scriptLiteralField is the LiteralFields entry that keeps a script body from being rendered.
const scriptLiteralField = "script"

// scriptInterpreterExtensions maps a lowercase script file extension to the interpreter Atmos
// infers for it. It is the only place the mapping lives. Every entry is an interpreter Atmos
// documents for type: script: the embedded starlark runtime, or an external interpreter that
// process.ScriptInvocation knows how to feed a script body.
var scriptInterpreterExtensions = map[string]string{
	".star": "starlark",
	".py":   "python3",
	".sh":   "sh",
	".bash": "bash",
}

// scriptInferableExtensions lists the extensions in scriptInterpreterExtensions for error hints.
const scriptInferableExtensions = ".star (starlark), .py (python3), .sh (sh), .bash (bash)"

// goGetterForcedGetters are the go-getter forced-getter prefixes (for example `git::`) that mark
// a source as remote even when the rest of the value has no URL scheme.
var goGetterForcedGetters = []string{"git::", "hg::", "s3::", "gcs::", "http::", "https::", "file::", "oci::", "smb::"}

// inferScriptInterpreter returns the interpreter Atmos infers from a script file path, matching
// the extension case-insensitively. It reports false for an unknown extension or no extension.
func inferScriptInterpreter(path string) (string, bool) {
	interpreter, ok := scriptInterpreterExtensions[strings.ToLower(filepath.Ext(path))]
	return interpreter, ok
}

// scriptSourcePath returns the step's source as a path string. It reports false when source is
// unset, and an error when the value is not a non-empty string or names a remote location.
func scriptSourcePath(step *schema.WorkflowStep) (string, bool, error) {
	if step.Source == nil {
		return "", false, nil
	}
	path, ok := step.Source.(string)
	if !ok || strings.TrimSpace(path) == "" {
		return "", false, errUtils.Build(errUtils.ErrScriptSourceInvalid).
			WithContext("step", step.Name).
			WithContext("type", step.Type).
			WithContext("field", "source").
			WithHint("Set `source` to the path of a local script file, for example `scripts/check.star`. A source map is only supported by `type: workdir`.").
			Err()
	}
	if err := rejectRemoteScriptSource(step, path); err != nil {
		return "", false, err
	}
	return path, true, nil
}

// rejectRemoteScriptSource fails when path looks like a URL or a go-getter source.
func rejectRemoteScriptSource(step *schema.WorkflowStep, path string) error {
	if !isRemoteScriptSource(path) {
		return nil
	}
	return errUtils.Build(errUtils.ErrScriptSourceRemote).
		WithContext("step", step.Name).
		WithContext("type", step.Type).
		WithContext("source", path).
		WithHint("Download the script first, or inline it with `script`. Only local file paths are supported.").
		Err()
}

// isRemoteScriptSource reports whether path looks like a URL or a go-getter source.
func isRemoteScriptSource(path string) bool {
	if strings.Contains(path, "://") {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(path))
	for _, prefix := range goGetterForcedGetters {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// validateScriptBody checks the script/source pairing of a script step and returns the source
// path when the body comes from a file. Exactly one of script and source must be set.
func validateScriptBody(step *schema.WorkflowStep) (string, bool, error) {
	source, hasSource, err := scriptSourcePath(step)
	if err != nil {
		return "", false, err
	}
	switch {
	case hasSource && step.Script != "":
		return "", false, scriptSourceConflict(step)
	case !hasSource && step.Script == "":
		// Keep ErrStepFieldRequired in the chain: a script step without a body has always reported it.
		return "", false, errUtils.Build(fmt.Errorf("%w: %w", errUtils.ErrStepFieldRequired, errUtils.ErrScriptBodyRequired)).
			WithContext("step", step.Name).
			WithContext("type", step.Type).
			WithHint("Set either `script` (inline body) or `source` (path to a script file).").
			Err()
	}
	return source, hasSource, nil
}

// scriptSourceConflict reports a script step that sets both script and source.
func scriptSourceConflict(step *schema.WorkflowStep) error {
	return errUtils.Build(errUtils.ErrScriptSourceConflict).
		WithContext("step", step.Name).
		WithContext("type", step.Type).
		WithHint("Set either `script` (inline body) or `source` (path to a script file), not both.").
		Err()
}

// scriptInterpreterPath returns the file path the interpreter is inferred from: the step's
// source, or the file an included `script` body came from.
func scriptInterpreterPath(step *schema.WorkflowStep, source string) string {
	if source != "" {
		return source
	}
	return step.ScriptSource
}

// resolveScriptInterpreter returns the interpreter for a script step. An explicit interpreter
// always wins; otherwise it is inferred from the script file's extension.
func resolveScriptInterpreter(step *schema.WorkflowStep, interpreter, path string) (string, error) {
	if strings.TrimSpace(interpreter) != "" {
		return interpreter, nil
	}
	if inferred, ok := inferScriptInterpreter(path); ok {
		return inferred, nil
	}
	builder := errUtils.Build(errUtils.ErrScriptInterpreterRequired).
		WithContext("step", step.Name).
		WithContext("type", step.Type)
	if path != "" {
		builder = builder.WithContext("source", path)
	}
	return "", builder.
		WithHintf("Set `interpreter:` explicitly, or use a file with an inferable extension: %s.", scriptInferableExtensions).
		Err()
}

// scriptSourceFile resolves a script source path against workDir (a relative path is joined to
// it, and the result is made absolute) and reads the file. It returns the file text and the
// resolved absolute path.
func scriptSourceFile(step *schema.WorkflowStep, source, workDir string) (body, path string, err error) {
	path = source
	if !filepath.IsAbs(path) && workDir != "" {
		path = filepath.Join(workDir, path)
	}
	if abs, absErr := filepath.Abs(path); absErr == nil {
		path = abs
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return "", "", errUtils.Build(fmt.Errorf("%w: %w", errUtils.ErrScriptSourceRead, readErr)).
			WithContext("step", step.Name).
			WithContext("source", path).
			WithHint("Check that the file exists. A relative `source` resolves against the step's `working_directory`, or the current directory when that is unset.").
			Err()
	}
	return string(data), path, nil
}

// MaterializeScriptSource turns a script step that runs a `source` file into an equivalent
// inline step, for runners that execute script steps without the script step handler (for
// example inside a container). It reads the file, sets Script to its text, records the file as
// ScriptSource, clears Source, and marks the script as literal so a later render leaves the file
// exactly as written. It also infers an empty Interpreter from the script file's extension.
// A relative source resolves against workDir. When render is not nil, it renders a templated
// source.
//
// The step must be a copy the caller owns. Steps that are not script steps, and script steps
// that need no change, are left untouched.
func MaterializeScriptSource(step *schema.WorkflowStep, workDir string, render func(string) (string, error)) error {
	defer perf.Track(nil, "step.MaterializeScriptSource")()

	if step == nil || step.Type != schema.TaskTypeScript {
		return nil
	}
	// An inline script without a body is not rejected here: the runner decides what an empty
	// script means. Only a source file changes the step.
	source, hasSource, err := scriptSourcePath(step)
	if err != nil {
		return err
	}
	if hasSource {
		if step.Script != "" {
			return scriptSourceConflict(step)
		}
		if err := materializeScriptFile(step, source, workDir, render); err != nil {
			return err
		}
	}
	if strings.TrimSpace(step.Interpreter) != "" || step.ScriptSource == "" {
		return nil
	}
	step.Interpreter, err = resolveScriptInterpreter(step, "", step.ScriptSource)
	return err
}

// materializeScriptFile reads the script file named by source into the step's inline fields.
func materializeScriptFile(step *schema.WorkflowStep, source, workDir string, render func(string) (string, error)) error {
	// Render whenever a renderer is supplied: hosts pass renderers that honor the configured step
	// template delimiters, and rendering a plain path returns it unchanged.
	if render != nil {
		rendered, err := render(source)
		if err != nil {
			return TemplateFieldError(step, "source", err)
		}
		source = rendered
	}
	if err := rejectRemoteScriptSource(step, source); err != nil {
		return err
	}
	body, path, err := scriptSourceFile(step, source, workDir)
	if err != nil {
		return err
	}
	step.Script = body
	step.ScriptSource = path
	step.Source = nil
	if !slices.Contains(step.LiteralFields, scriptLiteralField) {
		step.LiteralFields = append(slices.Clone(step.LiteralFields), scriptLiteralField)
	}
	return nil
}
