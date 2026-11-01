package engine

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"

	"github.com/gruntwork-io/terragrunt/pkg/config"

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
