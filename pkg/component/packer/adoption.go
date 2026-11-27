package packer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	stackadoption "github.com/cloudposse/atmos/pkg/stack/adoption"
)

func packerEntry(selected, dir, base string) (map[string]any, error) {
	rel, err := stackadoption.RelativeDirectory(base, dir)
	if err != nil {
		return nil, err
	}
	files, err := packerTemplates(selected)
	if err != nil {
		return nil, err
	}
	count := 0
	for _, file := range files {
		n, err := packerBuildSources(file)
		if err != nil {
			return nil, err
		}
		count += n
	}
	if count != 1 {
		return nil, fmt.Errorf("%w: selected Packer configuration has %d build sources; select a template with one build source", errUtils.ErrInvalidComponent, count)
	}
	entry := map[string]any{"metadata": map[string]any{"component": rel}}
	if len(files) == 1 && files[0] == selected {
		entry["settings"] = map[string]any{packerKind: map[string]any{"template": filepath.Base(selected)}}
	}
	return entry, nil
}

func packerTemplates(selected string) ([]string, error) {
	info, err := os.Stat(selected)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{selected}, nil
	}
	files, err := os.ReadDir(selected)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, name := range stackadoption.FileNames(files) {
		if isPackerTemplate(name) {
			result = append(result, filepath.Join(selected, name))
		}
	}
	return result, nil
}

// Count source instances without evaluating variables, provisioning or fetching plugins.
func packerBuildSources(path string) (int, error) {
	p := hclparse.NewParser()
	var file *hcl.File
	var diags hcl.Diagnostics
	if strings.HasSuffix(path, ".json") {
		file, diags = p.ParseJSONFile(path)
	} else {
		file, diags = p.ParseHCLFile(path)
	}
	if diags.HasErrors() {
		return 0, fmt.Errorf("%w: %s", errUtils.ErrInvalidComponent, diags.Error())
	}
	body, _, diags := file.Body.PartialContent(&hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{{Type: "build"}}})
	if diags.HasErrors() {
		return 0, fmt.Errorf("%w: %s", errUtils.ErrInvalidComponent, diags.Error())
	}
	count := 0
	for _, block := range body.Blocks {
		n, err := countPackerSources(block)
		if err != nil {
			return 0, err
		}
		count += n
	}
	return count, nil
}

func countPackerSources(build *hcl.Block) (int, error) {
	body, _, diags := build.Body.PartialContent(&hcl.BodySchema{Attributes: []hcl.AttributeSchema{{Name: "sources"}}, Blocks: []hcl.BlockHeaderSchema{{Type: "source", LabelNames: []string{"source"}}}})
	if diags.HasErrors() {
		return 0, fmt.Errorf("%w: %s", errUtils.ErrInvalidComponent, diags.Error())
	}
	count := len(body.Blocks)
	if attr := body.Attributes["sources"]; attr != nil {
		list, diags := hcl.ExprList(attr.Expr)
		if diags.HasErrors() {
			return 0, fmt.Errorf("%w: %s must declare a literal list of Packer sources", errUtils.ErrInvalidComponent, attr.Range)
		}
		count += len(list)
	}
	return count, nil
}

func isPackerTemplate(name string) bool {
	return strings.HasSuffix(name, ".pkr.hcl") || strings.HasSuffix(name, ".pkr.json")
}

// Adapter registers local HCL2 Packer configurations with a single build source.
type Adapter struct{}

const packerKind = "packer"

func (Adapter) Kind() string {
	defer perf.Track(nil, "packer.Adapter.Kind")()
	return packerKind
}

func (Adapter) Options() []string {
	defer perf.Track(nil, "packer.Adapter.Options")()
	return []string{"name", "var-file"}
}

func (Adapter) Detect(s stackadoption.Source) (stackadoption.Match, error) {
	defer perf.Track(nil, "packer.Adapter.Detect")()

	if s.IsDir && slices.ContainsFunc(s.Files, isPackerTemplate) || !s.IsDir && isPackerTemplate(filepath.Base(s.Path)) {
		return stackadoption.Match{Path: s.Path}, nil
	}
	return stackadoption.Match{}, nil
}

func (a Adapter) Adopt(_ context.Context, r *stackadoption.Registration, path string) error {
	defer perf.Track(nil, "packer.Adapter.Adopt")()

	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	dir := path
	if !info.IsDir() {
		dir = filepath.Dir(path)
	}
	base := ""
	if r.Config != nil {
		base = r.Config.PackerDirAbsolutePath
	}
	return r.WriteLocalComponent(&stackadoption.LocalComponent{
		Kind:              a.Kind(),
		Name:              r.Name,
		Dir:               dir,
		ConfiguredBase:    base,
		NextCommand:       "validate",
		VarFileExtensions: []string{".pkrvars.hcl", ".pkrvars.json"},
		Entry:             func(dir, base string) (map[string]any, error) { return packerEntry(path, dir, base) },
	})
}
