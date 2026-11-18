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
		File:   path, Key: "generate", Safe: true, Err: ErrUnsupportedExecution,
	}
}

// backendConfigurationPaths names declarations participating in this unit's parse.
// FilesRead also contains data reads and mark_as_read paths, which are watches,
// not configuration declarations (and need not exist).
func backendConfigurationPaths(cfg *config.TerragruntConfig, path string) []string {
	paths := []string{path}
	for _, name := range slices.Sorted(maps.Keys(cfg.ProcessedIncludes)) {
		paths = append(paths, localPath(filepath.Dir(path), cfg.ProcessedIncludes[name].Path))
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
	return nonBackendGeneration(disabled, known, path, pathOK, text, textOK)
}

// staticallyNonBackendGenerateMap applies the checks of a named generate block to each entry of a
// literal `generate = { name = { path = ..., contents = ... } }` map. A value that is not a literal
// map of literal entries, or an entry whose target or contents cannot be determined, is rejected.
func staticallyNonBackendGenerateMap(attr *hcl.Attribute) bool {
	value, diagnostics := attr.Expr.Value(nil)
	if diagnostics.HasErrors() || !value.IsWhollyKnown() || value.IsNull() {
		return false
	}
	if !value.Type().IsObjectType() && !value.Type().IsMapType() {
		return false
	}
	for iterator := value.ElementIterator(); iterator.Next(); {
		_, entry := iterator.Element()
		if !staticallyNonBackendGenerateEntry(entry) {
			return false
		}
	}
	return true
}

func staticallyNonBackendGenerateEntry(entry cty.Value) bool {
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
	path = filepath.Clean(path)
	if !pathOK || !filepath.IsLocal(path) || path == "." {
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
