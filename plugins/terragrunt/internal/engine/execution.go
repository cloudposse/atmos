package engine

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
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
	"github.com/hashicorp/hcl/v2/hclsyntax"
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
func (e *unitEvaluator) applyTimeExecution(planned *model.Execution, parse func(*config.ParsingContext) (*config.TerragruntConfig, error)) (*model.Execution, []string, error) {
	read := e.pctx.FilesRead.Paths()
	if e.request.Command != "plan" || !observesCommand(e.environment.FS, append(slices.Clone(read), e.path)) {
		return planned, nil, nil
	}
	if err := rejectArgumentDependentExecution(e.environment.FS, append(slices.Clone(read), e.path)); err != nil {
		return nil, nil, err
	}
	// Apply is evaluated with the request's own download directory, so the working directory
	// matches the plan-time evaluation. requestParsingContext also carries the feature flags.
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
	// The plan-time execution is already finalized, so both are compared in the same form.
	finalizeExecution(applied, e.path)
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
				return nil, unsupportedHook(fmt.Sprintf("error_hook %q", converted.Name), fmt.Sprintf("Invalid `on_errors` pattern in error_hook %q", converted.Name),
					"An `on_errors` entry is not a valid regular expression.", "Fix the pattern; Atmos matches `on_errors` entries as Go regular expressions.")
			}
		}
		result = append(result, converted)
	}
	return result, nil
}

func validateHook(kind string, hook *model.Hook) error {
	if len(hook.Execute) == 0 || hook.Execute[0] == "" {
		return unsupportedHook(fmt.Sprintf("%s %q", kind, hook.Name), fmt.Sprintf("%s %q has no command to run", kind, hook.Name),
			"The hook sets no `execute` list or its first entry is empty.", "Set `execute = [\"program\", \"argument\"]` on the hook.")
	}
	return nil
}

// checkExecutionSyntax validates declaration shapes and literal hook restrictions without
// a Terragrunt evaluation context. Dynamic values (including dependency outputs and
// run_cmd) are validated by executionFromConfig during preparation.
func checkExecutionSyntax(filesystem afero.Fs, cfg *config.TerragruntConfig, path string) error {
	paths, err := includedExecutionPaths(filesystem, cfg, path)
	if err != nil {
		return err
	}
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

// includedExecutionPaths lists the files whose execution blocks apply to a unit, in merge order.
// Included files come first, in the order the unit declares its includes, then the unit and its
// highest-priority automatic override,
// so a later same-named declaration replaces an earlier one, as Terragrunt merges hooks and
// extra_arguments by name.
func includedExecutionPaths(filesystem afero.Fs, cfg *config.TerragruntConfig, path string) ([]string, error) {
	var paths []string
	for _, name := range includeDeclarationOrder(filesystem, path, cfg.ProcessedIncludes) {
		include := cfg.ProcessedIncludes[name]
		strategy, err := include.GetMergeStrategy()
		if err != nil {
			return nil, err
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
	auto := filepath.Join(filepath.Dir(path), config.DefaultAutoIncludeFile)
	if _, err := filesystem.Stat(auto); err == nil {
		paths = append(paths, auto)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return paths, nil
}

// includeDeclarationOrder returns the names of the processed includes in the order the unit declares
// them. The processed map has no order, and a later include takes precedence over an earlier one.
// An include the unit file does not declare by name keeps a deterministic place after the others.
func includeDeclarationOrder(filesystem afero.Fs, path string, processed config.IncludeConfigsMap) []string {
	var ordered []string
	for _, name := range declaredIncludeNames(filesystem, path) {
		if _, ok := processed[name]; ok && !slices.Contains(ordered, name) {
			ordered = append(ordered, name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(processed)) {
		if !slices.Contains(ordered, name) {
			ordered = append(ordered, name)
		}
	}
	return ordered
}

// declaredIncludeNames reads the `include` blocks of a unit file in declaration order. A bare
// `include` has the empty name. An unreadable or unparseable file yields none, because the
// execution blocks of the same file are parsed and reported afterwards.
func declaredIncludeNames(filesystem afero.Fs, path string) []string {
	data, err := afero.ReadFile(filesystem, path)
	if err != nil {
		return nil
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
		return nil
	}
	var names []string
	if body, ok := file.Body.(*hclsyntax.Body); ok {
		for _, block := range body.Blocks {
			if block.Type == "include" {
				names = append(names, strings.Join(block.Labels, ""))
			}
		}
		return names
	}
	// JSON bodies name every include, so the labelled schema reads them in order.
	content, _, diagnostics := file.Body.PartialContent(&hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{{Type: "include", LabelNames: []string{"name"}}}})
	if diagnostics.HasErrors() {
		return nil
	}
	for _, block := range content.Blocks {
		names = append(names, block.Labels[0])
	}
	return names
}

// executionDeclaration is one execution block and the file that declares it.
type executionDeclaration struct {
	block *hcl.Block
	path  string
}

// effectiveExecutionBlocks parses every file and keeps, for each block type and name, the last
// file in merge order, preserving repeated declarations within that file.
// Unparseable files are still reported.
func effectiveExecutionBlocks(filesystem afero.Fs, paths []string) ([]executionDeclaration, error) {
	var order []string
	effective := map[string][]executionDeclaration{}
	for _, path := range paths {
		blocks, err := executionDeclarations(filesystem, path)
		if err != nil {
			return nil, err
		}
		seenInFile := map[string]bool{}
		for _, block := range blocks {
			key := block.Type + "\x00" + strings.Join(block.Labels, "\x00")
			if _, seen := effective[key]; !seen {
				order = append(order, key)
			}
			if !seenInFile[key] {
				effective[key] = nil
				seenInFile[key] = true
			}
			effective[key] = append(effective[key], executionDeclaration{block: block, path: path})
		}
	}
	result := make([]executionDeclaration, 0, len(order))
	for _, key := range order {
		result = append(result, effective[key]...)
	}
	return result, nil
}

func checkExecutionBlock(block *hcl.Block) error {
	target := executionBlockTarget(block.Type)
	if target == nil {
		return nil
	}
	schema, _ := gohcl.ImpliedBodySchema(target)
	content, diagnostics := block.Body.Content(schema)
	if diagnostics.HasErrors() {
		return diagnostics
	}
	if block.Type == "extra_arguments" || !hookMayRun(content) {
		return nil
	}
	hook := model.Hook{Name: block.Labels[0], Execute: []string{"deferred"}}
	if commands, ok := literalStringList(content, "commands"); ok {
		hook.Commands = commands
	}
	if execute, ok := literalStringList(content, "execute"); ok {
		hook.Execute = execute
	}
	if err := validateHook(block.Type, &hook); err != nil {
		return err
	}
	if patterns, ok := literalStringList(content, "on_errors"); ok {
		_, err := errorHooksFromConfig([]config.ErrorHook{{Name: hook.Name, Commands: hook.Commands, Execute: hook.Execute, OnErrors: patterns}})
		return err
	}
	return nil
}

// executionBlockTarget returns the configuration type that declares a block, or nil for a block that
// holds no execution settings.
func executionBlockTarget(blockType string) any {
	switch blockType {
	case "extra_arguments":
		return &config.TerraformExtraArguments{}
	case "before_hook", "after_hook":
		return &config.Hook{}
	case "error_hook":
		return &config.ErrorHook{}
	}
	return nil
}

// hookMayRun reports false when a literal `if = false` disables the hook. A dynamic condition may
// disable it too, but that is decided only during preparation.
func hookMayRun(content *hcl.BodyContent) bool {
	condition, ok := content.Attributes["if"]
	if !ok {
		return true
	}
	var enabled bool
	diagnostics := gohcl.DecodeExpression(condition.Expr, nil, &enabled)
	return !diagnostics.HasErrors() && enabled
}

// literalStringList decodes an attribute that is a literal list of strings. It reports false when
// the attribute is absent or dynamic, which preparation validates once values are known.
func literalStringList(content *hcl.BodyContent, name string) ([]string, bool) {
	attribute, ok := content.Attributes[name]
	if !ok {
		return nil, false
	}
	var values []string
	if diagnostics := gohcl.DecodeExpression(attribute.Expr, nil, &values); diagnostics.HasErrors() {
		return nil, false
	}
	return values, true
}

// rejectArgumentDependentExecution fails closed until saved apply arguments can be bound.
func rejectArgumentDependentExecution(filesystem afero.Fs, paths []string) error {
	for _, path := range paths {
		content, err := afero.ReadFile(filesystem, path)
		if err != nil || bytes.Contains(content, []byte("get_terraform_cli_args")) {
			return fmt.Errorf("%w: argument-dependent saved-plan execution cannot be bound before apply", ErrUnsupportedExecution)
		}
	}
	return nil
}

func executionDeclarations(filesystem afero.Fs, path string) ([]*hcl.Block, error) {
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
	var blocks []*hcl.Block
	schema, _ := gohcl.ImpliedBodySchema(&config.TerraformConfig{})
	for _, terraform := range content.Blocks {
		declarations, diagnostics := terraform.Body.Content(schema)
		if diagnostics.HasErrors() {
			return nil, diagnostics
		}
		blocks = append(blocks, declarations.Blocks...)
	}
	return blocks, nil
}
