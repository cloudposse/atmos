package engine

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// Names and limits of the built-in tflint hook. The executable is the hook program that Terragrunt
// replaces with its own tflint integration.
const (
	tflintExecutable   = "tflint"
	tflintExternalFlag = "--terragrunt-external-tflint"
	tflintConfigName   = ".tflint.hcl"
	// The host resolves this marker against the provisioned module directory when the hook runs.
	tflintOptionalVarFilePrefix = "--atmos-optional-var-file="
	// The host replaces this marker with one --var per input of the final prepared configuration, after it
	// has merged the stack's variables, so that TFLint and Terraform see the same values.
	tflintInputVariablesMarker = "--atmos-tflint-input-variables"
	// The host searches for this config only when the selected hook executes.
	tflintConfigSearchPrefix = "--atmos-tflint-config-search="
	// TFLint searches at most tflintMaxParents ancestors; this bounds the search for .tflint.hcl, like Terragrunt's default folder limit.
	tflintMaxParents = 100
)

// isLifecycleCommand reports whether a hook command names a Terragrunt stage that runs in the
// unit's configuration directory instead of the module directory.
func isLifecycleCommand(command string) bool {
	return command == model.LifecycleReadConfig || command == model.LifecycleReadConfigAlias || command == model.LifecycleInitFromModule
}

// finalizeExecution prepares evaluated hooks for the host: lifecycle hooks default to the unit's
// configuration directory, as in Terragrunt, and the built-in tflint hook becomes the two tflint
// invocations Terragrunt runs. The unit directory is where terragrunt.hcl lives.
func finalizeExecution(execution *model.Execution, path string) {
	if execution == nil {
		return
	}
	execution.BeforeHooks = finalizeHooks(execution.BeforeHooks, execution, path)
	execution.AfterHooks = finalizeHooks(execution.AfterHooks, execution, path)
	execution.ErrorHooks = finalizeHooks(execution.ErrorHooks, execution, path)
}

func finalizeHooks(hooks []model.Hook, execution *model.Execution, path string) []model.Hook {
	var result []model.Hook
	for i := range hooks {
		for _, hook := range splitLifecycle(&hooks[i], filepath.Dir(path)) {
			if len(hook.Execute) > 0 && hook.Execute[0] == tflintExecutable {
				result = append(result, expandTflint(&hook, execution, path)...)
				continue
			}
			result = append(result, hook)
		}
	}
	return result
}

// splitLifecycle gives lifecycle commands the unit directory as working directory. A hook that
// lists both lifecycle and Terraform commands, with no working_dir of its own, becomes two hooks so
// each command keeps its own default directory.
func splitLifecycle(hook *model.Hook, unitDir string) []model.Hook {
	var lifecycle, other []string
	for _, command := range hook.Commands {
		if isLifecycleCommand(command) {
			lifecycle = append(lifecycle, command)
		} else {
			other = append(other, command)
		}
	}
	if len(lifecycle) == 0 || hook.WorkingDirectory != "" {
		return []model.Hook{*hook}
	}
	first := *hook
	first.Commands, first.WorkingDirectory = lifecycle, unitDir
	if len(other) == 0 {
		return []model.Hook{first}
	}
	second := *hook
	second.Commands = other
	return []model.Hook{first, second}
}

// expandTflint replaces a hook that executes tflint with the two commands Terragrunt runs for it:
// `tflint --init` and the lint itself. Both run in the module directory against the .tflint.hcl
// that Terragrunt would find, with the unit's inputs and variable files passed as variables. A hook
// that lists several commands expands once per command, so each lint sees only the extra
// arguments of the command that is running.
func expandTflint(hook *model.Hook, execution *model.Execution, path string) []model.Hook {
	arguments := slices.DeleteFunc(slices.Clone(hook.Execute), func(arg string) bool { return arg == tflintExternalFlag })
	configFile := tflintConfigSearchPrefix + filepath.Dir(path)
	if explicit, found := explicitTflintConfig(arguments, filepath.Dir(path)); found {
		configFile = explicit
	}
	passthrough := withoutConfigFlag(arguments[1:])
	var result []model.Hook
	for _, command := range distinctHookCommands(hook.Commands) {
		variables := append([]string{tflintInputVariablesMarker}, tflintExtraVariables(command, execution)...)
		initHook := *hook
		initHook.Commands = []string{command}
		initHook.Execute = []string{tflintExecutable, "--init", "--config", configFile, "--chdir", "."}
		runHook := initHook
		runHook.Execute = append(append([]string{tflintExecutable, "--config", configFile, "--chdir", "."}, variables...), passthrough...)
		// The module directory is where tflint runs; a working_dir would only change the lock key upstream.
		// read-config runs before provisioning, when the host has no module directory, so it keeps the unit directory.
		initHook.WorkingDirectory, runHook.WorkingDirectory = "", ""
		if isLifecycleCommand(command) {
			initHook.WorkingDirectory, runHook.WorkingDirectory = filepath.Dir(path), filepath.Dir(path)
		}
		result = append(result, initHook, runHook)
	}
	return result
}

// distinctHookCommands preserves stage order while collapsing the read-config alias and duplicates.
func distinctHookCommands(commands []string) []string {
	var result []string
	for _, command := range commands {
		if command == model.LifecycleReadConfigAlias {
			command = model.LifecycleReadConfig
		}
		if !slices.Contains(result, command) {
			result = append(result, command)
		}
	}
	return result
}

// tflintConfig returns the configuration named by --config/-c, or the nearest .tflint.hcl.
func tflintConfig(arguments []string, unitDir string) (string, error) {
	if explicit, found := explicitTflintConfig(arguments, unitDir); found {
		return explicit, nil
	}
	dir := unitDir
	for range tflintMaxParents {
		candidate := filepath.Join(dir, tflintConfigName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", os.ErrNotExist
}

func explicitTflintConfig(arguments []string, unitDir string) (string, bool) {
	for i, arg := range arguments {
		for _, flag := range []string{"--config", "-c"} {
			if arg == flag && len(arguments) > i+1 {
				return anchored(arguments[i+1], unitDir), true
			}
			if value, found := strings.CutPrefix(arg, flag+"="); found {
				return anchored(value, unitDir), true
			}
		}
	}
	return "", false
}

func anchored(path, dir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(dir, path)
}

// withoutConfigFlag drops the --config/-c pair that tflintConfig already consumed.
func withoutConfigFlag(arguments []string) []string {
	var result []string
	for i := 0; i < len(arguments); i++ {
		arg := arguments[i]
		switch {
		case arg == "--config" || arg == "-c":
			i++
		case strings.HasPrefix(arg, "--config=") || strings.HasPrefix(arg, "-c="):
		default:
			result = append(result, arg)
		}
	}
	return result
}

// tflintExtraVariables mirrors the variables Terragrunt takes from extra_arguments that apply to
// the command the hook runs for.
func tflintExtraVariables(command string, execution *model.Execution) []string {
	var variables []string
	for i := range execution.ExtraArguments {
		extra := &execution.ExtraArguments[i]
		if slices.Contains(extra.Commands, command) {
			variables = append(variables, extraArgumentVariables(extra)...)
		}
	}
	return variables
}

// extraArgumentVariables lists the --var and --var-file flags one extra_arguments block implies.
// Both `-var=x` and `-var x` forms are recognized, as Terraform accepts both.
// TF_VAR_ entries of env_vars are not turned into --var flags, unlike Terragrunt: a flag would
// override terraform.tfvars, while Terraform ranks the environment below it. The hook inherits the
// phase environment, and TFLint reads TF_VAR_ variables with the same precedence as Terraform.
func extraArgumentVariables(extra *model.ExtraArguments) []string {
	var variables []string
	for i := 0; i < len(extra.Arguments); i++ {
		argument := extra.Arguments[i]
		for _, flag := range []string{"var", "var-file"} {
			if value, found := strings.CutPrefix(argument, "-"+flag+"="); found {
				variables = append(variables, "--"+flag+"="+value)
			} else if argument == "-"+flag && i+1 < len(extra.Arguments) {
				i++
				variables = append(variables, "--"+flag+"="+extra.Arguments[i])
			}
		}
	}
	for _, file := range extra.RequiredVarFiles {
		variables = append(variables, "--var-file="+file)
	}
	for _, file := range extra.OptionalVarFiles {
		variables = append(variables, tflintOptionalVarFilePrefix+file)
	}
	return variables
}
