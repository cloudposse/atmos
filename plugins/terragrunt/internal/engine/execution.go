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
				return nil, setting(model.CodeUnsupportedHook, fmt.Sprintf("error_hook %q", converted.Name), "", fmt.Sprintf("Invalid `on_errors` pattern in error_hook %q", converted.Name),
					"An `on_errors` entry is not a valid regular expression.", "Fix the pattern; Atmos matches `on_errors` entries as Go regular expressions.")
			}
		}
		result = append(result, converted)
	}
	return result, nil
}

func validateHook(kind string, hook *model.Hook) error {
	for _, command := range hook.Commands {
		if reason, unsupported := unsupportedLifecycles[command]; unsupported {
			return setting(model.CodeUnsupportedHook, fmt.Sprintf("%s %q", kind, hook.Name), "", fmt.Sprintf("Unsupported hook command `%s` in %s %q", command, kind, hook.Name),
				fmt.Sprintf("The `%s` lifecycle command %s, which Atmos does not implement, so the hook would never run.", command, reason),
				"Remove the lifecycle command from `commands`. Hooks can name Terraform commands such as `plan`, `apply`, and `init`.")
		}
	}
	if len(hook.Execute) == 0 || hook.Execute[0] == "" {
		return setting(model.CodeUnsupportedHook, fmt.Sprintf("%s %q", kind, hook.Name), "", fmt.Sprintf("%s %q has no command to run", kind, hook.Name),
			"The hook sets no `execute` list or its first entry is empty.", "Set `execute = [\"program\", \"argument\"]` on the hook.")
	}
	if _, builtin := builtinHookExecutables[hook.Execute[0]]; builtin {
		return setting(model.CodeUnsupportedHook, fmt.Sprintf("%s %q", kind, hook.Name), "", fmt.Sprintf("Unsupported built-in `%s` hook in %s %q", hook.Execute[0], kind, hook.Name),
			fmt.Sprintf("Terragrunt intercepts a hook that executes `%s` and runs its own integration, which Atmos does not implement.", hook.Execute[0]),
			"Run the tool through a wrapper script, for example `execute = [\"./run-tflint.sh\"]`, or run it as a separate workflow step before the Terraform command.")
	}
	return nil
}
