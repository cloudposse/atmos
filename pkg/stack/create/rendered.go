package create

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/datafetcher"
	"github.com/cloudposse/atmos/pkg/generator/engine"
	"github.com/cloudposse/atmos/pkg/schema"
)

// validateRendered checks the primary manifest and every other rendered stack file under stacks.base_path:
// each must be one YAML document that satisfies the stack manifest JSON schema. It returns the parsed
// primary manifest.
func validateRendered(config *schema.AtmosConfiguration, root string, files []engine.File, manifest string) (map[string]any, error) {
	var primary map[string]any
	var schemaBytes []byte
	stacksBase := stacksBasePath(config)
	for i := range files {
		file := &files[i]
		isPrimary := filepath.Clean(file.Path) == filepath.Clean(manifest)
		if !isPrimary && !isStackDocument(stacksBase, root, file.Path) {
			continue
		}
		if schemaBytes == nil {
			var err error
			if schemaBytes, err = datafetcher.NewDataFetcher(config).GetData("atmos://schema/atmos/manifest/1.0"); err != nil {
				return nil, err
			}
		}
		document, err := validateStackDocument(schemaBytes, file)
		if err != nil {
			return nil, err
		}
		if isPrimary {
			primary = document
		}
	}
	if primary == nil {
		return nil, fmt.Errorf("%w: scaffold did not generate stack manifest %s", errUtils.ErrInvalidConfig, manifest)
	}
	return primary, nil
}

// isStackDocument reports whether a rendered file is a YAML stack file inside stacks.base_path. Template
// files are left alone because they are not valid YAML until the stack loader renders them.
func isStackDocument(stacksBase, root, path string) bool {
	if ext := filepath.Ext(path); ext != ".yaml" && ext != ".yml" {
		return false
	}
	relative, err := filepath.Rel(stacksBase, filepath.Join(root, path))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func validateStackDocument(schemaBytes []byte, file *engine.File) (map[string]any, error) {
	path := filepath.ToSlash(file.Path)
	var document map[string]any
	decoder := yaml.NewDecoder(strings.NewReader(file.Content))
	if err := decoder.Decode(&document); err != nil {
		// A template the author forgot to mark leaves actions in the output that fail to parse as YAML.
		return nil, engine.ExplainUnmarkedTemplate(file.Path, file.Content, file.Delimiters, fmt.Errorf("%w: generated stack %s: %w", errUtils.ErrInvalidConfig, path, err))
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: generated stack %s: a stack manifest must contain one YAML document", errUtils.ErrInvalidConfig, path)
	}
	result, err := gojsonschema.Validate(gojsonschema.NewBytesLoader(schemaBytes), gojsonschema.NewGoLoader(document))
	if err != nil {
		return nil, err
	}
	if !result.Valid() {
		return nil, fmt.Errorf("%w: generated stack %s: %v", errUtils.ErrInvalidConfig, path, result.Errors())
	}
	return document, nil
}
