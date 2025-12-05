package exec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/filetype"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/utils"
)

// Supported formats for stack conversion.
const (
	FormatYAML      = "yaml"
	FormatJSON      = "json"
	FormatHCL       = "hcl"
	filePermissions = 0o600
)

var (
	ErrUnsupportedFormat   = errors.New("unsupported format")
	ErrConversionFailed    = errors.New("conversion failed")
	ErrUnknownSourceFormat = errors.New("unknown source format")
)

// ExecuteStackConvert converts a stack configuration file between formats.
func ExecuteStackConvert(
	atmosConfig *schema.AtmosConfiguration,
	inputPath string,
	targetFormat string,
	outputPath string,
	dryRun bool,
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteStackConvert")()

	// Validate target format.
	targetFormat = strings.ToLower(targetFormat)
	if !isValidFormat(targetFormat) {
		return fmt.Errorf("%w: %s (must be yaml, json, or hcl)", ErrUnsupportedFormat, targetFormat)
	}

	// Read and parse input file.
	inputData, sourceFormat, err := readAndParseFile(inputPath)
	if err != nil {
		return err
	}

	// Check if conversion would be a no-op.
	if sourceFormat == targetFormat {
		_ = ui.Warning(fmt.Sprintf("Source file is already in %s format", targetFormat))
	}

	// Convert to target format.
	output, err := convertToFormat(inputData, targetFormat)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrConversionFailed, err)
	}

	// Handle output.
	if dryRun {
		_ = ui.Info("Dry run - conversion preview:")
		_ = data.Writeln(output)
		return nil
	}

	if outputPath == "" {
		// Write to stdout.
		_ = data.Writeln(output)
		return nil
	}

	// Write to file.
	if err := os.WriteFile(outputPath, []byte(output), filePermissions); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	_ = ui.Success(fmt.Sprintf("Converted %s to %s", inputPath, outputPath))
	return nil
}

// isValidFormat checks if the format is supported.
func isValidFormat(format string) bool {
	switch format {
	case FormatYAML, FormatJSON, FormatHCL:
		return true
	default:
		return false
	}
}

// readAndParseFile reads a file and parses it, returning the data and detected format.
//
//nolint:cyclop,funlen,gocognit,nolintlint,revive
func readAndParseFile(path string) (map[string]any, string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read file: %w", err)
	}

	data := string(content)

	// Detect format and parse.
	var parsed any
	var format string

	switch {
	case filetype.IsJSON(data):
		format = FormatJSON
		if err := json.Unmarshal(content, &parsed); err != nil {
			return nil, "", fmt.Errorf("failed to parse JSON: %w", err)
		}
	case filetype.IsHCL(data):
		format = FormatHCL
		parsed, err = filetype.DetectFormatAndParseFile(os.ReadFile, path)
		if err != nil {
			return nil, "", fmt.Errorf("failed to parse HCL: %w", err)
		}
		// Unwrap HCL "stack" block if present.
		if m, ok := parsed.(map[string]any); ok {
			if stack, hasStack := m["stack"]; hasStack {
				if stackMap, ok := stack.(map[string]any); ok {
					parsed = stackMap
				}
			}
		}
	case filetype.IsYAML(data):
		format = FormatYAML
		if err := yaml.Unmarshal(content, &parsed); err != nil {
			return nil, "", fmt.Errorf("failed to parse YAML: %w", err)
		}
	default:
		// Try to infer from extension.
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".yaml", ".yml":
			format = FormatYAML
			if err := yaml.Unmarshal(content, &parsed); err != nil {
				return nil, "", fmt.Errorf("failed to parse YAML: %w", err)
			}
		case ".json":
			format = FormatJSON
			if err := json.Unmarshal(content, &parsed); err != nil {
				return nil, "", fmt.Errorf("failed to parse JSON: %w", err)
			}
		case ".hcl":
			format = FormatHCL
			parsed, err = filetype.DetectFormatAndParseFile(os.ReadFile, path)
			if err != nil {
				return nil, "", fmt.Errorf("failed to parse HCL: %w", err)
			}
		default:
			return nil, "", fmt.Errorf("%w: could not detect format for %s", ErrUnknownSourceFormat, path)
		}
	}

	// Ensure we have a map.
	result, ok := parsed.(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("%w: expected map, got %T", errUtils.ErrParseFile, parsed)
	}

	return result, format, nil
}

// convertToFormat converts the data to the specified format.
func convertToFormat(data map[string]any, format string) (string, error) {
	switch format {
	case FormatYAML:
		return convertToYAML(data)
	case FormatJSON:
		return convertToJSON(data)
	case FormatHCL:
		return convertToHCL(data)
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedFormat, format)
	}
}

// convertToYAML converts data to YAML format.
func convertToYAML(data map[string]any) (string, error) {
	out, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// convertToJSON converts data to JSON format.
func convertToJSON(data map[string]any) (string, error) {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// convertToHCL converts data to HCL format with stack wrapper block.
// Uses labeled blocks for components: component "name" { }.
// This format supports future stack naming: stack "name" { }.
func convertToHCL(data map[string]any) (string, error) {
	hclFile := hclwrite.NewEmptyFile()
	rootBody := hclFile.Body()

	// Create stack wrapper block.
	stackBlock := rootBody.AppendNewBlock("stack", nil)
	stackBody := stackBlock.Body()

	// Sort top-level keys for deterministic output.
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Write each top-level section.
	for _, key := range keys {
		value := data[key]
		switch key {
		case "components":
			// Special handling for components section.
			writeComponentsBlock(stackBody, value)
		default:
			// Generic block for other sections (vars, settings, etc.).
			writeGenericBlock(stackBody, key, value)
		}
	}

	return string(hclFile.Bytes()), nil
}

// writeComponentsBlock writes the components section with special handling for labeled blocks.
func writeComponentsBlock(body *hclwrite.Body, value any) {
	body.AppendNewline()
	componentsBlock := body.AppendNewBlock("components", nil)
	componentsBody := componentsBlock.Body()

	componentsMap, ok := value.(map[string]any)
	if !ok {
		return
	}

	// Sort component types (terraform, helmfile, etc.).
	typeKeys := sortedKeys(componentsMap)

	for _, componentType := range typeKeys {
		typeValue := componentsMap[componentType]
		typeMap, ok := typeValue.(map[string]any)
		if !ok {
			continue
		}

		componentsBody.AppendNewline()
		typeBlock := componentsBody.AppendNewBlock(componentType, nil)
		typeBody := typeBlock.Body()

		// Sort component names.
		componentNames := sortedKeys(typeMap)

		for _, componentName := range componentNames {
			componentValue := typeMap[componentName]
			componentMap, ok := componentValue.(map[string]any)
			if !ok {
				continue
			}

			typeBody.AppendNewline()
			// Use labeled block: component "name" { }.
			componentBlock := typeBody.AppendNewBlock("component", []string{componentName})
			componentBody := componentBlock.Body()

			writeMapToBody(componentBody, componentMap)
		}
	}
}

// writeGenericBlock writes a generic named block.
func writeGenericBlock(body *hclwrite.Body, name string, value any) {
	body.AppendNewline()

	switch v := value.(type) {
	case map[string]any:
		block := body.AppendNewBlock(name, nil)
		blockBody := block.Body()
		writeMapToBody(blockBody, v)
	default:
		// For non-map values, write as attribute.
		body.SetAttributeValue(name, utils.GoToCty(value))
	}
}

// writeMapToBody writes a map to an HCL body.
func writeMapToBody(body *hclwrite.Body, data map[string]any) {
	keys := sortedKeys(data)

	for _, key := range keys {
		value := data[key]
		writeValue(body, key, value)
	}
}

// writeValue writes a value to an HCL body, using blocks for nested maps.
func writeValue(body *hclwrite.Body, key string, value any) {
	switch v := value.(type) {
	case map[string]any:
		// For nested maps, create a block.
		body.AppendNewline()
		block := body.AppendNewBlock(key, nil)
		blockBody := block.Body()
		writeMapToBody(blockBody, v)
	default:
		// For other values, use attribute.
		body.SetAttributeValue(key, goToCtyForHCL(value))
	}
}

// goToCtyForHCL converts Go types to cty.Value, handling special cases for HCL.
//
//nolint:cyclop,funlen,gocognit,nolintlint,revive
func goToCtyForHCL(value any) cty.Value {
	if value == nil {
		return cty.NullVal(cty.DynamicPseudoType)
	}

	switch v := value.(type) {
	case string:
		return cty.StringVal(v)
	case bool:
		return cty.BoolVal(v)
	case int:
		return cty.NumberIntVal(int64(v))
	case int64:
		return cty.NumberIntVal(v)
	case float64:
		return cty.NumberFloatVal(v)
	case []any:
		if len(v) == 0 {
			return cty.EmptyTupleVal
		}
		vals := make([]cty.Value, len(v))
		for i, item := range v {
			vals[i] = goToCtyForHCL(item)
		}
		return cty.TupleVal(vals)
	case map[string]any:
		if len(v) == 0 {
			return cty.EmptyObjectVal
		}
		vals := make(map[string]cty.Value, len(v))
		for k, item := range v {
			vals[k] = goToCtyForHCL(item)
		}
		return cty.ObjectVal(vals)
	default:
		// Try to use the existing utils function.
		return utils.GoToCty(value)
	}
}

// sortedKeys returns the keys of a map in sorted order.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ConvertStackToBytes converts stack data to the specified format as bytes.
// This is useful for programmatic use.
func ConvertStackToBytes(data map[string]any, format string) ([]byte, error) {
	defer perf.Track(nil, "exec.ConvertStackToBytes")()

	output, err := convertToFormat(data, format)
	if err != nil {
		return nil, err
	}
	return []byte(output), nil
}

// DetectStackFormat detects the format of a stack file by its content.
func DetectStackFormat(content string) string {
	defer perf.Track(nil, "exec.DetectStackFormat")()

	switch {
	case filetype.IsJSON(content):
		return FormatJSON
	case filetype.IsHCL(content):
		return FormatHCL
	case filetype.IsYAML(content):
		return FormatYAML
	default:
		return ""
	}
}

// ParseStackFile parses a stack file and returns the data.
func ParseStackFile(path string) (map[string]any, error) {
	defer perf.Track(nil, "exec.ParseStackFile")()

	data, _, err := readAndParseFile(path)
	return data, err
}

// WriteStackFile writes stack data to a file in the specified format.
func WriteStackFile(path string, data map[string]any, format string) error {
	defer perf.Track(nil, "exec.WriteStackFile")()

	output, err := convertToFormat(data, format)
	if err != nil {
		return err
	}

	var content bytes.Buffer
	content.WriteString(output)

	// Ensure trailing newline.
	if !strings.HasSuffix(output, "\n") {
		content.WriteByte('\n')
	}

	return os.WriteFile(path, content.Bytes(), filePermissions)
}
