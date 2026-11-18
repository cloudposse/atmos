package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/hashicorp/hcl/v2"
	"github.com/spf13/afero"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// readOnlyDenial is the text of the guard that stops discovery from running commands, decrypting,
// or using the network.
const readOnlyDenial = "read-only discovery cannot"

// unsupportedSettingHints maps each setting the native adaptation rejects to its remedy.
var unsupportedSettingHints = map[string]string{
	"terraform_binary":       "Select the executable with `components.terraform.command` in atmos.yaml, for example `tofu`, and remove `terraform_binary` from the unit.",
	"errors":                 "Remove the `errors` block. Retry failing steps with a workflow step `retry:` setting instead.",
	"exclude":                "Remove the `exclude` block and leave the unit out with `discovery.excluded_paths` in atmos.yaml.",
	"iam_role":               "Remove the IAM setting and authenticate with an Atmos identity, for example `atmos terraform plan --identity <name>`.",
	"iam_web_identity_token": "Remove `iam_web_identity_token` and authenticate with an Atmos identity, for example `atmos terraform plan --identity <name>`.",
	"feature":                "Remove the `feature` blocks and pass the values as Atmos stack variables instead.",
}

// unsupportedSetting reports a Terragrunt setting the native adaptation does not implement.
func unsupportedSetting(key, file string) *model.Problem {
	hint := unsupportedSettingHints[key]
	problem := &model.Problem{
		Code: model.CodeUnsupportedSetting, Summary: fmt.Sprintf("Unsupported Terragrunt setting `%s`", key),
		Detail: fmt.Sprintf("The native Terragrunt adaptation does not implement `%s`, so this unit cannot be planned or applied.", key),
		File:   file, Key: key, Safe: true, Err: ErrUnsupportedExecution,
	}
	if hint != "" {
		problem.Hints = []string{hint}
	}
	return problem
}

// problemFor classifies an evaluation error. A Problem passes through with its file filled in.
// An HCL diagnostic becomes a syntax or evaluation problem at the diagnostic's position.
func problemFor(err error, file string) error {
	if err == nil {
		return nil
	}
	var problem *model.Problem
	if errors.As(err, &problem) {
		if problem.File == "" {
			problem.File = file
		}
		return problem
	}
	var diagnostics hcl.Diagnostics
	if errors.As(err, &diagnostics) && len(diagnostics) > 0 {
		return diagnosticProblem(diagnostics, file, err)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	// Other evaluator errors can contain evaluated values, so their text stays out of the safe fields.
	return &model.Problem{
		Code: model.CodeEvaluationError, Summary: "Terragrunt could not evaluate the configuration", File: file, Err: err,
		Hints: []string{"Check the unit's configuration."},
	}
}

// diagnosticProblem describes the first error diagnostic. A diagnostic that points at a source range
// and carries neither an expression nor an evaluation context comes from the parser or the block
// schema, so it holds no evaluated values and its detail is safe. A conversion diagnostic can lack
// an expression while it quotes an evaluated value, so a missing expression alone proves nothing.
func diagnosticProblem(diagnostics hcl.Diagnostics, file string, cause error) *model.Problem {
	diagnostic := diagnostics[0]
	for _, candidate := range diagnostics {
		if candidate.Severity == hcl.DiagError {
			diagnostic = candidate
			break
		}
	}
	problem := &model.Problem{Code: model.CodeEvaluationError, Summary: "Terragrunt configuration error: " + diagnostic.Summary, File: file, Detail: diagnostic.Detail, Safe: parserDiagnostic(diagnostic), Err: cause}
	if diagnostic.Subject != nil {
		if diagnostic.Subject.Filename != "" {
			problem.File = diagnostic.Subject.Filename
		}
		problem.Line, problem.Column = diagnostic.Subject.Start.Line, diagnostic.Subject.Start.Column
	}
	switch {
	case strings.Contains(diagnostic.Detail, readOnlyDenial):
		problem.Code, problem.Safe = model.CodeEffectDenied, false
		problem.Summary = "Terragrunt configuration runs a command, reads a secret, or uses the network during discovery"
		problem.Hints = []string{"Discovery only reads files. Keep `run_cmd`, `sops_decrypt_file`, and other effectful functions out of the values that `terraform.source` and `dependency` blocks use; they can stay in `inputs`, which are evaluated when the unit runs."}
	case problem.Safe:
		problem.Code = model.CodeSyntaxError
		problem.Summary = "Invalid Terragrunt configuration: " + diagnostic.Summary
		problem.Hints = []string{"Fix the HCL at the reported position, then run the command again."}
	default:
		problem.Hints = []string{"Check the expression at the reported position."}
	}
	return problem
}

// parserDiagnostic reports whether a diagnostic originates from parsing or schema decoding. Those
// diagnostics always carry the source range they describe and never an expression or evaluation
// context, which is how a diagnostic about an evaluated value is attached.
func parserDiagnostic(diagnostic *hcl.Diagnostic) bool {
	return diagnostic.Expression == nil && diagnostic.EvalContext == nil && diagnostic.Subject != nil && diagnostic.Subject.Filename != ""
}

// unsupportedHook names a rejected hook, with a custom summary and remedy.
func unsupportedHook(key, summary, detail string, hints ...string) *model.Problem {
	return &model.Problem{Code: model.CodeUnsupportedHook, Summary: summary, Detail: detail, Hints: hints, Key: key, Safe: true, Err: ErrUnsupportedExecution}
}

// rejectUnsupported fails a unit that uses a setting the native adaptation does not implement. It
// checks everything a partial or full parse decodes, in a fixed order, so the first rejection is
// stable. Hook and extra_arguments semantics are checked by executionFromConfig.
func rejectUnsupported(cfg *config.TerragruntConfig, path string) error {
	switch {
	case cfg.TerraformBinary != "":
		return unsupportedSetting("terraform_binary", path)
	case cfg.Errors != nil:
		return unsupportedSetting("errors", path)
	case cfg.Exclude != nil:
		return unsupportedSetting("exclude", path)
	case len(cfg.FeatureFlags) > 0:
		return unsupportedSetting("feature", path)
	case cfg.IamRole != "":
		return unsupportedSetting("iam_role", path)
	case cfg.IamWebIdentityToken != "":
		return unsupportedSetting("iam_web_identity_token", path)
	}
	if cfg.Terraform != nil {
		if _, err := executionFromConfig(cfg.Terraform); err != nil {
			return hookProblem(err, path)
		}
	}
	return nil
}

func workspaceDependencyProblem(path, workspace string) *model.Problem {
	return &model.Problem{
		Code: model.CodeWorkspace, Summary: "Dependency outputs are not supported with a non-default Terraform workspace",
		Detail: fmt.Sprintf("TF_WORKSPACE is `%s` and this unit declares a `dependency` block. Reading dependency outputs from a non-default workspace needs a dependency backend adapter that does not exist yet.", workspace),
		Hints:  []string{"Unset `TF_WORKSPACE` for this command, or run the unit's dependencies and this unit in the default workspace."},
		File:   path, Key: "dependency", Safe: true, Err: ErrUnsupportedExecution,
	}
}

func missingSourceProblem(path string) *model.Problem {
	return &model.Problem{
		Code: model.CodeMissingSource, Summary: "The unit has no `terraform.source`",
		Detail: "Native Terragrunt support runs units that name a module with `terraform { source = ... }`. A unit with inline Terraform files is not supported.",
		Hints:  []string{"Add `terraform { source = \"...\" }` to the unit, or move the inline Terraform files into a module directory and reference it."},
		File:   path, Key: "terraform.source", Safe: true, Err: ErrUnsupportedExecution,
	}
}

// sourceDirectoryProblem keeps the evaluated source out of the shown detail, because it can hold a
// value that is not registered for masking. The cause still carries it for local debugging.
func sourceDirectoryProblem(path, source string) *model.Problem {
	return &model.Problem{
		Code: model.CodeInvalidSource, Summary: "The module `source` is not an existing local directory",
		Detail: "`terraform.source` resolves to a local path that does not exist or is not a directory.",
		Hints:  []string{"Point `terraform.source` at a module directory relative to the unit, or use a Git or HTTPS module source."},
		File:   path, Key: "terraform.source", Safe: true, Err: fmt.Errorf("%w: local source %q is not a directory", ErrUnsupportedExecution, source),
	}
}

// hookProblem keeps the hook diagnostics from executionFromConfig, which name hooks but no values.
func hookProblem(err error, path string) error {
	var problem *model.Problem
	if errors.As(err, &problem) {
		if problem.File == "" {
			problem.File = path
		}
		return problem
	}
	return err
}

func mustRel(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil {
		return rel
	}
	return path
}

// localSourceProblem checks that a local module source is an existing directory. A missing path or
// a file is reported as such; any other failure, such as a permission error, is reported as an
// unreadable source and keeps its cause.
func localSourceProblem(filesystem afero.Fs, path, source, resolved string) *model.Problem {
	info, err := filesystem.Stat(resolved)
	switch {
	case errors.Is(err, os.ErrNotExist), err == nil && !info.IsDir():
		return sourceDirectoryProblem(path, source)
	case err != nil:
		return &model.Problem{
			Code: model.CodeInvalidSource, Summary: "The module `source` directory cannot be read",
			Detail: "Checking the local `terraform.source` directory failed with a filesystem error other than a missing path.",
			Hints:  []string{"Check the permissions of the module directory and its parent directories."},
			File:   path, Key: "terraform.source", Safe: true, Err: err,
		}
	}
	return nil
}

// dependencyProblem keeps evaluated target paths out of safe details; the cause remains available
// for classification and local debugging.
func dependencyProblem(file, _ string, err error) *model.Problem {
	summary := "A dependency points at a unit that cannot be read"
	hint := "Create the target unit, or fix `config_path` in the `dependency` or `dependencies` block."
	if errors.Is(err, os.ErrNotExist) {
		summary = "A dependency points at a path that does not exist"
		hint = "The target may have been deleted or renamed. Fix `config_path` in the `dependency` or `dependencies` block, or remove the dependency."
	}
	return &model.Problem{Code: model.CodeDependency, Summary: summary, Detail: "The dependency target could not be read.", Hints: []string{hint}, File: file, Key: "dependency", Safe: true, Err: err}
}

// localSubdirSourceProblem checks a local `dir//subdir` module source. Terragrunt copies the tree
// before `//` and runs the subdirectory, so both must exist; each is reported like a plain local
// source. A source without a local copied tree has nothing to check.
func localSubdirSourceProblem(filesystem afero.Fs, path, source string) *model.Problem {
	root, ok := copiedSourceRoot(filepath.Dir(path), source)
	if !ok {
		return nil
	}
	if problem := localSourceProblem(filesystem, path, source, root); problem != nil {
		return problem
	}
	_, subdirectory, _ := strings.Cut(filepath.ToSlash(source), "//")
	if subdirectory == "" {
		return nil
	}
	return localSourceProblem(filesystem, path, source, filepath.Join(root, filepath.FromSlash(subdirectory)))
}
