package terraformadoption

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"

	errUtils "github.com/cloudposse/atmos/errors"
)

func packerEntry(selected, dir, base string) (map[string]any, error) {
	rel, err := relativeDirectory(base, dir)
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
	for _, f := range files {
		if !ignoredSource(f) && isPackerTemplate(f.Name()) {
			result = append(result, filepath.Join(selected, f.Name()))
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
