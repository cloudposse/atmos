package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/spf13/afero"
	"github.com/zclconf/go-cty/cty"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

var errTrackSources = errors.New("cannot track configuration sources in")

// trackInputSources inspects syntax, not values. Partial discovery intentionally
// leaves inputs unevaluated; syntax watches include conditional and optional reads
// without reading state, decrypting secrets, or invoking user commands.
//
// Configuration that inputs read (read_terragrunt_config) is scanned the same way, transitively,
// so a change to its own inputs' sources affects the unit. Each file is scanned once.
func trackInputSources(fs afero.Fs, root string, result *model.Resolution) error {
	scanned := map[string]bool{}
	for i := 0; i < len(result.Files); i++ {
		path := result.Files[i]
		if scanned[path] || !strings.HasSuffix(path, ".hcl") {
			continue
		}
		scanned[path] = true
		contents, err := afero.ReadFile(fs, path)
		if err != nil {
			// mark_as_read records paths without opening them, including absent files.
			// Keep absent watches, but never hide permission or other I/O failures.
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("track configuration sources %s: %w", path, err)
		}
		if err := scanInputSources(fs, root, path, contents, result); err != nil {
			return err
		}
	}
	result.Files = slices.Compact(slices.Sorted(slices.Values(result.Files)))
	result.Folders = slices.Compact(slices.Sorted(slices.Values(result.Folders)))
	return nil
}

// scanInputSources adds syntax-derived watches while retaining non-HCL data files.
func scanInputSources(fs afero.Fs, root, path string, contents []byte, result *model.Resolution) error {
	file, diagnostics := hclsyntax.ParseConfig(contents, path, hcl.InitialPos)
	if diagnostics.HasErrors() {
		// Files also lists data files that functions read, and a data file may be named
		// *.hcl without being HCL. Every configuration file already parsed during
		// evaluation, so one that does not parse is data: it stays a file watch.
		return nil
	}
	inputs := file.Body.(*hclsyntax.Body).Attributes["inputs"]
	if inputs == nil {
		return nil
	}
	var scanErr error
	walk := hclsyntax.VisitAll(inputs.Expr, func(node hclsyntax.Node) hcl.Diagnostics {
		if call, ok := node.(*hclsyntax.FunctionCallExpr); ok {
			if scanErr == nil {
				scanErr = trackSourceCall(fs, root, filepath.Dir(path), call, result)
			}
		}
		return nil
	})
	if walk.HasErrors() {
		return fmt.Errorf("%w %s", errTrackSources, path)
	}
	return scanErr
}

func trackSourceCall(fs afero.Fs, root, directory string, call *hclsyntax.FunctionCallExpr, result *model.Resolution) error {
	switch call.Name {
	case "file", "fileexists", "filebase64", "filemd5", "filesha1", "filesha256", "filesha512",
		"filebase64sha256", "filebase64sha512", "templatefile", "read_tfvars_file", "read_terragrunt_config",
		"sops_decrypt_file", "mark_as_read", "fileset", "find_in_parent_folders":
	default:
		return nil
	}
	name, known := literalSourcePath(call)
	if !known {
		result.Folders = append(result.Folders, root)
		return nil
	}
	path := localPath(directory, name)
	if call.Name == "read_terragrunt_config" {
		var err error
		path, err = watchedConfigPath(fs, path, result)
		if err != nil {
			return err
		}
	}
	if call.Name == "fileset" {
		result.Folders = append(result.Folders, path)
	} else {
		result.Files = append(result.Files, path)
	}
	return nil
}

// watchedConfigPath retains directory creation watches and scans the selected configuration.
func watchedConfigPath(fs afero.Fs, path string, result *model.Resolution) (string, error) {
	info, err := fs.Stat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if errors.Is(err, os.ErrNotExist) {
		result.Folders = append(result.Folders, path)
		return path, nil
	}
	if !info.IsDir() {
		return path, nil
	}
	result.Folders = append(result.Folders, path)
	return config.GetDefaultConfigPath(fs, path), nil
}

// literalSourcePath leaves dynamic and parent-search expressions as conservative directory watches.
func literalSourcePath(call *hclsyntax.FunctionCallExpr) (string, bool) {
	if len(call.Args) == 0 || call.Name == "find_in_parent_folders" {
		return "", false
	}
	value, diagnostics := call.Args[0].Value(nil)
	if diagnostics.HasErrors() || !value.IsKnown() || value.IsNull() || value.Type() != cty.String {
		return "", false
	}
	return value.AsString(), true
}
