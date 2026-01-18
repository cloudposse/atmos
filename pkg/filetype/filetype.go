package filetype

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	hclv1 "github.com/hashicorp/hcl"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/function"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ErrFailedToProcessHclFile is an alias for the central error sentinel.
// Deprecated: Use errUtils.ErrFailedToProcessHCL directly.
var ErrFailedToProcessHclFile = errUtils.ErrFailedToProcessHCL

const (
	errFmtProcessHCLFile = "%w, file: %s, error: %s"

	// StackBlockType is the HCL block type for Atmos stack configurations.
	// When this block has a label, it becomes the explicit stack name.
	stackBlockType = "stack"

	// NameSectionName is the key for explicit stack names in stack manifests.
	// Duplicated here to avoid import cycle with pkg/config.
	nameSectionName = "name"
)

// StackDocument represents a single stack from a multi-stack file.
// Used when a file contains multiple stack definitions.
type StackDocument struct {
	Name   string         // Explicit stack name (from label or name field).
	Config map[string]any // Stack configuration.
}

// hclEvalContext returns an HCL evaluation context with Atmos functions available.
// Functions like env(), exec(), template(), and repo_root() can be used in HCL expressions.
//
// NOTE: PreMerge functions (env, random, include, etc.) are evaluated during parsing.
// This means stack convert output may vary between runs if these functions are used.
// For deterministic conversion, avoid PreMerge functions in the source file or use
// PostMerge functions (terraform.output, store, etc.) which return placeholders.
func hclEvalContext() *hcl.EvalContext {
	registry := function.DefaultRegistry(nil)
	return function.HCLEvalContextWithFunctions(registry, nil)
}

// IsYAML checks if data is in YAML format.
func IsYAML(data string) bool {
	defer perf.Track(nil, "filetype.IsYAML")()

	if strings.TrimSpace(data) == "" {
		return false
	}

	var yml any
	err := yaml.Unmarshal([]byte(data), &yml)
	if err != nil {
		return false
	}

	// Ensure that the parsed result is not nil and has some meaningful content.
	_, isMap := yml.(map[string]any)
	_, isSlice := yml.([]any)

	return isMap || isSlice
}

// IsHCL checks if data is in HCL format.
func IsHCL(data string) bool {
	defer perf.Track(nil, "filetype.IsHCL")()

	if strings.TrimSpace(data) == "" {
		return false
	}

	var hclData any
	return hclv1.Unmarshal([]byte(data), &hclData) == nil
}

// IsJSON checks if data is in JSON format.
func IsJSON(data string) bool {
	defer perf.Track(nil, "filetype.IsJSON")()

	if strings.TrimSpace(data) == "" {
		return false
	}

	var js json.RawMessage
	return json.Unmarshal([]byte(data), &js) == nil
}

// DetectFormatAndParseFile detects the format of the file (JSON, YAML, HCL) and parses the file into a Go type.
// For all other formats, it just reads the file and returns the content as a string.
func DetectFormatAndParseFile(readFileFunc func(string) ([]byte, error), filename string) (any, error) {
	defer perf.Track(nil, "filetype.DetectFormatAndParseFile")()

	d, err := readFileFunc(filename)
	if err != nil {
		return nil, err
	}

	data := string(d)
	switch {
	case IsJSON(data):
		return parseJSON(d)
	case IsHCL(data):
		return parseHCL(d, filename)
	case IsYAML(data):
		return parseYAML(d)
	default:
		return data, nil
	}
}

func parseJSON(data []byte) (any, error) {
	var v any
	err := json.Unmarshal(data, &v)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func parseYAML(data []byte) (any, error) {
	// First, unmarshal into a yaml.Node to preserve the original structure.
	var node yaml.Node
	err := yaml.Unmarshal(data, &node)
	if err != nil {
		return nil, err
	}

	// Process the node to ensure strings starting with '#' are properly handled.
	processYAMLNode(&node)

	// Decode the processed node into a Go value.
	var v any
	err = node.Decode(&v)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func processYAMLNode(node *yaml.Node) {
	if node == nil {
		return
	}

	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" && strings.HasPrefix(node.Value, "#") {
		node.Style = yaml.SingleQuotedStyle
	}

	for _, child := range node.Content {
		processYAMLNode(child)
	}
}

func parseHCL(data []byte, filename string) (any, error) {
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL(data, filename)
	if diags != nil && diags.HasErrors() {
		return nil, fmt.Errorf(errFmtProcessHCLFile, ErrFailedToProcessHclFile, filename, diags.Error())
	}
	if file == nil {
		return nil, fmt.Errorf("%w, file: %s, file parsing returned nil", ErrFailedToProcessHclFile, filename)
	}

	// Parse both attributes and blocks from the HCL body.
	return parseHCLBody(file.Body, filename)
}

// parseHCLBody parses an HCL body, handling both attributes and blocks.
// This supports both attribute syntax (key = value) and block syntax (block { ... }).
func parseHCLBody(body hcl.Body, filename string) (map[string]any, error) {
	result := make(map[string]any)

	// First, try to get just attributes (for simple HCL files or attribute-only sections).
	attrs, attrDiags := body.JustAttributes()

	// If JustAttributes fails, check if it's due to blocks or actual syntax errors.
	if attrDiags != nil && attrDiags.HasErrors() {
		// Check if the diagnostics indicate blocks are present vs. real syntax errors.
		if isBlockRelatedDiagnostic(attrDiags) {
			// There are blocks present - we need to handle them differently.
			return parseHCLBodyWithBlocks(body, filename)
		}
		// This is a genuine syntax error - propagate it.
		return nil, fmt.Errorf(errFmtProcessHCLFile, ErrFailedToProcessHclFile, filename, attrDiags.Error())
	}

	// Process attributes only (no blocks in this body).
	evalCtx := hclEvalContext()
	for name, attr := range attrs {
		ctyValue, valDiags := attr.Expr.Value(evalCtx)
		if valDiags != nil && valDiags.HasErrors() {
			return nil, fmt.Errorf(errFmtProcessHCLFile, ErrFailedToProcessHclFile, filename, valDiags.Error())
		}
		result[name] = ctyToGo(ctyValue)
	}
	return result, nil
}

// parseHCLBodyWithBlocks handles HCL bodies that contain blocks using hclsyntax direct access.
// Complexity is inherent to recursive HCL structure traversal - refactoring deferred.
//
//nolint:cyclop,funlen,gocognit,nestif,revive
func parseHCLBodyWithBlocks(body hcl.Body, filename string) (map[string]any, error) {
	result := make(map[string]any)
	evalCtx := hclEvalContext()

	// Type assert to get the underlying hclsyntax.Body which gives us direct access.
	// This is needed because the hcl.Body interface doesn't provide a way to iterate
	// over unknown block types.
	if syntaxBody, ok := body.(*hclsyntax.Body); ok {
		// Process attributes.
		for name, attr := range syntaxBody.Attributes {
			ctyValue, valDiags := attr.Expr.Value(evalCtx)
			if valDiags != nil && valDiags.HasErrors() {
				return nil, fmt.Errorf("%w, file: %s, attribute %s error: %s", ErrFailedToProcessHclFile, filename, name, valDiags.Error())
			}
			result[name] = ctyToGo(ctyValue)
		}

		// Process blocks recursively.
		for _, block := range syntaxBody.Blocks {
			blockContent, err := parseHCLBodyWithBlocks(block.Body, filename)
			if err != nil {
				return nil, err
			}

			// For blocks without labels, just use the block type as the key.
			if len(block.Labels) == 0 {
				// No labels - merge directly or create nested map.
				if existing, ok := result[block.Type]; ok {
					// If the key already exists, merge the content.
					if existingMap, ok := existing.(map[string]any); ok {
						for k, v := range blockContent {
							existingMap[k] = v
						}
					} else {
						// Type mismatch - existing value is not a map, overwrite.
						result[block.Type] = blockContent
					}
				} else {
					result[block.Type] = blockContent
				}
			} else {
				// Has labels - this is less common in Atmos stack config but handle it.
				// e.g., `resource "aws_instance" "example" { ... }`
				current := result
				if _, ok := current[block.Type]; !ok {
					current[block.Type] = make(map[string]any)
				}
				typeMap, ok := current[block.Type].(map[string]any)
				if !ok {
					return nil, fmt.Errorf("%w, file: %s, block type %s has unexpected value type", ErrFailedToProcessHclFile, filename, block.Type)
				}
				for i, label := range block.Labels {
					if i == len(block.Labels)-1 {
						typeMap[label] = blockContent
					} else {
						if _, ok := typeMap[label]; !ok {
							typeMap[label] = make(map[string]any)
						}
						labelMap, ok := typeMap[label].(map[string]any)
						if !ok {
							return nil, fmt.Errorf("%w, file: %s, block label %s has unexpected value type", ErrFailedToProcessHclFile, filename, label)
						}
						typeMap = labelMap
					}
				}
			}
		}

		return result, nil
	}

	// Fallback: try JustAttributes if type assertion fails.
	attrs, diags := body.JustAttributes()
	if diags != nil && diags.HasErrors() {
		return nil, fmt.Errorf(errFmtProcessHCLFile, ErrFailedToProcessHclFile, filename, diags.Error())
	}
	for name, attr := range attrs {
		ctyValue, valDiags := attr.Expr.Value(evalCtx)
		if valDiags != nil && valDiags.HasErrors() {
			return nil, fmt.Errorf(errFmtProcessHCLFile, ErrFailedToProcessHclFile, filename, valDiags.Error())
		}
		result[name] = ctyToGo(ctyValue)
	}
	return result, nil
}

// isBlockRelatedDiagnostic checks if the diagnostics indicate blocks are present
// rather than genuine syntax errors like unexpected tokens or invalid expressions.
func isBlockRelatedDiagnostic(diags hcl.Diagnostics) bool {
	for _, diag := range diags {
		// HCL reports "Argument or block definition required" when blocks are present.
		// Other common block-related messages include mentions of "block" in the summary.
		summary := strings.ToLower(diag.Summary)
		detail := strings.ToLower(diag.Detail)

		// Block-related patterns.
		if strings.Contains(summary, "block") ||
			strings.Contains(detail, "block") ||
			strings.Contains(summary, "argument or block") {
			return true
		}
	}
	return false
}

// ParseHCLStacks parses an HCL file that may contain multiple stack blocks.
// Returns a slice of StackDocument, one for each stack block found.
// If no stack blocks are found, returns a single StackDocument with the entire file content.
func ParseHCLStacks(data []byte, filename string) ([]StackDocument, error) {
	defer perf.Track(nil, "filetype.ParseHCLStacks")()

	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL(data, filename)
	if diags != nil && diags.HasErrors() {
		return nil, fmt.Errorf(errFmtProcessHCLFile, ErrFailedToProcessHclFile, filename, diags.Error())
	}
	if file == nil {
		return nil, fmt.Errorf("%w, file: %s, file parsing returned nil", ErrFailedToProcessHclFile, filename)
	}

	return parseHCLBodyForStacks(file.Body, filename)
}

// parseHCLBodyForStacks parses an HCL body looking for stack blocks.
// Returns multiple StackDocuments if stack blocks are found.
// Complexity is inherent to recursive HCL structure traversal - refactoring deferred.
//
//nolint:cyclop,funlen,gocognit,nestif,revive
func parseHCLBodyForStacks(body hcl.Body, filename string) ([]StackDocument, error) {
	evalCtx := hclEvalContext()

	// Type assert to get the underlying hclsyntax.Body.
	syntaxBody, ok := body.(*hclsyntax.Body)
	if !ok {
		// Fallback to regular parsing if type assertion fails.
		result, err := parseHCLBody(body, filename)
		if err != nil {
			return nil, err
		}
		return []StackDocument{{Config: result}}, nil
	}

	// Collect top-level attributes (outside any stack block).
	topLevelAttrs := make(map[string]any)
	for name, attr := range syntaxBody.Attributes {
		ctyValue, valDiags := attr.Expr.Value(evalCtx)
		if valDiags != nil && valDiags.HasErrors() {
			return nil, fmt.Errorf("%w, file: %s, attribute %s error: %s",
				ErrFailedToProcessHclFile, filename, name, valDiags.Error())
		}
		topLevelAttrs[name] = ctyToGo(ctyValue)
	}

	// Find stack blocks and other blocks.
	var stackBlocks []*hclsyntax.Block
	var otherBlocks []*hclsyntax.Block
	for _, block := range syntaxBody.Blocks {
		if block.Type == stackBlockType {
			stackBlocks = append(stackBlocks, block)
		} else {
			otherBlocks = append(otherBlocks, block)
		}
	}

	// If no stack blocks found, treat entire file as single stack.
	if len(stackBlocks) == 0 {
		result, err := parseHCLBodyWithBlocks(body, filename)
		if err != nil {
			return nil, err
		}
		// Check for explicit name field.
		name := ""
		if n, ok := result[nameSectionName].(string); ok {
			name = n
		}
		return []StackDocument{{Name: name, Config: result}}, nil
	}

	// Process each stack block.
	var stacks []StackDocument
	for _, block := range stackBlocks {
		// Validate: stack blocks can have at most one label.
		if len(block.Labels) > 1 {
			return nil, fmt.Errorf("%w, file: %s, stack block cannot have more than one label",
				ErrFailedToProcessHclFile, filename)
		}

		// Parse the stack block content.
		stackConfig, err := parseHCLBodyWithBlocks(block.Body, filename)
		if err != nil {
			return nil, err
		}

		// Merge top-level attributes into stack config (stack block takes precedence).
		merged := make(map[string]any)
		for k, v := range topLevelAttrs {
			merged[k] = v
		}
		for k, v := range stackConfig {
			merged[k] = v
		}

		// Process other blocks (non-stack) and merge into this stack.
		for _, otherBlock := range otherBlocks {
			blockContent, err := parseHCLBodyWithBlocks(otherBlock.Body, filename)
			if err != nil {
				return nil, err
			}
			// Merge block content like the original parseHCLBodyWithBlocks does.
			if len(otherBlock.Labels) == 0 {
				if existing, ok := merged[otherBlock.Type]; ok {
					if existingMap, ok := existing.(map[string]any); ok {
						for k, v := range blockContent {
							existingMap[k] = v
						}
					} else {
						merged[otherBlock.Type] = blockContent
					}
				} else {
					merged[otherBlock.Type] = blockContent
				}
			} else {
				// Labeled blocks get nested.
				if _, ok := merged[otherBlock.Type]; !ok {
					merged[otherBlock.Type] = make(map[string]any)
				}
				typeMap, ok := merged[otherBlock.Type].(map[string]any)
				if !ok {
					return nil, fmt.Errorf("%w, file: %s, block type '%s' conflicts with existing attribute of different type",
						ErrFailedToProcessHclFile, filename, otherBlock.Type)
				}
				for i, label := range otherBlock.Labels {
					if i == len(otherBlock.Labels)-1 {
						typeMap[label] = blockContent
					} else {
						if _, ok := typeMap[label]; !ok {
							typeMap[label] = make(map[string]any)
						}
						nestedMap, ok := typeMap[label].(map[string]any)
						if !ok {
							return nil, fmt.Errorf("%w, file: %s, nested label '%s' conflicts with existing value of different type",
								ErrFailedToProcessHclFile, filename, label)
						}
						typeMap = nestedMap
					}
				}
			}
		}

		// Determine stack name: label takes precedence, then name field.
		stackName := ""
		if len(block.Labels) == 1 {
			stackName = block.Labels[0]
			// Also set the name field in the config.
			merged[nameSectionName] = stackName
		} else if n, ok := merged[nameSectionName].(string); ok {
			stackName = n
		}

		stacks = append(stacks, StackDocument{
			Name:   stackName,
			Config: merged,
		})
	}

	return stacks, nil
}

// ParseYAMLStacks parses a YAML file that may contain multiple documents (separated by ---).
// Returns a slice of StackDocument, one for each document found.
func ParseYAMLStacks(data []byte) ([]StackDocument, error) {
	defer perf.Track(nil, "filetype.ParseYAMLStacks")()

	decoder := yaml.NewDecoder(bytes.NewReader(data))

	var stacks []StackDocument
	for {
		var doc map[string]any
		err := decoder.Decode(&doc)
		if err != nil {
			// Check if we reached the end of the stream.
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("failed to parse YAML document: %w", err)
		}

		// Skip empty documents.
		if doc == nil {
			continue
		}

		// Extract name if present.
		name := ""
		if n, ok := doc[nameSectionName].(string); ok {
			name = n
		}

		stacks = append(stacks, StackDocument{
			Name:   name,
			Config: doc,
		})
	}

	// If no documents found, return empty slice.
	if len(stacks) == 0 {
		return []StackDocument{}, nil
	}

	return stacks, nil
}

// ctyToGo converts cty.Value to Go types.
func ctyToGo(value cty.Value) any {
	switch value.Type() {
	case cty.String:
		return value.AsString()
	case cty.Number:
		if n, _ := value.AsBigFloat().Int64(); true {
			return n
		}
		return value.AsBigFloat()
	case cty.Bool:
		return value.True()
	}

	if value.Type().IsObjectType() {
		m := map[string]any{}
		for k, v := range value.AsValueMap() {
			m[k] = ctyToGo(v)
		}
		return m
	}

	if value.Type().IsListType() || value.Type().IsTupleType() {
		var list []any
		for _, v := range value.AsValueSlice() {
			list = append(list, ctyToGo(v))
		}
		return list
	}

	return value
}
