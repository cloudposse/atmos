package engine

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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
func applyParity(cfg *config.TerragruntConfig, request *model.Request, path string, result *model.Resolution) error {
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
		if err := validateRetryBlock(retry, path); err != nil {
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
	return evaluationSetting(key, file, summary, detail, hint)
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
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&plain); err != nil {
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
//
//nolint:lintroller // Standalone SDK-only plugin has no host perf runtime dependency.
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
//
//nolint:lintroller // Standalone SDK-only plugin has no host perf runtime dependency.
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
//
//nolint:lintroller // Standalone SDK-only plugin has no host perf runtime dependency.
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
					return nil, evaluationSetting("feature "+flag.Name, path,
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
//
//nolint:lintroller // Standalone SDK-only plugin has no host perf runtime dependency.
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
			return nil, evaluationSetting("TG_FEATURE", "",
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
func validateEngine(ctx context.Context, request *model.Request, binary, path string) error {
	var version struct {
		Version string `json:"terraform_version"`
	}
	executable, err := resolveOnInvocationPath(binary, request.Environment)
	if err == nil {
		command := exec.CommandContext(ctx, executable, "version", "-json")
		command.Env = make([]string, 0, len(request.Environment))
		for key, value := range request.Environment {
			command.Env = append(command.Env, key+"="+value)
		}
		var data []byte
		if data, err = command.Output(); err == nil {
			err = json.Unmarshal(data, &version)
		}
	}
	if err == nil && version.Version != "" {
		return nil
	}
	return evaluationSetting("terraform_binary", path,
		"`terraform_binary` is not a Terraform or OpenTofu executable",
		"The selected executable does not answer `version -json` the way Terraform and OpenTofu do.",
		"Point `terraform_binary` at `tofu`, `terraform`, or a wrapper that forwards to one of them, or remove it to use `components.terraform.command`.")
}

// resolveOnInvocationPath resolves a bare executable name against the PATH of the invocation, which
// is not the PATH of this process. A name with a path separator, and any name when the invocation
// has no PATH, is returned as it is. On Windows the extensions of the invocation's PATHEXT apply.
func resolveOnInvocationPath(binary string, environment map[string]string) (string, error) {
	search := environmentValue(environment, "PATH")
	if search == "" || filepath.IsAbs(binary) || strings.ContainsAny(binary, `/\`) {
		return binary, nil
	}
	extensions := []string{""}
	if runtime.GOOS == "windows" {
		pathext := cmp.Or(environmentValue(environment, "PATHEXT"), ".COM;.EXE;.BAT;.CMD")
		extensions = strings.Split(strings.ToLower(pathext), ";")
		if slices.Contains(extensions, strings.ToLower(filepath.Ext(binary))) {
			extensions = []string{""}
		}
	}
	for _, directory := range filepath.SplitList(search) {
		for _, extension := range extensions {
			candidate := filepath.Join(cmp.Or(directory, "."), binary+extension)
			if isInvocationExecutable(candidate) {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("%w: %q is not on the invocation PATH", exec.ErrNotFound, binary)
}

// environmentValue reads a variable of the invocation, ignoring case on Windows where names have none.
func environmentValue(environment map[string]string, name string) string {
	if value, found := environment[name]; found || runtime.GOOS != "windows" {
		return value
	}
	for key, value := range environment {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
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

// applyUnitSettings applies the parity settings of a fully evaluated unit and checks the
// executable it selected.
func applyUnitSettings(ctx context.Context, cfg *config.TerragruntConfig, request *model.Request, path string, result *model.Resolution) error {
	if err := applyParity(cfg, request, path, result); err != nil {
		return err
	}
	result.Warnings = append(result.Warnings, lifecycleWarnings(cfg.Terraform, path)...)
	if result.TerraformBinary == "" {
		return nil
	}
	if warning, overridden := binaryOverrideWarning(request, result.TerraformBinary, path); overridden {
		result.Warnings = append(result.Warnings, warning)
		return nil
	}
	return validateEngine(ctx, request, result.TerraformBinary, path)
}

// executablePermissionBits selects any executable permission on POSIX.
const executablePermissionBits = 0o111

func isInvocationExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && (runtime.GOOS == "windows" || info.Mode()&executablePermissionBits != 0)
}

func validateRetryBlock(retry *config.RetryBlock, path string) error {
	key := fmt.Sprintf("errors.retry %q", retry.Label)
	if retry.MaxAttempts < 1 {
		return errorsProblem(key, path, fmt.Sprintf("`max_attempts` is %d in %s", retry.MaxAttempts, key), "A retry policy must allow at least one attempt.", "Set `max_attempts` to 1 or more.")
	}
	if retry.SleepIntervalSec < 0 {
		return errorsProblem(key, path, fmt.Sprintf("`sleep_interval_sec` is %d in %s", retry.SleepIntervalSec, key), "The sleep between attempts cannot be negative.", "Set `sleep_interval_sec` to 0 or more.")
	}
	if int64(retry.MaxAttempts) > math.MaxUint32 || int64(retry.SleepIntervalSec) > math.MaxUint32 {
		return errorsProblem(key, path, fmt.Sprintf("A value is too large in %s", key), "`max_attempts` and `sleep_interval_sec` must fit in 32 bits.", fmt.Sprintf("Use values no larger than %d.", uint32(math.MaxUint32)))
	}
	if err := checkPatterns(key, path, retry.RetryableErrors); err != nil {
		return err
	}
	return nil
}

// resolveIAM resolves the role before the full parse evaluates AWS functions. The host
// assumes it and supplies the resulting session on the second call.
func (r *inventoryReader) resolveIAM(pctx *config.ParsingContext, request *model.Request, path string) (*model.Resolution, error) {
	flags, err := config.PartialParseConfigFile(r.ctx, r.logger, r.environment,
		pctx.WithDecodeList(config.TerragruntFlags, config.FeatureFlagsBlock, config.ExcludeBlock), path, nil)
	if err != nil {
		return nil, err
	}
	if PreventsRun(excludeFromConfig(flags.Exclude), OperationCommand(request.Command, request.Arguments)) {
		return &model.Resolution{Path: path}, nil
	}
	iam, warnings, err := iamSettings(flags, request, path)
	if err != nil {
		return nil, err
	}
	return &model.Resolution{Path: path, IAM: iam, Warnings: warnings}, nil
}

// checkRequestFeatureOverrides rejects feature overrides the unit does not declare. Backend
// reads do not evaluate feature flags.
func (r *inventoryReader) checkRequestFeatureOverrides(pctx *config.ParsingContext, request *model.Request, path string) error {
	if request.Phase == "backend" {
		return nil
	}
	return checkFeatureOverrides(path, request.Features, func() (*config.TerragruntConfig, error) {
		// Read the declarations without the overrides, which are what is being checked.
		declarations := pctx.WithDecodeList(config.FeatureFlagsBlock)
		declarations.FeatureFlags = nil
		return config.PartialParseConfigFile(r.ctx, r.logger, r.environment, declarations, path, nil)
	})
}

// unitSettings applies the parity and IAM settings of a fully evaluated unit. Backend reads
// evaluate only the backend identity, so they skip them.
func unitSettings(ctx context.Context, cfg *config.TerragruntConfig, request *model.Request, path string, result *model.Resolution) error {
	if request.Phase == "backend" {
		return nil
	}
	if err := applyUnitSettings(ctx, cfg, request, path, result); err != nil {
		return err
	}
	iam, warnings, err := iamSettings(cfg, request, path)
	if err != nil {
		return err
	}
	result.IAM, result.Warnings = iam, append(result.Warnings, warnings...)
	return nil
}
