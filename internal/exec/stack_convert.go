package exec

import (
	"encoding/json"
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
	FormatYAML            = "yaml"
	FormatJSON            = "json"
	FormatHCL             = "hcl"
	stackConvertFilePerms = 0o600
	nameSectionName       = "name"
)

// ExecuteStackConvert converts a stack configuration file between formats.
// Supports multi-document YAML files and multi-stack HCL files.
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
		return errUtils.Build(errUtils.ErrUnsupportedFormat).
			WithExplanation(fmt.Sprintf("Format '%s' is not supported", targetFormat)).
			WithHint("Use yaml, json, or hcl").
			WithContext("format", targetFormat).
			Err()
	}

	// Read and parse input file (supports multi-document).
	stacks, sourceFormat, err := readAndParseMultiDocFile(inputPath)
	if err != nil {
		return err
	}

	// Check if conversion would be a no-op.
	if sourceFormat == targetFormat {
		ui.Warning(fmt.Sprintf("Source file is already in %s format", targetFormat))
	}

	// Convert to target format.
	output, err := convertStacksToFormat(stacks, targetFormat)
	if err != nil {
		return errUtils.Build(errUtils.ErrStackConversionFailed).
			WithCause(err).
			WithExplanation("Failed to convert stack configuration").
			WithContext("source_format", sourceFormat).
			WithContext("target_format", targetFormat).
			Err()
	}

	// Handle output.
	if dryRun {
		ui.Info("Dry run - conversion preview:")
		data.Writeln(output)
		return nil
	}

	if outputPath == "" {
		// Write to stdout.
		data.Writeln(output)
		return nil
	}

	// Write to file.
	if err := os.WriteFile(outputPath, []byte(output), stackConvertFilePerms); err != nil {
		return errUtils.Build(errUtils.ErrWriteToStream).
			WithCause(err).
			WithExplanation("Failed to write output file").
			WithContext("output_path", outputPath).
			Err()
	}

	ui.Success(fmt.Sprintf("Converted %s to %s", inputPath, outputPath))
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

// readAndParseMultiDocFile reads a file and parses it, supporting multi-document YAML and multi-stack HCL.
// Returns a slice of StackDocument and the detected source format.
func readAndParseMultiDocFile(path string) ([]filetype.StackDocument, string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, "", errUtils.Build(errUtils.ErrReadFile).
			WithCause(err).
			WithExplanation("Failed to read input file").
			WithContext("path", path).
			Err()
	}

	dataStr := string(content)
	ext := strings.ToLower(filepath.Ext(path))

	// Detect format.
	format := detectFormat(dataStr, ext)
	if format == "" {
		return nil, "", errUtils.Build(errUtils.ErrUnknownSourceFormat).
			WithExplanation("Could not detect file format from content or extension").
			WithHint("Ensure the file has a valid extension (.yaml, .yml, .json, .hcl) or valid content").
			WithContext("path", path).
			WithContext("extension", ext).
			Err()
	}

	// Parse based on format.
	stacks, err := parseByFormat(content, path, format)
	if err != nil {
		return nil, "", err
	}

	return stacks, format, nil
}

// detectFormat detects the format from content or falls back to file extension.
func detectFormat(content, ext string) string {
	switch {
	case filetype.IsJSON(content):
		return FormatJSON
	case filetype.IsHCL(content):
		return FormatHCL
	case filetype.IsYAML(content):
		return FormatYAML
	}

	// Fall back to extension.
	switch ext {
	case ".yaml", ".yml":
		return FormatYAML
	case ".json":
		return FormatJSON
	case ".hcl":
		return FormatHCL
	default:
		return ""
	}
}

// parseByFormat parses content according to the specified format.
func parseByFormat(content []byte, path, format string) ([]filetype.StackDocument, error) {
	switch format {
	case FormatYAML:
		stacks, err := filetype.ParseYAMLStacks(content)
		if err != nil {
			return nil, errUtils.Build(errUtils.ErrLoaderParseFailed).
				WithCause(err).
				WithExplanation("Failed to parse YAML content").
				WithContext("path", path).
				Err()
		}
		return stacks, nil
	case FormatHCL:
		stacks, err := filetype.ParseHCLStacks(content, path)
		if err != nil {
			return nil, errUtils.Build(errUtils.ErrLoaderParseFailed).
				WithCause(err).
				WithExplanation("Failed to parse HCL content").
				WithContext("path", path).
				Err()
		}
		return stacks, nil
	case FormatJSON:
		return parseJSONStack(content)
	default:
		return nil, errUtils.Build(errUtils.ErrUnsupportedFormat).
			WithExplanation(fmt.Sprintf("Format '%s' is not supported for parsing", format)).
			WithContext("format", format).
			Err()
	}
}

// parseJSONStack parses JSON content as a single stack document or array of stacks.
func parseJSONStack(content []byte) ([]filetype.StackDocument, error) {
	// First, try to parse as an array (multi-stack).
	var parsed any
	if err := json.Unmarshal(content, &parsed); err != nil {
		return nil, errUtils.Build(errUtils.ErrLoaderParseFailed).
			WithCause(err).
			WithExplanation("Failed to parse JSON content").
			Err()
	}

	switch v := parsed.(type) {
	case map[string]any:
		// Single stack object.
		name := ""
		if n, ok := v[nameSectionName].(string); ok {
			name = n
		}
		return []filetype.StackDocument{{Name: name, Config: v}}, nil

	case []any:
		// Array of stacks.
		out := make([]filetype.StackDocument, 0, len(v))
		for i, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, errUtils.Build(errUtils.ErrLoaderParseFailed).
					WithExplanation(fmt.Sprintf("Array element %d is not an object", i)).
					Err()
			}
			name := ""
			if n, ok := m[nameSectionName].(string); ok {
				name = n
			}
			out = append(out, filetype.StackDocument{Name: name, Config: m})
		}
		return out, nil

	default:
		return nil, errUtils.Build(errUtils.ErrLoaderParseFailed).
			WithExplanation("JSON must be an object or array of objects").
			Err()
	}
}

// convertStacksToFormat converts multiple stacks to the specified format.
func convertStacksToFormat(stacks []filetype.StackDocument, format string) (string, error) {
	switch format {
	case FormatYAML:
		return convertStacksToYAML(stacks)
	case FormatJSON:
		return convertStacksToJSON(stacks)
	case FormatHCL:
		return convertStacksToHCL(stacks)
	default:
		return "", errUtils.Build(errUtils.ErrUnsupportedFormat).
			WithExplanation(fmt.Sprintf("Format '%s' is not supported for output", format)).
			WithContext("format", format).
			Err()
	}
}

// convertStacksToYAML converts stacks to multi-document YAML format.
func convertStacksToYAML(stacks []filetype.StackDocument) (string, error) {
	if len(stacks) == 0 {
		return "", nil
	}

	var result strings.Builder
	for i, stack := range stacks {
		if i > 0 {
			result.WriteString("---\n")
		}
		// Use prepareStackConfig to get a cloned config with name included.
		config := prepareStackConfig(stack)
		out, err := yaml.Marshal(config)
		if err != nil {
			return "", err
		}
		result.Write(out)
	}
	return result.String(), nil
}

// convertStacksToJSON converts stacks to JSON format.
// For multiple stacks, outputs a JSON array.
func convertStacksToJSON(stacks []filetype.StackDocument) (string, error) {
	if len(stacks) == 0 {
		return "{}", nil
	}

	// For a single stack, output as object.
	if len(stacks) == 1 {
		config := prepareStackConfig(stacks[0])
		out, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return "", err
		}
		return string(out), nil
	}

	// For multiple stacks, output as array.
	configs := make([]map[string]any, 0, len(stacks))
	for _, stack := range stacks {
		configs = append(configs, prepareStackConfig(stack))
	}
	out, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// prepareStackConfig ensures the stack name is included in the config map.
// Returns a shallow clone to avoid mutating the input.
func prepareStackConfig(stack filetype.StackDocument) map[string]any {
	// Create a shallow clone to avoid mutating the input map.
	config := make(map[string]any, len(stack.Config)+1)
	for k, v := range stack.Config {
		config[k] = v
	}

	if stack.Name != "" {
		config[nameSectionName] = stack.Name
	}
	return config
}

// convertStacksToHCL converts stacks to multi-stack HCL format.
func convertStacksToHCL(stacks []filetype.StackDocument) (string, error) {
	if len(stacks) == 0 {
		return "", nil
	}

	hclFile := hclwrite.NewEmptyFile()
	rootBody := hclFile.Body()

	for i, stack := range stacks {
		if i > 0 {
			rootBody.AppendNewline()
		}
		writeStackToHCL(rootBody, stack)
	}

	return string(hclFile.Bytes()), nil
}

// writeStackToHCL writes a single stack document to the HCL body.
func writeStackToHCL(rootBody *hclwrite.Body, stack filetype.StackDocument) {
	// Use stack name as block label if present.
	var stackLabels []string
	if stack.Name != "" {
		stackLabels = []string{stack.Name}
	}

	stackBlock := rootBody.AppendNewBlock("stack", stackLabels)
	stackBody := stackBlock.Body()

	// Get sorted keys, excluding name if used as block label.
	keys := getStackConfigKeys(stack.Config, len(stackLabels) > 0)

	// Write each top-level section.
	for _, key := range keys {
		value := stack.Config[key]
		if key == "components" {
			writeComponentsBlock(stackBody, value)
		} else {
			writeGenericBlock(stackBody, key, value)
		}
	}
}

// getStackConfigKeys returns sorted keys from config, optionally excluding the name key.
func getStackConfigKeys(config map[string]any, excludeName bool) []string {
	keys := make([]string, 0, len(config))
	for k := range config {
		if excludeName && k == nameSectionName {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
//nolint:cyclop,funlen,gocognit,nolintlint,revive // Type conversion requires exhaustive switch with explicit handling for each Go→cty type mapping.
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
