package engine

import (
	"errors"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/spf13/afero"
	"github.com/zclconf/go-cty/cty"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

func unresolvedGeneratedBackend(path string) error {
	return &model.Problem{
		Code: model.CodeUnsupportedSetting, Summary: "Cannot verify the backend identity of a `generate` block",
		Detail: "State readers cannot use an unverified generated backend or cloud configuration.",
		Hints:  []string{"Declare the backend in `remote_state` and use `remote_state.generate` to generate it. For state reads, other generate blocks must have literal paths and contents that can be verified without evaluating inputs or commands."},
		File:   path, Key: "generate", Safe: true, Err: errors.Join(ErrUnsupportedExecution, errUnverifiedGeneration),
	}
}

// backendConfigurationPaths names declarations participating in this unit's parse.
// FilesRead also contains data reads and mark_as_read paths, which are watches,
// not configuration declarations (and need not exist).
func backendConfigurationPaths(cfg *config.TerragruntConfig, path string) []string {
	paths := []string{path}
	for _, name := range slices.Sorted(maps.Keys(cfg.ProcessedIncludes)) {
		include := cfg.ProcessedIncludes[name]
		// Terragrunt records a no_merge include but never merges its generate or remote_state
		// settings, so a backend declared there is never generated for this unit. An invalid
		// strategy returns an error and stays in the scan, which keeps it fail-closed.
		if strategy, err := include.GetMergeStrategy(); err == nil && strategy == config.NoMerge {
			continue
		}
		paths = append(paths, localPath(filepath.Dir(path), include.Path))
	}
	return paths
}

// automaticIncludeName is the sibling file Terragrunt registers and merges on top of a unit during
// partial parsing, so its declarations take part in backend identity like an explicit include.
const automaticIncludeName = "terragrunt.autoinclude.hcl"

// withAutomaticIncludes adds the sibling automatic include of each parsed configuration that has
// one. Its presence alone proves nothing about the backend: the partial parse already merged it,
// so what matters is whether it generates files that could declare one.
func withAutomaticIncludes(filesystem afero.Fs, paths []string) ([]string, error) {
	expanded := slices.Clone(paths)
	for _, path := range paths {
		if filepath.Base(path) == automaticIncludeName {
			continue
		}
		auto := filepath.Join(filepath.Dir(path), automaticIncludeName)
		if _, err := filesystem.Stat(auto); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		if !slices.Contains(expanded, auto) {
			expanded = append(expanded, auto)
		}
	}
	return expanded, nil
}

// checkBackendGeneration inspects the unit, its parsed includes, and the automatic includes that
// merge into them. Unknown generation expressions fail closed without evaluating inputs, hooks or commands.
func checkBackendGeneration(filesystem afero.Fs, paths []string) error {
	paths, err := withAutomaticIncludes(filesystem, paths)
	if err != nil {
		return err
	}
	for _, path := range paths {
		content, err := afero.ReadFile(filesystem, path)
		if err != nil {
			return err
		}
		parser := hclparse.NewParser()
		var file *hcl.File
		var diagnostics hcl.Diagnostics
		if strings.HasSuffix(path, ".json") {
			file, diagnostics = parser.ParseJSON(content, path)
		} else {
			file, diagnostics = parser.ParseHCL(content, path)
		}
		// The upstream parser already validates configuration files.
		if diagnostics.HasErrors() {
			continue
		}
		if err := checkBackendGenerationFile(file); err != nil {
			return err
		}
	}
	return nil
}

func checkBackendGenerationFile(file *hcl.File) error {
	content, _, diagnostics := file.Body.PartialContent(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{{Name: "generate"}},
		Blocks:     []hcl.BlockHeaderSchema{{Type: "generate", LabelNames: []string{"name"}}},
	})
	if diagnostics.HasErrors() {
		return diagnostics
	}
	if attr, ok := content.Attributes["generate"]; ok && !staticallyNonBackendGenerateMap(attr) {
		return unresolvedGeneratedBackend(attr.Range.Filename)
	}
	for _, block := range content.Blocks {
		if !staticallyNonBackendGeneration(block.Body) {
			return unresolvedGeneratedBackend(block.DefRange.Filename)
		}
	}
	return nil
}

func staticallyNonBackendGeneration(body hcl.Body) bool {
	content, _, diagnostics := body.PartialContent(&hcl.BodySchema{Attributes: []hcl.AttributeSchema{
		{Name: "disable"}, {Name: "path"}, {Name: "contents"},
	}})
	if diagnostics.HasErrors() {
		return false
	}
	disabled, known := literalGenerateDisabled(content.Attributes["disable"])
	path, pathOK := literalGenerateString(content.Attributes["path"])
	text, textOK := literalGenerateString(content.Attributes["contents"])
	if attr := content.Attributes["contents"]; attr != nil {
		textOK = textOK && !unevaluatedJSONTemplate(attr.Range.Filename, text)
	}
	return nonBackendGeneration(disabled, known, path, pathOK, text, textOK)
}

// unevaluatedJSONTemplate reports contents read from a JSON-syntax file that still hold a template
// sequence. JSON strings are read without an evaluation context, so ${...} and %{...} reach the
// check literally and could generate a backend once evaluated. Escaped $${ and %%{ are static text.
func unevaluatedJSONTemplate(filename, text string) bool {
	return strings.HasSuffix(filename, ".json") && unevaluatedTemplate(text)
}

// unevaluatedTemplate reports text that still holds an unescaped ${...} or %{...} sequence.
func unevaluatedTemplate(text string) bool {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "$${", ""), "%%{", "")
	return strings.Contains(text, "${") || strings.Contains(text, "%{")
}

// staticallyNonBackendGenerateMap applies the checks of a named generate block to each entry of a
// literal `generate = { name = { path = ..., contents = ... } }` map. A value that is not a literal
// map of literal entries, or an entry whose target or contents cannot be determined, is rejected.
func staticallyNonBackendGenerateMap(attr *hcl.Attribute) bool {
	jsonSyntax := strings.HasSuffix(attr.Range.Filename, ".json")
	value, diagnostics := attr.Expr.Value(nil)
	if diagnostics.HasErrors() || !value.IsWhollyKnown() || value.IsNull() {
		return false
	}
	if !value.Type().IsObjectType() && !value.Type().IsMapType() {
		return false
	}
	for iterator := value.ElementIterator(); iterator.Next(); {
		_, entry := iterator.Element()
		if !staticallyNonBackendGenerateEntry(entry, jsonSyntax) {
			return false
		}
	}
	return true
}

func staticallyNonBackendGenerateEntry(entry cty.Value, jsonSyntax bool) bool {
	if entry.IsNull() || (!entry.Type().IsObjectType() && !entry.Type().IsMapType()) {
		return false
	}
	disabled, known := false, true
	if field, ok := generateField(entry, "disable"); ok {
		known = field.Type() == cty.Bool && !field.IsNull()
		if known {
			disabled = field.True()
		}
	}
	path, pathOK := generateFieldString(entry, "path")
	text, textOK := generateFieldString(entry, "contents")
	if jsonSyntax && textOK {
		textOK = !unevaluatedTemplate(text)
	}
	return nonBackendGeneration(disabled, known, path, pathOK, text, textOK)
}

func generateField(entry cty.Value, name string) (cty.Value, bool) {
	switch {
	case entry.Type().IsObjectType() && entry.Type().HasAttribute(name):
		return entry.GetAttr(name), true
	case entry.Type().IsMapType() && entry.HasIndex(cty.StringVal(name)).True():
		return entry.Index(cty.StringVal(name)), true
	}
	return cty.NilVal, false
}

func generateFieldString(entry cty.Value, name string) (string, bool) {
	field, ok := generateField(entry, name)
	if !ok || field.IsNull() || field.Type() != cty.String {
		return "", false
	}
	return field.AsString(), true
}

// nonBackendGeneration decides from literal values whether one generate entry can declare a backend.
func nonBackendGeneration(disabled, disabledKnown bool, path string, pathOK bool, text string, textOK bool) bool {
	if !disabledKnown || disabled {
		return disabledKnown && disabled
	}
	// JSON syntax read without an evaluation context returns a template literally, such as
	// ${"backend.tf"}. It is dynamic and could name a backend file once evaluated, so it fails closed.
	if !pathOK || strings.Contains(path, "${") || strings.Contains(path, "%{") {
		return false
	}
	path = filepath.Clean(path)
	if !filepath.IsLocal(path) || path == "." {
		return false
	}
	if !terraformConfigurationFile(path) {
		return true
	}
	return textOK && !declaresBackend(path, text)
}

func literalGenerateString(attr *hcl.Attribute) (string, bool) {
	if attr == nil {
		return "", false
	}
	value, diagnostics := attr.Expr.Value(nil)
	if diagnostics.HasErrors() || !value.IsKnown() || value.IsNull() || value.Type() != cty.String {
		return "", false
	}
	return value.AsString(), true
}

func literalGenerateDisabled(attr *hcl.Attribute) (bool, bool) {
	if attr == nil {
		return false, true
	}
	value, diags := attr.Expr.Value(nil)
	if diags.HasErrors() || !value.IsKnown() || value.IsNull() || value.Type() != cty.Bool {
		return false, false
	}
	return value.True(), true
}

// terraformConfigurationFile identifies generated files Terraform can load.
func terraformConfigurationFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".tf") || strings.HasSuffix(lower, ".tf.json") ||
		strings.HasSuffix(lower, ".tofu") || strings.HasSuffix(lower, ".tofu.json")
}

// mentionsDownloadDir reports whether any parsed configuration file names download_dir. The probe
// in downloadDir parses the whole unit, so a unit that never sets it skips the probe and its
// inputs stay unevaluated during backend reads. Automatic includes merge into the unit, so they
// are scanned too; one that cannot be inspected is skipped, which at worst skips an optional probe.
func mentionsDownloadDir(filesystem afero.Fs, paths []string) bool {
	if expanded, err := withAutomaticIncludes(filesystem, paths); err == nil {
		paths = expanded
	}
	for _, path := range paths {
		data, err := afero.ReadFile(filesystem, path)
		if err != nil {
			continue
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
			continue
		}
		content, _, _ := file.Body.PartialContent(&hcl.BodySchema{Attributes: []hcl.AttributeSchema{{Name: "download_dir"}}})
		if content.Attributes["download_dir"] != nil {
			return true
		}
	}
	return false
}

// checkLiteralGeneratedBackends rejects, during discovery, a generate block whose path and contents
// are literal and declare a backend. Full preparation rejects such a unit anyway; rejecting it here
// records the failure before bulk preflight lets a healthy prerequisite run. Declarations that need
// evaluation are left alone: discovery does not evaluate inputs, commands, or functions.
func checkLiteralGeneratedBackends(filesystem afero.Fs, paths []string) error {
	paths, err := withAutomaticIncludes(filesystem, paths)
	if err != nil {
		return err
	}
	for _, path := range paths {
		content, err := afero.ReadFile(filesystem, path)
		if err != nil {
			return err
		}
		parser := hclparse.NewParser()
		var file *hcl.File
		var diagnostics hcl.Diagnostics
		if strings.HasSuffix(path, ".json") {
			file, diagnostics = parser.ParseJSON(content, path)
		} else {
			file, diagnostics = parser.ParseHCL(content, path)
		}
		if diagnostics.HasErrors() {
			continue
		}
		if literalBackendInFile(file) {
			return unresolvedGeneratedBackend(path)
		}
	}
	return nil
}

// literalBackendInFile reports a named generate block, or an entry of a literal generate map, that
// certainly writes a backend.
func literalBackendInFile(file *hcl.File) bool {
	content, _, diagnostics := file.Body.PartialContent(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{{Name: "generate"}},
		Blocks:     []hcl.BlockHeaderSchema{{Type: "generate", LabelNames: []string{"name"}}},
	})
	if diagnostics.HasErrors() {
		return false
	}
	for _, block := range content.Blocks {
		fields, _, diagnostics := block.Body.PartialContent(&hcl.BodySchema{Attributes: []hcl.AttributeSchema{
			{Name: "disable"}, {Name: "path"}, {Name: "contents"},
		}})
		if diagnostics.HasErrors() {
			continue
		}
		disabled, known := literalGenerateDisabled(fields.Attributes["disable"])
		path, pathOK := literalGenerateString(fields.Attributes["path"])
		text, textOK := literalGenerateString(fields.Attributes["contents"])
		if literalGeneratedBackend(disabled, known, path, pathOK, text, textOK) {
			return true
		}
	}
	attr, ok := content.Attributes["generate"]
	if !ok {
		return false
	}
	return literalBackendInGenerateMap(attr.Expr)
}

// literalBackendInGenerateMap checks each entry of a generate map independently, so one dynamic
// entry (such as a notes.txt using get_env) is skipped without hiding a literal backend entry.
func literalBackendInGenerateMap(expr hcl.Expression) bool {
	if pairs, diagnostics := hcl.ExprMap(expr); !diagnostics.HasErrors() {
		return literalBackendInPairs(pairs)
	}
	return literalBackendInMapValue(expr)
}

// literalBackendInPairs evaluates each generate map entry alone and skips the dynamic ones.
func literalBackendInPairs(pairs []hcl.KeyValuePair) bool {
	for _, pair := range pairs {
		value, diagnostics := pair.Value.Value(nil)
		if !diagnostics.HasErrors() && value.IsWhollyKnown() && literalBackendInEntry(value) {
			return true
		}
	}
	return false
}

// literalBackendInMapValue evaluates a generate map that is not a literal map expression as a whole.
func literalBackendInMapValue(expr hcl.Expression) bool {
	value, diagnostics := expr.Value(nil)
	if diagnostics.HasErrors() || !value.IsWhollyKnown() || value.IsNull() || (!value.Type().IsObjectType() && !value.Type().IsMapType()) {
		return false
	}
	for iterator := value.ElementIterator(); iterator.Next(); {
		_, entry := iterator.Element()
		if literalBackendInEntry(entry) {
			return true
		}
	}
	return false
}

func literalBackendInEntry(entry cty.Value) bool {
	if entry.IsNull() || (!entry.Type().IsObjectType() && !entry.Type().IsMapType()) {
		return false
	}
	disabled, known := false, true
	if field, ok := generateField(entry, "disable"); ok {
		known = field.Type() == cty.Bool && !field.IsNull()
		disabled = known && field.True()
	}
	path, pathOK := generateFieldString(entry, "path")
	text, textOK := generateFieldString(entry, "contents")
	return literalGeneratedBackend(disabled, known, path, pathOK, text, textOK)
}

// literalGeneratedBackend reports an enabled declaration whose literal path names a Terraform file
// and whose literal contents declare a backend. Anything dynamic is not certain, so it is false.
func literalGeneratedBackend(disabled, disabledKnown bool, path string, pathOK bool, text string, textOK bool) bool {
	if !disabledKnown || disabled || !pathOK || !textOK {
		return false
	}
	for _, value := range []string{path, text} {
		if strings.Contains(value, "${") || strings.Contains(value, "%{") {
			return false
		}
	}
	path = filepath.Clean(path)
	return filepath.IsLocal(path) && terraformConfigurationFile(path) && declaresBackend(path, text)
}
