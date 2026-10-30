package engine

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/spf13/afero"
	"github.com/zclconf/go-cty/cty"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// trackInputSources inspects syntax, not values. Partial discovery intentionally
// leaves inputs unevaluated; syntax watches include conditional and optional reads
// without reading state, decrypting secrets, or invoking user commands.
func trackInputSources(fs afero.Fs, root string, result *model.Resolution) error {
	for _, path := range slices.Clone(result.Files) {
		if !strings.HasSuffix(path, ".hcl") {
			continue
		}
		contents, err := afero.ReadFile(fs, path)
		if err != nil {
			return fmt.Errorf("track configuration sources %s: %w", path, err)
		}
		file, diagnostics := hclsyntax.ParseConfig(contents, path, hcl.InitialPos)
		if diagnostics.HasErrors() {
			return fmt.Errorf("cannot track configuration sources in %s", path)
		}
		inputs := file.Body.(*hclsyntax.Body).Attributes["inputs"]
		if inputs == nil {
			continue
		}
		walk := hclsyntax.VisitAll(inputs.Expr, func(node hclsyntax.Node) hcl.Diagnostics {
			if call, ok := node.(*hclsyntax.FunctionCallExpr); ok {
				trackSourceCall(root, filepath.Dir(path), call, result)
			}
			return nil
		})
		if walk.HasErrors() {
			return fmt.Errorf("cannot track configuration sources in %s", path)
		}
	}
	result.Files = slices.Compact(slices.Sorted(slices.Values(result.Files)))
	result.Folders = slices.Compact(slices.Sorted(slices.Values(result.Folders)))
	return nil
}

func trackSourceCall(root, directory string, call *hclsyntax.FunctionCallExpr, result *model.Resolution) {
	switch call.Name {
	case "file", "fileexists", "filebase64", "filemd5", "filesha1", "filesha256", "filesha512",
		"filebase64sha256", "filebase64sha512", "templatefile", "read_tfvars_file", "read_terragrunt_config",
		"sops_decrypt_file", "mark_as_read", "fileset", "find_in_parent_folders":
	default:
		return
	}
	// Unknown paths can refer anywhere in the project. A root directory watch is
	// deliberately conservative and catches new files as well as changed files.
	if len(call.Args) == 0 || call.Name == "find_in_parent_folders" {
		result.Folders = append(result.Folders, root)
		return
	}
	value, diagnostics := call.Args[0].Value(nil)
	if diagnostics.HasErrors() || !value.IsKnown() || value.IsNull() || value.Type() != cty.String {
		result.Folders = append(result.Folders, root)
		return
	}
	path := localPath(directory, value.AsString())
	if call.Name == "fileset" {
		result.Folders = append(result.Folders, path)
	} else {
		result.Files = append(result.Files, path)
	}
}
