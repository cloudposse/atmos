// https://github.com/mikefarah/yq
// https://mikefarah.gitbook.io/yq
// https://mikefarah.gitbook.io/yq/recipes
// https://mikefarah.gitbook.io/yq/operators/pipe

package utils

import (
	"fmt"
	"strings"

	"github.com/mikefarah/yq/v4/pkg/yqlib"
	"gopkg.in/op/go-logging.v1"
	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

type logBackend struct{}

func (n logBackend) Log(level logging.Level, i int, record *logging.Record) error {
	return nil
}

func (n logBackend) GetLevel(s string) logging.Level {
	return logging.ERROR
}

func (n logBackend) SetLevel(level logging.Level, s string) {
}

func (n logBackend) IsEnabledFor(level logging.Level, s string) bool {
	return false
}

// configureYqLogger configures the yq logger based on Atmos configuration.
// If atmosConfig is nil or log level is not Trace, use a no-op logging backend.
func configureYqLogger(atmosConfig *schema.AtmosConfiguration) {
	defer perf.Track(atmosConfig, "utils.configureYqLogger")()

	// Only use the default (chatty) logger when atmosConfig is not nil and log level is Trace
	// In all other cases, use the no-op logging backend
	if atmosConfig == nil || atmosConfig.Logs.Level != LogLevelTrace {
		logger := yqlib.GetLogger()
		backend := logBackend{}
		logger.SetBackend(backend)
	}
}

func EvaluateYqExpression(atmosConfig *schema.AtmosConfiguration, data any, yq string) (any, error) {
	defer perf.Track(atmosConfig, "utils.EvaluateYqExpression")()

	// Configure the yq logger based on Atmos configuration
	configureYqLogger(atmosConfig)

	evaluator := yqlib.NewStringEvaluator()

	yamlData, err := ConvertToYAML(data)
	if err != nil {
		return nil, fmt.Errorf("EvaluateYqExpression: failed to convert data to YAML: %w", err)
	}

	pref := yqlib.YamlPreferences{
		Indent:                      2,
		ColorsEnabled:               false,
		LeadingContentPreProcessing: true,
		PrintDocSeparators:          true,
		UnwrapScalar:                true,
		EvaluateTogether:            false,
	}

	encoder := yqlib.NewYamlEncoder(pref)
	decoder := yqlib.NewYamlDecoder(pref)

	result, err := evaluator.Evaluate(yq, yamlData, encoder, decoder)
	if err != nil {
		return nil, fmt.Errorf("EvaluateYqExpression: failed to evaluate YQ expression '%s': %w", yq, err)
	}

	trimmedResult := strings.TrimSpace(result)

	// Handle scalar strings that could be misinterpreted by the YAML parser.
	// When yq returns a scalar with UnwrapScalar=true, special characters like trailing
	// colons can cause the YAML parser to misinterpret the value as a map.
	// E.g., "arn:aws:secretsmanager:...::password::" would become {"password:": null}.
	// This also covers IPv6 addresses ending in "::" (e.g., "2041:0000:140F::875B::"),
	// which were similarly misinterpreted as {"2041:0000:140F::875B:": null}.
	// Fix introduced in v1.206.0 (PR #2059); regression guard added in #2155.
	if isScalarString(trimmedResult) {
		return trimmedResult, nil
	}

	var node yaml.Node
	err = yaml.Unmarshal([]byte(result), &node)
	if err != nil {
		return nil, fmt.Errorf("EvaluateYqExpression: failed to unmarshal result: %w", err)
	}

	// Check if the YAML parser misinterpreted a scalar string as a map.
	// This happens when the string contains colons that look like YAML map syntax.
	if isMisinterpretedScalar(&node, trimmedResult) {
		return trimmedResult, nil
	}

	processYAMLNode(&node)
	resultBytes, err := yaml.Marshal(&node)
	if err != nil {
		return nil, fmt.Errorf("EvaluateYqExpression: failed to marshal processed node: %w", err)
	}

	res, err := UnmarshalYAML[any](string(resultBytes))
	if err != nil {
		return nil, fmt.Errorf("EvaluateYqExpression: failed to convert YAML to Go type: %w", err)
	}

	return res, nil
}

// isScalarString checks if the yq result appears to be a simple scalar string value
// that should not be parsed as YAML. This handles edge cases where the YAML parser
// would misinterpret the string as a map because it ends with one or more colons.
//
// Known patterns covered:
//   - AWS ARNs: "arn:aws:secretsmanager:...:password::" → would become {"password:": null}
//   - IPv6 addresses ending in "::": "2041:0000:140F::875B::" → would become {"2041:0000:140F::875B:": null}
//   - Any single-line string ending with ":" or "::" that contains no ": " (YAML map) pattern.
//
// This fix was introduced in v1.206.0 (PR #2059) for AWS ARNs and also covers IPv6 (#2155).
func isScalarString(s string) bool {
	// Handle strings starting with # (comments would be stripped by YAML parser).
	if strings.HasPrefix(s, "#") && !strings.Contains(s, "\n") {
		return true
	}
	// Empty strings should go through YAML parsing which converts them to nil.
	// This is the expected behavior for YQ default value expressions.
	if s == "" {
		return false
	}
	// Check for YAML flow syntax (maps or arrays).
	if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		return false
	}
	// Check for multi-line content (could be a YAML document).
	if strings.Contains(s, "\n") {
		return false
	}
	// Single-line strings ending with colons that don't have ": " pattern
	// are likely scalar values (like ARNs) that would be misinterpreted as maps.
	if strings.HasSuffix(s, ":") {
		if !strings.Contains(s, ": ") {
			return true
		}
	}
	return false
}

// isYAMLNullValue checks if a YAML node represents a null value.
func isYAMLNullValue(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && (node.Value == "" || node.Tag == "!!null")
}

// keyMatchesOriginalWithColon checks if the key plus trailing colon(s) matches the original string.
// Only single (:) and double (::) colon suffixes are handled, as real-world values like AWS ARNs
// and IPv6 addresses use at most :: as a separator. Triple or more colons are not matched intentionally.
func keyMatchesOriginalWithColon(key, original string) bool {
	return key+":" == original || key+"::" == original
}

// isMisinterpretedScalar checks if the YAML parser has misinterpreted a scalar string as a map.
// This happens when a string ends with colons (e.g., "value:" or "value::") which YAML
// interprets as a map key with a null value.
// Affected patterns include AWS ARNs (e.g., "arn:aws:...:password::") and IPv6 addresses
// ending with "::" (e.g., "2041:0000:140F::875B::"). Fix introduced in v1.206.0 (PR #2059).
func isMisinterpretedScalar(node *yaml.Node, originalResult string) bool {
	// Navigate to document content if this is a document node.
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}
	// Only check mapping nodes with exactly one key-value pair.
	if node.Kind != yaml.MappingNode || len(node.Content) != 2 {
		return false
	}
	keyNode := node.Content[0]
	valueNode := node.Content[1]
	// Check if this is a misinterpreted scalar: null value and key matches original with colon.
	if !isYAMLNullValue(valueNode) || keyNode.Kind != yaml.ScalarNode {
		return false
	}
	return keyMatchesOriginalWithColon(keyNode.Value, originalResult)
}

func processYAMLNode(node *yaml.Node) {
	defer perf.Track(nil, "utils.processYAMLNode")()

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

func EvaluateYqExpressionWithType[T any](atmosConfig *schema.AtmosConfiguration, data T, yq string) (*T, error) {
	defer perf.Track(atmosConfig, "utils.EvaluateYqExpressionWithType")()

	// Configure the yq logger based on Atmos configuration
	configureYqLogger(atmosConfig)

	evaluator := yqlib.NewStringEvaluator()

	yaml, err := ConvertToYAML(data)
	if err != nil {
		return nil, fmt.Errorf("EvaluateYqExpressionWithType: failed to convert data to YAML: %w", err)
	}

	pref := yqlib.YamlPreferences{
		Indent:                      2,
		ColorsEnabled:               false,
		LeadingContentPreProcessing: true,
		PrintDocSeparators:          true,
		UnwrapScalar:                true,
		EvaluateTogether:            false,
	}

	encoder := yqlib.NewYamlEncoder(pref)
	decoder := yqlib.NewYamlDecoder(pref)

	result, err := evaluator.Evaluate(yq, yaml, encoder, decoder)
	if err != nil {
		return nil, fmt.Errorf("EvaluateYqExpressionWithType: failed to evaluate YQ expression '%s': %w", yq, err)
	}

	res, err := UnmarshalYAML[T](result)
	if err != nil {
		return nil, fmt.Errorf("EvaluateYqExpressionWithType: failed to convert YAML to Go type: %w", err)
	}

	return &res, nil
}
