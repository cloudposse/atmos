package engine

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/spf13/afero"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// ErrUnsupportedExecution identifies Terragrunt hook or extra_arguments semantics that the
// host-owned execution contract does not implement. Messages name only hook identifiers and
// command names, never the argv or evaluated values, so they are safe to show to the user.
var ErrUnsupportedExecution = errors.New("unsupported native Terragrunt execution configuration")

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
func applyTimeExecution(request model.Request, path string, planned *model.Execution, filesystem afero.Fs, read []string,
	parse func(*config.ParsingContext) (*config.TerragruntConfig, error),
) (*model.Execution, []string, error) {
	if request.Command != "plan" || !observesCommand(filesystem, append(slices.Clone(read), path)) {
		return planned, nil, nil
	}
	apply := parsingContext(path, request.Tofu, "apply", nil)
	apply.FeatureFlags = request.Features
	cfg, err := parse(apply)
	if err != nil {
		return nil, nil, err
	}
	// Configuration read only when evaluating for apply also shapes the reviewed plan.
	files := apply.FilesRead.Paths()
	applied, err := executionFromConfig(cfg.Terraform)
	if err != nil {
		return nil, nil, err
	}
	// The plan-time execution is already finalized, so both are compared in the same form.
	finalizeExecution(applied, path)
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
			if bytes.Contains(contents, name) {
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
				return nil, setting(model.CodeUnsupportedHook, fmt.Sprintf("error_hook %q", converted.Name), "", fmt.Sprintf("Invalid `on_errors` pattern in error_hook %q", converted.Name),
					"An `on_errors` entry is not a valid regular expression.", "Fix the pattern; Atmos matches `on_errors` entries as Go regular expressions.")
			}
		}
		result = append(result, converted)
	}
	return result, nil
}

func validateHook(kind string, hook *model.Hook) error {
	if len(hook.Execute) == 0 || hook.Execute[0] == "" {
		return setting(model.CodeUnsupportedHook, fmt.Sprintf("%s %q", kind, hook.Name), "", fmt.Sprintf("%s %q has no command to run", kind, hook.Name),
			"The hook sets no `execute` list or its first entry is empty.", "Set `execute = [\"program\", \"argument\"]` on the hook.")
	}
	return nil
}

// checkExecutionSyntax validates declaration shapes and literal hook restrictions without
// a Terragrunt evaluation context. Dynamic values (including dependency outputs and
// run_cmd) are validated by executionFromConfig during preparation.
func checkExecutionSyntax(filesystem afero.Fs, cfg *config.TerragruntConfig, path string) error {
	// Included files come first and the unit last, so a later same-named declaration replaces an
	// earlier one, as Terragrunt merges hooks and extra_arguments by name.
	var paths []string
	for _, name := range slices.Sorted(maps.Keys(cfg.ProcessedIncludes)) {
		include := cfg.ProcessedIncludes[name]
		strategy, err := include.GetMergeStrategy()
		if err != nil {
			return err
		}
		if strategy == config.NoMerge {
			continue
		}
		included := include.Path
		if !filepath.IsAbs(included) {
			included = filepath.Join(filepath.Dir(path), included)
		}
		paths = append(paths, included)
	}
	paths = append(paths, path)
	effective, err := effectiveExecutionBlocks(filesystem, paths)
	if err != nil {
		return err
	}
	// A declaration that a later file replaces never runs, so only the effective ones are checked.
	for _, declaration := range effective {
		if err := checkExecutionBlock(declaration.block); err != nil {
			return hookProblem(err, declaration.path)
		}
	}
	return nil
}

// executionDeclaration is one execution block and the file that declares it.
type executionDeclaration struct {
	block *hcl.Block
	path  string
}

// effectiveExecutionBlocks parses every file and keeps, for each block type and name, the last
// declaration in file order. Unparseable files are still reported.
func effectiveExecutionBlocks(filesystem afero.Fs, paths []string) ([]executionDeclaration, error) {
	var order []string
	effective := map[string]executionDeclaration{}
	for _, path := range paths {
		data, err := afero.ReadFile(filesystem, path)
		if err != nil {
			return nil, err
		}
		parser := hclparse.NewParser()
		var file *hcl.File
		var diagnostics hcl.Diagnostics
		if strings.HasSuffix(path, ".json") {
			file, diagnostics = parser.ParseJSON(data, path)
		} else {
			file, diagnostics = parser.ParseHCL(data, path)
		}
		if diagnostics.HasErrors() {
			return nil, diagnostics
		}
		content, _, diagnostics := file.Body.PartialContent(&hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{{Type: "terraform"}}})
		if diagnostics.HasErrors() {
			return nil, diagnostics
		}
		for _, terraform := range content.Blocks {
			schema, _ := gohcl.ImpliedBodySchema(&config.TerraformConfig{})
			declarations, diagnostics := terraform.Body.Content(schema)
			if diagnostics.HasErrors() {
				return nil, diagnostics
			}
			for _, block := range declarations.Blocks {
				key := block.Type + "\x00" + strings.Join(block.Labels, "\x00")
				if _, seen := effective[key]; !seen {
					order = append(order, key)
				}
				effective[key] = executionDeclaration{block: block, path: path}
			}
		}
	}
	result := make([]executionDeclaration, 0, len(order))
	for _, key := range order {
		result = append(result, effective[key])
	}
	return result, nil
}

func checkExecutionBlock(block *hcl.Block) error {
	var target any
	switch block.Type {
	case "extra_arguments":
		target = &config.TerraformExtraArguments{}
	case "before_hook", "after_hook":
		target = &config.Hook{}
	case "error_hook":
		target = &config.ErrorHook{}
	default:
		return nil
	}
	schema, _ := gohcl.ImpliedBodySchema(target)
	content, diagnostics := block.Body.Content(schema)
	if diagnostics.HasErrors() {
		return diagnostics
	}
	if block.Type == "extra_arguments" {
		return nil
	}
	// A dynamic condition may disable the hook. Decide only during preparation.
	if condition, ok := content.Attributes["if"]; ok {
		var enabled bool
		if diagnostics := gohcl.DecodeExpression(condition.Expr, nil, &enabled); diagnostics.HasErrors() || !enabled {
			return nil
		}
	}
	hook := model.Hook{Name: block.Labels[0], Execute: []string{"deferred"}}
	if attribute, ok := content.Attributes["commands"]; ok {
		var commands []string
		if diagnostics := gohcl.DecodeExpression(attribute.Expr, nil, &commands); !diagnostics.HasErrors() {
			hook.Commands = commands
		}
	}
	if attribute, ok := content.Attributes["execute"]; ok {
		var execute []string
		if diagnostics := gohcl.DecodeExpression(attribute.Expr, nil, &execute); !diagnostics.HasErrors() {
			hook.Execute = execute
		}
	}
	if err := validateHook(block.Type, &hook); err != nil {
		return err
	}
	if attribute, ok := content.Attributes["on_errors"]; ok {
		var patterns []string
		if diagnostics := gohcl.DecodeExpression(attribute.Expr, nil, &patterns); !diagnostics.HasErrors() {
			_, err := errorHooksFromConfig([]config.ErrorHook{{Name: hook.Name, Commands: hook.Commands, Execute: hook.Execute, OnErrors: patterns}})
			return err
		}
	}
	return nil
}
