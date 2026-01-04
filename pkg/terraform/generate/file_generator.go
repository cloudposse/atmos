// Package generate provides functionality to generate files from the generate section
// in Atmos stack configuration.
package generate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// filePermissions is the default permission mode for generated config files.
const filePermissions = 0o644

// GenerateConfig contains configuration for file generation.
type GenerateConfig struct {
	// DryRun when true, shows what would be generated without writing.
	DryRun bool
	// Clean when true, deletes generated files instead of creating.
	Clean bool
}

// GenerateResult contains information about a generated file.
type GenerateResult struct {
	// Filename is the name of the generated file.
	Filename string
	// Path is the full path where the file was written.
	Path string
	// Created indicates if the file was created (vs updated).
	Created bool
	// Deleted indicates if the file was deleted (clean mode).
	Deleted bool
	// Skipped indicates if the file was skipped (dry-run mode).
	Skipped bool
	// Error contains any error that occurred.
	Error error
}

// GenerateFiles generates files from the generate section of a component configuration.
// It returns a slice of GenerateResult describing what was generated.
func GenerateFiles(
	generateSection map[string]any,
	componentDir string,
	templateContext map[string]any,
	config GenerateConfig,
) ([]GenerateResult, error) {
	defer perf.Track(nil, "generate.GenerateFiles")()

	if generateSection == nil {
		return nil, nil
	}

	var results []GenerateResult

	for filename, content := range generateSection {
		result := GenerateResult{Filename: filename}
		filePath := filepath.Join(componentDir, filename)
		result.Path = filePath

		if config.Clean {
			processCleanFile(&result, filePath, config.DryRun)
		} else {
			processGenerateFile(&result, fileContext{
				filename:        filename,
				filePath:        filePath,
				content:         content,
				templateContext: templateContext,
				dryRun:          config.DryRun,
			})
		}

		results = append(results, result)
	}

	return results, nil
}

// fileContext holds context for file generation operations.
type fileContext struct {
	filename        string
	filePath        string
	content         any
	templateContext map[string]any
	dryRun          bool
}

// processCleanFile handles file deletion in clean mode.
func processCleanFile(result *GenerateResult, filePath string, dryRun bool) {
	if dryRun {
		result.Skipped = true
		_ = ui.Writef("Would delete: %s\n", filePath)
		return
	}

	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			result.Skipped = true
		} else {
			result.Error = fmt.Errorf("failed to delete %s: %w", filePath, err)
		}
		return
	}

	result.Deleted = true
	_ = ui.Writef("Deleted: %s\n", filePath)
}

// processGenerateFile handles file generation in generate mode.
func processGenerateFile(result *GenerateResult, ctx fileContext) {
	fileContent, err := renderContent(ctx.filename, ctx.content, ctx.templateContext)
	if err != nil {
		result.Error = fmt.Errorf("failed to render %s: %w", ctx.filename, err)
		return
	}

	if ctx.dryRun {
		result.Skipped = true
		_ = ui.Writef("Would generate: %s\n", ctx.filePath)
		return
	}

	// Check if file exists.
	_, statErr := os.Stat(ctx.filePath)
	result.Created = os.IsNotExist(statErr)

	// Write file with standard permissions for config files.
	if err := os.WriteFile(ctx.filePath, fileContent, filePermissions); err != nil {
		result.Error = fmt.Errorf("failed to write %s: %w", ctx.filePath, err)
		return
	}

	if result.Created {
		_ = ui.Writef("Created: %s\n", ctx.filePath)
	} else {
		_ = ui.Writef("Updated: %s\n", ctx.filePath)
	}
}

// GetGenerateFilenames extracts the list of filenames from a generate section.
// This is used by terraform clean to know which files to delete.
func GetGenerateFilenames(generateSection map[string]any) []string {
	defer perf.Track(nil, "generate.GetGenerateFilenames")()

	if generateSection == nil {
		return nil
	}

	filenames := make([]string, 0, len(generateSection))
	for filename := range generateSection {
		filenames = append(filenames, filename)
	}
	return filenames
}

// renderContent renders the content for a file based on its type and extension.
func renderContent(filename string, content any, templateContext map[string]any) ([]byte, error) {
	switch v := content.(type) {
	case string:
		// String content is a Go template.
		return renderTemplate(filename, v, templateContext)
	case map[string]any:
		// Map content is serialized based on file extension.
		return serializeByExtension(filename, v, templateContext)
	default:
		return nil, fmt.Errorf("%w: unsupported content type %T for file %s", errUtils.ErrInvalidConfig, content, filename)
	}
}

// renderTemplate renders a Go template string with the given context.
func renderTemplate(name, templateStr string, context map[string]any) ([]byte, error) {
	tmpl, err := template.New(name).Parse(templateStr)
	if err != nil {
		return nil, fmt.Errorf("template parse error: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, context); err != nil {
		return nil, fmt.Errorf("template execution error: %w", err)
	}

	return buf.Bytes(), nil
}

// yamlIndent is the number of spaces to use for YAML indentation.
const yamlIndent = 2

// serializeByExtension serializes a map to the appropriate format based on file extension.
// All formats are pretty-printed with proper indentation for readability.
func serializeByExtension(filename string, content map[string]any, templateContext map[string]any) ([]byte, error) {
	ext := strings.ToLower(filepath.Ext(filename))

	// First, render any template strings in the content.
	rendered, err := renderMapTemplates(content, templateContext)
	if err != nil {
		return nil, err
	}

	switch ext {
	case ".json":
		return json.MarshalIndent(rendered, "", "  ")
	case ".yaml", ".yml":
		return serializeToYAML(rendered)
	case ".hcl", ".tf":
		return serializeToHCL(rendered)
	default:
		// Default to JSON for unknown extensions.
		return json.MarshalIndent(rendered, "", "  ")
	}
}

// serializeToYAML converts a map to pretty-printed YAML format.
func serializeToYAML(content map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(yamlIndent)

	if err := encoder.Encode(content); err != nil {
		return nil, fmt.Errorf("YAML encoding error: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("YAML encoder close error: %w", err)
	}

	return buf.Bytes(), nil
}

// renderMapTemplates recursively renders template strings in a map.
func renderMapTemplates(content map[string]any, templateContext map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	for key, value := range content {
		switch v := value.(type) {
		case string:
			// Render template string.
			rendered, err := renderTemplate(key, v, templateContext)
			if err != nil {
				return nil, fmt.Errorf("error rendering template for key %s: %w", key, err)
			}
			result[key] = string(rendered)
		case map[string]any:
			// Recursively render nested maps.
			rendered, err := renderMapTemplates(v, templateContext)
			if err != nil {
				return nil, err
			}
			result[key] = rendered
		case []any:
			// Handle arrays.
			rendered, err := renderArrayTemplates(v, templateContext)
			if err != nil {
				return nil, err
			}
			result[key] = rendered
		default:
			// Keep other types as-is.
			result[key] = value
		}
	}

	return result, nil
}

// renderArrayTemplates recursively renders template strings in an array.
func renderArrayTemplates(content []any, templateContext map[string]any) ([]any, error) {
	result := make([]any, len(content))

	for i, value := range content {
		switch v := value.(type) {
		case string:
			rendered, err := renderTemplate(fmt.Sprintf("[%d]", i), v, templateContext)
			if err != nil {
				return nil, err
			}
			result[i] = string(rendered)
		case map[string]any:
			rendered, err := renderMapTemplates(v, templateContext)
			if err != nil {
				return nil, err
			}
			result[i] = rendered
		case []any:
			rendered, err := renderArrayTemplates(v, templateContext)
			if err != nil {
				return nil, err
			}
			result[i] = rendered
		default:
			result[i] = value
		}
	}

	return result, nil
}

// serializeToHCL converts a map to HCL format.
func serializeToHCL(content map[string]any) ([]byte, error) {
	f := hclwrite.NewEmptyFile()
	body := f.Body()

	if err := writeHCLBlock(body, content); err != nil {
		return nil, err
	}

	return f.Bytes(), nil
}

// writeHCLBlock writes content to an HCL body.
func writeHCLBlock(body *hclwrite.Body, content map[string]any) error {
	for key, value := range content {
		switch v := value.(type) {
		case map[string]any:
			// Create a block for nested maps.
			block := body.AppendNewBlock(key, nil)
			if err := writeHCLBlock(block.Body(), v); err != nil {
				return err
			}
		default:
			// Convert value to cty and set as attribute.
			ctyVal, err := toCtyValue(value)
			if err != nil {
				return fmt.Errorf("error converting %s to HCL: %w", key, err)
			}
			body.SetAttributeValue(key, ctyVal)
		}
	}
	return nil
}

// toCtyValue converts a Go value to a cty.Value for HCL serialization.
// Uses TupleVal for slices and ObjectVal for maps to support mixed types.
func toCtyValue(value any) (cty.Value, error) {
	switch v := value.(type) {
	case string:
		return cty.StringVal(v), nil
	case bool:
		return cty.BoolVal(v), nil
	case int:
		return cty.NumberIntVal(int64(v)), nil
	case int64:
		return cty.NumberIntVal(v), nil
	case float64:
		return cty.NumberFloatVal(v), nil
	case []any:
		return sliceToCtyTuple(v)
	case map[string]any:
		return mapToCtyObject(v)
	case nil:
		return cty.NullVal(cty.DynamicPseudoType), nil
	default:
		return cty.NilVal, errUtils.ErrUnsupportedInputType
	}
}

// sliceToCtyTuple converts a Go slice to a cty.TupleVal.
// Using TupleVal instead of ListVal allows mixed element types.
func sliceToCtyTuple(v []any) (cty.Value, error) {
	if len(v) == 0 {
		return cty.EmptyTupleVal, nil
	}
	vals := make([]cty.Value, len(v))
	for i, item := range v {
		val, err := toCtyValue(item)
		if err != nil {
			return cty.NilVal, err
		}
		vals[i] = val
	}
	return cty.TupleVal(vals), nil
}

// mapToCtyObject converts a Go map to a cty.ObjectVal.
// Using ObjectVal instead of MapVal allows mixed value types.
func mapToCtyObject(v map[string]any) (cty.Value, error) {
	if len(v) == 0 {
		return cty.EmptyObjectVal, nil
	}
	vals := make(map[string]cty.Value)
	for key, item := range v {
		val, err := toCtyValue(item)
		if err != nil {
			return cty.NilVal, err
		}
		vals[key] = val
	}
	return cty.ObjectVal(vals), nil
}
