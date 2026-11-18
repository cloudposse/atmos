package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/spf13/afero"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// ErrUnsupportedExecution identifies Terragrunt hook or extra_arguments semantics that the
// host-owned execution contract does not implement. Messages name only hook identifiers and
// command names, never the argv or evaluated values, so they are safe to show to the user.
var ErrUnsupportedExecution = errors.New("unsupported native Terragrunt execution configuration")

// unsupportedLifecycles are hook commands that name Terragrunt's own lifecycle rather than a
// Terraform command. The host has no equivalent lifecycle yet, so accepting them would silently
// never run the hook.
var unsupportedLifecycles = map[string]string{
	"terragrunt-read-config": "runs after Terragrunt reads configuration",
	"read-config":            "runs after Terragrunt reads configuration",
	"init-from-module":       "runs while Terragrunt downloads the module source",
}

// builtinHookExecutables are executable names Terragrunt intercepts instead of running them.
var builtinHookExecutables = map[string]struct{}{"tflint": {}}

// executionFromConfig converts the evaluated terraform block into data the host executes.
// It returns nil when the unit declares no hooks or extra arguments. Evaluation never runs a hook.
func executionFromConfig(terraform *config.TerraformConfig) (*model.Execution, error) {
	if terraform == nil {
		return nil, nil
	}
	result := &model.Execution{}
	for i := range terraform.ExtraArgs {
		result.ExtraArguments = append(result.ExtraArguments, extraArguments(&terraform.ExtraArgs[i]))
	}
	var err error
	if result.BeforeHooks, err = hooksFromConfig("before_hook", terraform.BeforeHooks); err != nil {
		return nil, err
	}
	if result.AfterHooks, err = hooksFromConfig("after_hook", terraform.AfterHooks); err != nil {
		return nil, err
	}
	if result.ErrorHooks, err = errorHooksFromConfig(terraform.ErrorHooks); err != nil {
		return nil, err
	}
	if len(result.ExtraArguments)+len(result.BeforeHooks)+len(result.AfterHooks)+len(result.ErrorHooks) == 0 {
		return nil, nil
	}
	return result, nil
}

// commandFunctions are the Terragrunt functions whose value depends on the Terraform command.
var commandFunctions = [][]byte{[]byte("get_terraform_command"), []byte("get_terraform_cli_args")}

// applyTimeExecution records how the apply command sees hooks and extra arguments when a
// plan is prepared. A saved plan is applied later from that preparation, while Terragrunt
// evaluates the configuration again for apply; without this, a hook enabled only for apply
// (for example `if = get_terraform_command() == "apply"`) would silently never run.
// Configuration that cannot observe the command evaluates identically, so it is not parsed twice.
func (e *unitEvaluator) applyTimeExecution(planned *model.Execution, parse func(*config.ParsingContext) (*config.TerragruntConfig, error)) (*model.Execution, []string, error) {
	read := e.pctx.FilesRead.Paths()
	if e.request.Command != "plan" || !observesCommand(e.environment.FS, append(slices.Clone(read), e.path)) {
		return planned, nil, nil
	}
	if err := rejectArgumentDependentExecution(e.environment.FS, append(slices.Clone(read), e.path)); err != nil {
		return nil, nil, err
	}
	// Apply is evaluated with the request's own download directory, so the working directory
	// matches the plan-time evaluation.
	applyRequest := *e.request
	applyRequest.Command, applyRequest.Arguments = "apply", nil
	apply := requestParsingContext(&applyRequest, e.path)
	// Dependency mock eligibility belongs to the plan being prepared; hook expressions see apply.
	apply.OriginalTerraformCommand = e.request.Command
	cfg, err := parse(apply)
	if err != nil {
		return nil, nil, err
	}
	// Configuration read only when evaluating for apply also shapes the reviewed plan.
	files := apply.FilesRead.Paths()
	if err := rejectArgumentDependentExecution(e.environment.FS, files); err != nil {
		return nil, nil, err
	}
	applied, err := executionFromConfig(cfg.Terraform)
	if err != nil {
		return nil, nil, err
	}
	if applied == nil {
		applied = &model.Execution{}
	}
	if planned == nil {
		planned = &model.Execution{}
	}
	if reflect.DeepEqual(*applied, *planned) {
		return nilIfEmpty(planned), files, nil
	}
	planned.Apply = applied
	return planned, files, nil
}

// observesCommand reports whether any configuration file the unit read can call a function
// that depends on the command. An unreadable file is assumed to, so apply is evaluated.
func observesCommand(filesystem afero.Fs, paths []string) bool {
	for _, path := range paths {
		contents, err := afero.ReadFile(filesystem, path)
		if err != nil {
			return true
		}
		for _, name := range commandFunctions {
			if callsFunction(contents, path, string(name)) {
				return true
			}
		}
	}
	return false
}

func nilIfEmpty(execution *model.Execution) *model.Execution {
	if execution.Apply == nil && len(execution.ExtraArguments)+len(execution.BeforeHooks)+len(execution.AfterHooks)+len(execution.ErrorHooks) == 0 {
		return nil
	}
	return execution
}

func extraArguments(source *config.TerraformExtraArguments) model.ExtraArguments {
	result := model.ExtraArguments{Commands: slices.Clone(source.Commands)}
	if source.Arguments != nil {
		result.Arguments = slices.Clone(*source.Arguments)
	}
	if source.RequiredVarFiles != nil {
		result.RequiredVarFiles = slices.Clone(*source.RequiredVarFiles)
	}
	if source.OptionalVarFiles != nil {
		result.OptionalVarFiles = slices.Clone(*source.OptionalVarFiles)
	}
	if source.EnvVars != nil && len(*source.EnvVars) > 0 {
		result.Environment = maps.Clone(*source.EnvVars)
	}
	return result
}

func hooksFromConfig(kind string, source []config.Hook) ([]model.Hook, error) {
	var result []model.Hook
	for i := range source {
		hook := &source[i]
		// A false condition disables the hook, exactly as it does for Terragrunt itself.
		if hook.If != nil && !*hook.If {
			continue
		}
		converted := model.Hook{Name: hook.Name, Commands: slices.Clone(hook.Commands), Execute: slices.Clone(hook.Execute)}
		if hook.WorkingDir != nil {
			converted.WorkingDirectory = *hook.WorkingDir
		}
		if hook.RunOnError != nil {
			converted.RunOnError = *hook.RunOnError
		}
		if hook.SuppressStdout != nil {
			converted.SuppressStdout = *hook.SuppressStdout
		}
		if err := validateHook(kind, &converted); err != nil {
			return nil, err
		}
		result = append(result, converted)
	}
	return result, nil
}

func errorHooksFromConfig(source []config.ErrorHook) ([]model.Hook, error) {
	var result []model.Hook
	for i := range source {
		hook := &source[i]
		converted := model.Hook{Name: hook.Name, Commands: slices.Clone(hook.Commands), Execute: slices.Clone(hook.Execute), OnErrors: slices.Clone(hook.OnErrors)}
		if hook.WorkingDir != nil {
			converted.WorkingDirectory = *hook.WorkingDir
		}
		if hook.SuppressStdout != nil {
			converted.SuppressStdout = *hook.SuppressStdout
		}
		if err := validateHook("error_hook", &converted); err != nil {
			return nil, err
		}
		for _, pattern := range converted.OnErrors {
			if _, err := regexp.Compile(pattern); err != nil {
				return nil, fmt.Errorf("%w: error_hook %q on_errors is not a valid regular expression", ErrUnsupportedExecution, converted.Name)
			}
		}
		result = append(result, converted)
	}
	return result, nil
}

func validateHook(kind string, hook *model.Hook) error {
	for _, command := range hook.Commands {
		if reason, unsupported := unsupportedLifecycles[command]; unsupported {
			return fmt.Errorf("%w: %s %q lists the %q lifecycle command, which %s; it is not implemented", ErrUnsupportedExecution, kind, hook.Name, command, reason)
		}
	}
	if len(hook.Execute) == 0 || hook.Execute[0] == "" {
		return fmt.Errorf("%w: %s %q must set a non-empty execute list", ErrUnsupportedExecution, kind, hook.Name)
	}
	if _, builtin := builtinHookExecutables[hook.Execute[0]]; builtin {
		return fmt.Errorf("%w: %s %q runs the built-in %q integration, which is not implemented", ErrUnsupportedExecution, kind, hook.Name, hook.Execute[0])
	}
	return nil
}

// rejectArgumentDependentExecution fails closed until saved apply arguments can be bound.
func rejectArgumentDependentExecution(filesystem afero.Fs, paths []string) error {
	for _, path := range paths {
		content, err := afero.ReadFile(filesystem, path)
		if err != nil || callsFunction(content, path, "get_terraform_cli_args") {
			return fmt.Errorf("%w: argument-dependent saved-plan execution cannot be bound before apply", ErrUnsupportedExecution)
		}
	}
	return nil
}

// callsFunction distinguishes evaluated function calls from comments and literal strings.
// Invalid syntax remains conservative; the ordinary parser reports its diagnostics.
func callsFunction(contents []byte, path, name string) bool {
	if strings.HasSuffix(path, ".json") {
		var value any
		if json.Unmarshal(contents, &value) != nil {
			return true
		}
		return jsonCallsFunction(value, path, name)
	}
	if !bytes.Contains(contents, []byte(name)) {
		return false
	}
	file, diagnostics := hclsyntax.ParseConfig(contents, path, hcl.InitialPos)
	if diagnostics.HasErrors() {
		return true
	}
	return expressionCallsFunction(file.Body.(*hclsyntax.Body), name)
}

// JSON HCL interprets string values as templates, while object keys remain literal.
func jsonCallsFunction(value any, path, name string) bool {
	switch item := value.(type) {
	case map[string]any:
		for _, nested := range item {
			if jsonCallsFunction(nested, path, name) {
				return true
			}
		}
	case []any:
		for _, nested := range item {
			if jsonCallsFunction(nested, path, name) {
				return true
			}
		}
	case string:
		expression, diagnostics := hclsyntax.ParseTemplate([]byte(item), path, hcl.InitialPos)
		if diagnostics.HasErrors() {
			return strings.Contains(item, name)
		}
		return expressionCallsFunction(expression, name)
	}
	return false
}

func expressionCallsFunction(expression hclsyntax.Node, name string) bool {
	found := false
	diagnostics := hclsyntax.VisitAll(expression, func(node hclsyntax.Node) hcl.Diagnostics {
		if call, ok := node.(*hclsyntax.FunctionCallExpr); ok && call.Name == name {
			found = true
		}
		return nil
	})
	return found || diagnostics.HasErrors()
}
