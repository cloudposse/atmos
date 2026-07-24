package validator

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/pkg/datafetcher"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// DeprecatedField is a schema-declared compatibility field found in authored YAML.
// Path uses the same dot/bracket notation as validation errors and YAML positions.
type DeprecatedField struct {
	Path        string
	Replacement string
}

// DeprecatedYAMLSchema is a parsed schema that can be reused to scan multiple
// YAML documents for deprecated fields.
type DeprecatedYAMLSchema struct {
	document map[string]any
}

// FindDeprecatedYAMLFields finds authored properties marked with the standard
// JSON Schema deprecated annotation. x-atmos-replacement is optional guidance
// emitted alongside the warning. The scanner deliberately follows only local
// references: remote schemas are still validated normally, but must be a single
// self-contained document before their annotations can be inspected.
func FindDeprecatedYAMLFields(atmosConfig *schema.AtmosConfiguration, schemaSource string, yamlContent []byte) ([]DeprecatedField, error) {
	defer perf.Track(atmosConfig, "validator.FindDeprecatedYAMLFields")()

	deprecatedSchema, err := LoadDeprecatedYAMLSchema(atmosConfig, schemaSource)
	if err != nil {
		return nil, err
	}
	return deprecatedSchema.FindYAMLFields(yamlContent)
}

// LoadDeprecatedYAMLSchema loads and parses a schema for reuse when scanning
// multiple YAML documents.
func LoadDeprecatedYAMLSchema(atmosConfig *schema.AtmosConfiguration, schemaSource string) (*DeprecatedYAMLSchema, error) {
	defer perf.Track(atmosConfig, "validator.LoadDeprecatedYAMLSchema")()

	schemaData, err := datafetcher.NewDataFetcher(atmosConfig).GetData(schemaSource)
	if err != nil {
		return nil, fmt.Errorf("fetch deprecation schema %q: %w", schemaSource, err)
	}
	var schemaDoc map[string]any
	if err := json.Unmarshal(schemaData, &schemaDoc); err != nil {
		return nil, fmt.Errorf("decode deprecation schema %q: %w", schemaSource, err)
	}
	return &DeprecatedYAMLSchema{document: schemaDoc}, nil
}

// FindYAMLFields returns deprecated fields in yamlContent using the parsed
// schema. The scanner follows only local references.
func (s *DeprecatedYAMLSchema) FindYAMLFields(yamlContent []byte) ([]DeprecatedField, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(yamlContent, &node); err != nil {
		return nil, fmt.Errorf("decode YAML for deprecation scan: %w", err)
	}
	if len(node.Content) == 0 {
		return nil, nil
	}

	findings := make(map[string]DeprecatedField)
	walkDeprecatedSchema(s.document, s.document, node.Content[0], "", findings)
	result := make([]DeprecatedField, 0, len(findings))
	for path, finding := range findings {
		if hasMoreSpecificDeprecatedField(path, findings) {
			continue
		}
		result = append(result, finding)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

func hasMoreSpecificDeprecatedField(path string, findings map[string]DeprecatedField) bool {
	for candidate := range findings {
		if strings.HasPrefix(candidate, path+".") || strings.HasPrefix(candidate, path+"[") {
			return true
		}
	}
	return false
}

var schemaArrayIndex = regexp.MustCompile(`\[(\d+)\]`)

func walkDeprecatedSchema(root, current map[string]any, node *yaml.Node, path string, findings map[string]DeprecatedField) {
	current = resolveLocalRef(root, current)
	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		for _, branch := range schemaArray(current[key]) {
			walkDeprecatedSchema(root, branch, node, path, findings)
		}
	}

	switch node.Kind {
	case yaml.MappingNode:
		properties, _ := current["properties"].(map[string]any)
		patterns, _ := current["patternProperties"].(map[string]any)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i].Value, node.Content[i+1]
			propertySchema, ok := properties[key].(map[string]any)
			if !ok {
				for pattern, candidate := range patterns {
					matched, err := regexp.MatchString(pattern, key)
					if err == nil && matched {
						propertySchema, ok = candidate.(map[string]any)
						break
					}
				}
			}
			if !ok {
				propertySchema, ok = current["additionalProperties"].(map[string]any)
			}
			if !ok {
				continue
			}
			propertyPath := joinSchemaPath(path, key)
			if deprecated, replacement := deprecatedSchemaAnnotation(root, propertySchema); deprecated {
				findings[propertyPath] = DeprecatedField{Path: propertyPath, Replacement: replacement}
			}
			propertySchema = resolveLocalRef(root, propertySchema)
			walkDeprecatedSchema(root, propertySchema, value, propertyPath, findings)
		}
	case yaml.SequenceNode:
		items, _ := current["items"].(map[string]any)
		for i, child := range node.Content {
			walkDeprecatedSchema(root, items, child, fmt.Sprintf("%s[%d]", path, i), findings)
		}
	}
}

func deprecatedSchemaAnnotation(root, current map[string]any) (bool, string) {
	if deprecated, _ := current["deprecated"].(bool); deprecated {
		replacement, _ := current["x-atmos-replacement"].(string)
		return true, replacement
	}
	current = resolveLocalRef(root, current)
	if deprecated, _ := current["deprecated"].(bool); deprecated {
		replacement, _ := current["x-atmos-replacement"].(string)
		return true, replacement
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		for _, branch := range schemaArray(current[key]) {
			if deprecated, replacement := deprecatedSchemaAnnotation(root, branch); deprecated {
				return true, replacement
			}
		}
	}
	return false, ""
}

func resolveLocalRef(root, current map[string]any) map[string]any {
	ref, _ := current["$ref"].(string)
	if ref == "" || ref[0] != '#' {
		return current
	}
	var value any = root
	for _, part := range splitJSONPointer(ref[1:]) {
		object, ok := value.(map[string]any)
		if !ok {
			return current
		}
		value = object[part]
	}
	if resolved, ok := value.(map[string]any); ok {
		return resolved
	}
	return current
}

func splitJSONPointer(pointer string) []string {
	if pointer == "" {
		return nil
	}
	parts := make([]string, 0)
	for _, part := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		parts = append(parts, strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~"))
	}
	return parts
}

func schemaArray(value any) []map[string]any {
	values, _ := value.([]any)
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if object, ok := value.(map[string]any); ok {
			result = append(result, object)
		}
	}
	return result
}

func joinSchemaPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// NormalizeSchemaPath converts dot-number paths emitted by gojsonschema into
// the position-map format used by the YAML helpers. It is exported for CLI and
// LSP adapters that combine schema errors and deprecation warnings.
func NormalizeSchemaPath(path string) string {
	return schemaArrayIndex.ReplaceAllString(path, "[$1]")
}

// FormatDeprecatedField renders a deprecation finding as Markdown. A trailing
// parenthetical in the replacement is explanatory prose, not part of the YAML
// path, so it intentionally remains outside inline-code delimiters.
func FormatDeprecatedField(field DeprecatedField) string {
	message := fmt.Sprintf("`%s` is deprecated", field.Path)
	if field.Replacement == "" {
		return message
	}
	replacement, note := field.Replacement, ""
	if index := strings.Index(replacement, " ("); index >= 0 && strings.HasSuffix(replacement, ")") {
		note = replacement[index:]
		replacement = replacement[:index]
	}
	return fmt.Sprintf("%s; use `%s`%s", message, replacement, note)
}
