package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// applyParity copies the settings that select how a unit runs from the evaluated configuration:
// terraform_binary, the errors block, the exclude block, and feature flags. Everything it returns
// is data; the host owns executable selection, retries, and unit exclusion.
func applyParity(cfg *config.TerragruntConfig, request model.Request, path string, result *model.Resolution) error {
	result.TerraformBinary = terraformBinary(cfg.TerraformBinary, path)
	policies, err := errorsFromConfig(cfg.Errors, path)
	if err != nil {
		return err
	}
	result.Errors = policies
	result.Exclude = excludeFromConfig(cfg.Exclude)
	flags, err := featureFlagValues(cfg.FeatureFlags, request.Features, path)
	if err != nil {
		return err
	}
	result.FeatureFlags = flags
	return nil
}

// terraformBinary keeps a bare name for the toolchain or PATH to resolve, and anchors a relative
// path at the unit's directory so the result does not depend on where Atmos runs.
func terraformBinary(binary, path string) string {
	binary = strings.TrimSpace(binary)
	if binary == "" || filepath.IsAbs(binary) || !strings.ContainsAny(binary, `/\`) {
		return binary
	}
	return filepath.Join(filepath.Dir(path), filepath.FromSlash(binary))
}

// errorsFromConfig validates and converts the errors block. A negative pattern ("!" prefix) vetoes
// a policy, exactly as it does in Terragrunt.
func errorsFromConfig(block *config.ErrorsConfig, path string) (*model.Errors, error) {
	if block == nil || len(block.Retry)+len(block.Ignore) == 0 {
		return nil, nil
	}
	result := &model.Errors{}
	for _, retry := range block.Retry {
		if retry == nil {
			continue
		}
		key := fmt.Sprintf("errors.retry %q", retry.Label)
		if retry.MaxAttempts < 1 {
			return nil, errorsProblem(key, path, fmt.Sprintf("`max_attempts` is %d in %s", retry.MaxAttempts, key), "A retry policy must allow at least one attempt.", "Set `max_attempts` to 1 or more.")
		}
		if retry.SleepIntervalSec < 0 {
			return nil, errorsProblem(key, path, fmt.Sprintf("`sleep_interval_sec` is %d in %s", retry.SleepIntervalSec, key), "The sleep between attempts cannot be negative.", "Set `sleep_interval_sec` to 0 or more.")
		}
		if int64(retry.MaxAttempts) > math.MaxUint32 || int64(retry.SleepIntervalSec) > math.MaxUint32 {
			return nil, errorsProblem(key, path, fmt.Sprintf("A value is too large in %s", key), "`max_attempts` and `sleep_interval_sec` must fit in 32 bits.", fmt.Sprintf("Use values no larger than %d.", uint32(math.MaxUint32)))
		}
		if err := checkPatterns(key, path, retry.RetryableErrors); err != nil {
			return nil, err
		}
		result.Retry = append(result.Retry, model.RetryPolicy{
			Name: retry.Label, Patterns: slices.Clone(retry.RetryableErrors), MaxAttempts: retry.MaxAttempts, SleepIntervalSec: retry.SleepIntervalSec,
		})
	}
	for _, ignore := range block.Ignore {
		if ignore == nil {
			continue
		}
		key := fmt.Sprintf("errors.ignore %q", ignore.Label)
		if err := checkPatterns(key, path, ignore.IgnorableErrors); err != nil {
			return nil, err
		}
		signals, err := signalValues(ignore.Signals)
		if err != nil {
			return nil, errorsProblem(key, path, fmt.Sprintf("`signals` cannot be represented in %s", key), "Signals must be plain values: strings, numbers, booleans, lists, or maps.", "Remove unsupported values from `signals`.")
		}
		result.Ignore = append(result.Ignore, model.IgnorePolicy{Name: ignore.Label, Patterns: slices.Clone(ignore.IgnorableErrors), Message: ignore.Message, Signals: signals})
	}
	return result, nil
}

func checkPatterns(key, path string, patterns []string) error {
	for _, pattern := range patterns {
		if _, err := regexp.Compile(strings.TrimPrefix(pattern, "!")); err != nil {
			return errorsProblem(key, path, fmt.Sprintf("Invalid error pattern in %s", key), "An entry of the error list is not a valid regular expression.", "Fix the pattern; Atmos matches these entries as Go regular expressions. A leading `!` makes a pattern veto the policy.")
		}
	}
	return nil
}

func errorsProblem(key, file, summary, detail, hint string) *model.Problem {
	return setting(model.CodeEvaluationError, key, file, summary, detail, hint)
}

// signalValues converts the cty values of an ignore block to plain JSON values.
func signalValues(signals map[string]cty.Value) (map[string]any, error) {
	if len(signals) == 0 {
		return nil, nil
	}
	result := make(map[string]any, len(signals))
	for _, name := range slices.Sorted(maps.Keys(signals)) {
		value, _ := signals[name].UnmarkDeep()
		if value.IsNull() || !value.IsWhollyKnown() {
			result[name] = nil
			continue
		}
		data, err := ctyjson.Marshal(value, value.Type())
		if err != nil {
			return nil, err
		}
		var plain any
		if err := json.Unmarshal(data, &plain); err != nil {
			return nil, err
		}
		result[name] = plain
	}
	return result, nil
}

// excludeFromConfig returns the exclude block only while its condition holds.
func excludeFromConfig(block *config.ExcludeConfig) *model.Exclude {
	if block == nil || !block.If {
		return nil
	}
	result := &model.Exclude{Actions: slices.Clone(block.Actions), NoRun: block.NoRun}
	if block.ExcludeDependencies != nil {
		result.ExcludeDependencies = *block.ExcludeDependencies
	}
	return result
}

// ExcludeListsAction reports whether the actions name the command. "all" matches every command and
// "all_except_output" every command but output; other entries match case-insensitively.
func ExcludeListsAction(actions []string, command string) bool {
	command = strings.ToLower(command)
	for _, action := range actions {
		switch {
		case action == "all":
			return true
		case action == "all_except_output" && command != "output":
			return true
		case strings.ToLower(action) == command:
			return true
		}
	}
	return false
}

// PreventsRun reports whether the exclude block makes a direct run of the command a no-op. Without
// no_run only an exact action match prevents the run; no_run = true also honors "all" and
// "all_except_output"; no_run = false never prevents it.
func PreventsRun(exclude *model.Exclude, command string) bool {
	if exclude == nil {
		return false
	}
	switch {
	case exclude.NoRun == nil:
		return slices.Contains(exclude.Actions, command)
	case !*exclude.NoRun:
		return false
	default:
		return ExcludeListsAction(exclude.Actions, command)
	}
}

// OperationCommand is the command name Terragrunt matches exclude actions against: destroy for
// `apply -destroy`, apply for deploy, otherwise the command itself.
func OperationCommand(command string, arguments []string) string {
	if command == "deploy" {
		command = "apply"
	}
	if command == "destroy" {
		return command
	}
	if command == "apply" {
		for _, argument := range slices.Backward(arguments) {
			name, value, hasValue := strings.Cut(argument, "=")
			if name != "-destroy" && name != "--destroy" {
				continue
			}
			if !hasValue {
				return "destroy"
			}
			if parsed, err := strconv.ParseBool(value); err == nil && parsed {
				return "destroy"
			}
			return command
		}
	}
	return command
}

// featureFlagValues returns the effective value of every declared feature, as a string. An override
// replaces the declared default; a boolean feature rejects an override that is not a boolean.
func featureFlagValues(flags []*config.FeatureFlag, overrides map[string]string, path string) (map[string]string, error) {
	if len(flags) == 0 {
		return nil, nil
	}
	result := make(map[string]string, len(flags))
	for _, flag := range flags {
		if flag == nil {
			continue
		}
		override, overridden := overrides[flag.Name]
		switch {
		case overridden:
			if flag.Default != nil && flag.Default.Type() == cty.Bool {
				if _, err := strconv.ParseBool(override); err != nil {
					return nil, setting(model.CodeEvaluationError, "feature "+flag.Name, path,
						fmt.Sprintf("Invalid value for the `%s` feature", flag.Name),
						fmt.Sprintf("The `%s` feature is a boolean, but its override is not `true` or `false`.", flag.Name),
						"Set the override to `true` or `false`, in `components.terraform.terragrunt.features` or in `TG_FEATURE`.")
				}
			}
			result[flag.Name] = override
		case flag.Default != nil:
			result[flag.Name] = ctyString(*flag.Default)
		}
	}
	return result, nil
}

func ctyString(value cty.Value) string {
	value, _ = value.UnmarkDeep()
	switch {
	case value.IsNull() || !value.IsKnown():
		return ""
	case value.Type() == cty.String:
		return value.AsString()
	case value.Type() == cty.Bool:
		return strconv.FormatBool(value.True())
	case value.Type() == cty.Number:
		return value.AsBigFloat().Text('f', -1)
	}
	data, err := ctyjson.Marshal(value, value.Type())
	if err != nil {
		return ""
	}
	return string(data)
}

// ParseFeatureEnvironment reads the comma-separated name=value list that Terragrunt accepts in
// TG_FEATURE.
func ParseFeatureEnvironment(value string) (map[string]string, error) {
	result := map[string]string{}
	for _, entry := range strings.Split(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, raw, found := strings.Cut(entry, "=")
		name = strings.TrimSpace(name)
		if !found || name == "" {
			return nil, setting(model.CodeEvaluationError, "TG_FEATURE", "",
				fmt.Sprintf("Invalid feature override %q in TG_FEATURE", name),
				"Each entry of TG_FEATURE must look like `name=value`.",
				"Set `TG_FEATURE=name=value[,name2=value2]`.")
		}
		result[name] = strings.TrimSpace(raw)
	}
	return result, nil
}

// unusedFeatureOverrides names the overrides that no discovered unit declares, in sorted order.
func unusedFeatureOverrides(overrides map[string]string, declared map[string]bool) []string {
	var unused []string
	for name := range overrides {
		if !declared[name] {
			unused = append(unused, name)
		}
	}
	slices.Sort(unused)
	return unused
}

// WarningFeatureOverrideUnused is reported when an override names a feature no discovered unit declares.
const WarningFeatureOverrideUnused = "feature_override_unused"

// WarningReadConfigBeforeHook is reported for a before_hook that lists the read-config lifecycle.
const WarningReadConfigBeforeHook = "read_config_before_hook_ignored"

// featureOverrideWarnings names every override that matches no declared feature, so a typo is
// visible instead of silently leaving the default in place. It is a warning, not an error: an
// override can legitimately name a feature that only some revisions of the project declare.
func featureOverrideWarnings(overrides map[string]string, inventory []model.DiscoveredUnit) []model.Warning {
	if len(overrides) == 0 {
		return nil
	}
	declared := map[string]bool{}
	for i := range inventory {
		for name := range inventory[i].Config.FeatureFlags {
			declared[name] = true
		}
	}
	var warnings []model.Warning
	for _, name := range unusedFeatureOverrides(overrides, declared) {
		warnings = append(warnings, model.Warning{
			Code:    WarningFeatureOverrideUnused,
			Message: fmt.Sprintf("The feature override `%s` does not match any `feature` block of the discovered units; check the spelling in `components.terraform.terragrunt.features` or `TG_FEATURE`", name),
		})
	}
	return warnings
}

// lifecycleWarnings reports hooks that can never run: Terragrunt runs read-config hooks as after
// hooks only, and ignores a before_hook that lists it.
func lifecycleWarnings(terraform *config.TerraformConfig, path string) []model.Warning {
	if terraform == nil {
		return nil
	}
	var warnings []model.Warning
	for i := range terraform.BeforeHooks {
		hook := &terraform.BeforeHooks[i]
		if hook.If != nil && !*hook.If {
			continue
		}
		for _, command := range hook.Commands {
			if command == model.LifecycleReadConfig || command == model.LifecycleReadConfigAlias {
				warnings = append(warnings, model.Warning{
					Code: WarningReadConfigBeforeHook, Path: path,
					Message: fmt.Sprintf("before_hook %q lists `%s`, which only runs from an after_hook, so it never runs; Terragrunt ignores it as well", hook.Name, command),
				})
			}
		}
	}
	return warnings
}

// validateEngine checks that the executable a unit selected with terraform_binary answers
// `version -json` the way Terraform and OpenTofu do, so a wrong path fails with the unit's name
// instead of as an obscure error from the first command that runs.
func validateEngine(ctx context.Context, request model.Request, path string) error {
	command := exec.CommandContext(ctx, request.Tofu, "version", "-json")
	command.Env = make([]string, 0, len(request.Environment))
	for key, value := range request.Environment {
		command.Env = append(command.Env, key+"="+value)
	}
	data, err := command.Output()
	var version struct {
		Version string `json:"terraform_version"`
	}
	if err == nil {
		err = json.Unmarshal(data, &version)
	}
	if err == nil && version.Version != "" {
		return nil
	}
	return setting(model.CodeEvaluationError, "terraform_binary", path,
		"`terraform_binary` is not a Terraform or OpenTofu executable",
		"The selected executable does not answer `version -json` the way Terraform and OpenTofu do.",
		"Point `terraform_binary` at `tofu`, `terraform`, or a wrapper that forwards to one of them, or remove it to use `components.terraform.command`.")
}

// checkFeatureOverrides rejects an override the unit's feature declarations cannot accept, naming
// the feature. The evaluator reports a bad override as a null feature far from its cause, so the
// declarations are read first. Anything else is left to the real evaluation to report.
func checkFeatureOverrides(path string, overrides map[string]string, declared func() (*config.TerragruntConfig, error)) error {
	if len(overrides) == 0 {
		return nil
	}
	cfg, err := declared()
	if err != nil {
		return nil //nolint:nilerr // The evaluation that follows reports the same error with full context.
	}
	_, err = featureFlagValues(cfg.FeatureFlags, overrides, path)
	return err
}

// Names and limits of the built-in tflint hook. The executable is the hook program that Terragrunt
// replaces with its own tflint integration.
const (
	tflintExecutable   = "tflint"
	tflintExternalFlag = "--terragrunt-external-tflint"
	tflintConfigName   = ".tflint.hcl"
	// tflintMaxParents bounds the search for .tflint.hcl, like Terragrunt's default folder limit.
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
func finalizeExecution(execution *model.Execution, path string, inputs map[string]any) error {
	if execution == nil {
		return nil
	}
	var err error
	if execution.BeforeHooks, err = finalizeHooks(execution.BeforeHooks, execution, path, inputs); err != nil {
		return err
	}
	if execution.AfterHooks, err = finalizeHooks(execution.AfterHooks, execution, path, inputs); err != nil {
		return err
	}
	if execution.ErrorHooks, err = finalizeHooks(execution.ErrorHooks, execution, path, inputs); err != nil {
		return err
	}
	return nil
}

func finalizeHooks(hooks []model.Hook, execution *model.Execution, path string, inputs map[string]any) ([]model.Hook, error) {
	var result []model.Hook
	for i := range hooks {
		for _, hook := range splitLifecycle(hooks[i], filepath.Dir(path)) {
			if len(hook.Execute) > 0 && hook.Execute[0] == tflintExecutable {
				expanded, err := expandTflint(&hook, execution, path, inputs)
				if err != nil {
					return nil, err
				}
				result = append(result, expanded...)
				continue
			}
			result = append(result, hook)
		}
	}
	return result, nil
}

// splitLifecycle gives lifecycle commands the unit directory as working directory. A hook that
// lists both lifecycle and Terraform commands, with no working_dir of its own, becomes two hooks so
// each command keeps its own default directory.
func splitLifecycle(hook model.Hook, unitDir string) []model.Hook {
	var lifecycle, other []string
	for _, command := range hook.Commands {
		if isLifecycleCommand(command) {
			lifecycle = append(lifecycle, command)
		} else {
			other = append(other, command)
		}
	}
	if len(lifecycle) == 0 || hook.WorkingDirectory != "" {
		return []model.Hook{hook}
	}
	first := hook
	first.Commands, first.WorkingDirectory = lifecycle, unitDir
	if len(other) == 0 {
		return []model.Hook{first}
	}
	second := hook
	second.Commands = other
	return []model.Hook{first, second}
}

// expandTflint replaces a hook that executes tflint with the two commands Terragrunt runs for it:
// `tflint --init` and the lint itself. Both run in the module directory against the .tflint.hcl
// that Terragrunt would find, with the unit's inputs and variable files passed as variables. A hook
// that lists several commands expands once per command, so each lint sees only the extra
// arguments of the command that is running.
func expandTflint(hook *model.Hook, execution *model.Execution, path string, inputs map[string]any) ([]model.Hook, error) {
	arguments := slices.DeleteFunc(slices.Clone(hook.Execute), func(arg string) bool { return arg == tflintExternalFlag })
	configFile, err := tflintConfig(arguments, filepath.Dir(path))
	if err != nil {
		return nil, setting(model.CodeUnsupportedHook, fmt.Sprintf("hook %q", hook.Name), path, fmt.Sprintf("tflint hook %q cannot find its configuration", hook.Name),
			"Terragrunt looks for `.tflint.hcl` in the unit directory and each parent directory, and none was found.",
			"Add a `.tflint.hcl` file next to the unit or in a parent directory, or pass `--config <file>` in `execute`.")
	}
	inputVariables, err := tflintVariables(inputs)
	if err != nil {
		return nil, err
	}
	passthrough := withoutConfigFlag(arguments[1:])
	var result []model.Hook
	for _, command := range hook.Commands {
		variables := append(slices.Clone(inputVariables), tflintExtraVariables(command, execution)...)
		initHook := *hook
		initHook.Commands = []string{command}
		initHook.Execute = []string{tflintExecutable, "--init", "--config", configFile, "--chdir", "."}
		runHook := initHook
		runHook.Execute = append(append([]string{tflintExecutable, "--config", configFile, "--chdir", "."}, variables...), passthrough...)
		// The module directory is where tflint runs; a working_dir would only change the lock key upstream.
		initHook.WorkingDirectory, runHook.WorkingDirectory = "", ""
		result = append(result, initHook, runHook)
	}
	return result, nil
}

// tflintConfig returns the configuration named by --config/-c, or the nearest .tflint.hcl.
func tflintConfig(arguments []string, unitDir string) (string, error) {
	for i, arg := range arguments {
		for _, flag := range []string{"--config", "-c"} {
			if arg == flag && len(arguments) > i+1 {
				return anchored(arguments[i+1], unitDir), nil
			}
			if value, found := strings.CutPrefix(arg, flag+"="); found {
				return anchored(value, unitDir), nil
			}
		}
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

// tflintVariables passes every input as a --var, sorted so repeated runs build the same command.
func tflintVariables(inputs map[string]any) ([]string, error) {
	variables := make([]string, 0, len(inputs))
	for _, name := range slices.Sorted(maps.Keys(inputs)) {
		value, err := variableValue(inputs[name])
		if err != nil {
			return nil, err
		}
		variables = append(variables, fmt.Sprintf("--var=%s=%s", name, value))
	}
	return variables, nil
}

// variableValue renders a string as itself and any other value as JSON, escaping interpolation.
func variableValue(value any) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(string(data), "${", "$${"), nil
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
func extraArgumentVariables(extra *model.ExtraArguments) []string {
	var variables []string
	for _, name := range slices.Sorted(maps.Keys(extra.Environment)) {
		if variable, found := strings.CutPrefix(name, "TF_VAR_"); found {
			variables = append(variables, fmt.Sprintf("--var=%s=%s", variable, extra.Environment[name]))
		}
	}
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
		if _, err := os.Stat(file); err == nil {
			variables = append(variables, "--var-file="+file)
		}
	}
	return variables
}

// applyUnitSettings applies the parity settings of a fully evaluated unit and checks the
// executable it selected.
func applyUnitSettings(ctx context.Context, cfg *config.TerragruntConfig, request model.Request, path string, result *model.Resolution) error {
	if err := applyParity(cfg, request, path, result); err != nil {
		return err
	}
	result.Warnings = append(result.Warnings, lifecycleWarnings(cfg.Terraform, path)...)
	if result.TerraformBinary == "" {
		return nil
	}
	return validateEngine(ctx, request, path)
}
