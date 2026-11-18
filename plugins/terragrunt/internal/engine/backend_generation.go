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

// checkBackendGeneration inspects the unit and its parsed includes. Unknown generation
// expressions fail closed without evaluating inputs, hooks or commands.
func checkBackendGeneration(filesystem afero.Fs, paths []string) error {
	for _, path := range paths {
		// Upstream partial parsing does not merge automatic includes. They can add
		// either generation or a different remote_state identity, so do not guess.
		auto := filepath.Join(filepath.Dir(path), "terragrunt.autoinclude.hcl")
		if _, err := filesystem.Stat(auto); err == nil {
			return &model.Problem{
				Code: model.CodeUnsupportedSetting, Summary: "Cannot verify backend identity from an automatic include",
				Detail: "Backend-only parsing does not merge terragrunt.autoinclude.hcl.",
				Hints:  []string{"Use an explicit include for backend configuration so state reads and execution resolve the same identity."},
				File:   auto, Key: "include", Safe: true, Err: ErrUnsupportedExecution,
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
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
	if attr, ok := content.Attributes["generate"]; ok {
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
	if disabled, known := literalGenerateDisabled(content.Attributes["disable"]); !known || disabled {
		return known && disabled
	}
	path, ok := literalGenerateString(content.Attributes["path"])
	path = filepath.Clean(path)
	if !ok || !filepath.IsLocal(path) || path == "." {
		return false
	}
	if !terraformConfigurationFile(path) {
		return true
	}
	text, ok := literalGenerateString(content.Attributes["contents"])
	return ok && !declaresBackend(path, text)
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
// inputs stay unevaluated during backend reads.
func mentionsDownloadDir(filesystem afero.Fs, paths []string) bool {
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
