package engine

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

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

// checkBackendGeneration inspects files read by the partial backend parse, including
// parents. Unknown generation expressions fail closed without evaluating inputs,
// hooks or commands. Non-HCL auxiliary files are not configuration declarations.
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
	if attr, ok := content.Attributes["disable"]; ok {
		value, diags := attr.Expr.Value(nil)
		if diags.HasErrors() || !value.IsKnown() || value.IsNull() || value.Type() != cty.Bool {
			return false
		}
		if value.True() {
			return true
		}
	}
	path, ok := literalGenerateString(content.Attributes["path"])
	if !ok || !filepath.IsLocal(path) || path == "." {
		return false
	}
	lower := strings.ToLower(path)
	if !strings.HasSuffix(lower, ".tf") && !strings.HasSuffix(lower, ".tf.json") {
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
