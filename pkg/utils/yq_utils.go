// https://github.com/mikefarah/yq
// https://mikefarah.gitbook.io/yq
// https://mikefarah.gitbook.io/yq/recipes
// https://mikefarah.gitbook.io/yq/operators/pipe

package utils

import (
	"fmt"
	"strings"
	"sync"

	"github.com/mikefarah/yq/v4/pkg/yqlib"
	logging "gopkg.in/op/go-logging.v1"
	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// yqLogModule is the go-logging module name that yq registers its internal
// logger under.
const yqLogModule = "yq-lib"

// yqSilentLevel is lower than yq's critical level, so its IsEnabledFor()
// gate rejects every message. Using a level keeps logger configuration
// reversible across repeated configureYqLogger calls.
const yqSilentLevel logging.Level = -1

// yqLogLevelMu serializes Atmos reads and writes of go-logging's module
// level registry. That registry is a plain map with no synchronization of
// its own, and yq reads it from every decoder it runs, so an unguarded
// write from one goroutine while another goroutine is decoding ends the
// process with a runtime fatal error that recover cannot intercept.
var yqLogLevelMu sync.Mutex

// yqParserOnce guards yq's process-global expression parser.
var yqParserOnce sync.Once

// init installs the default yq log level before main runs, and therefore
// before any goroutine exists. A non-Trace run then finds the level it
// wants already installed and never writes the registry while stack
// processing goroutines are live.
func init() {
	logging.SetLevel(yqSilentLevel, yqLogModule)
}

// initYqExpressionParser initializes yq's process-global expression parser
// exactly once. The yqlib.InitExpressionParser function guards
// yqlib.ExpressionParser with a plain nil check and every Evaluate call runs
// it, so concurrent first evaluations otherwise write and read that global
// at the same time.
// Doing it under a Once gives the later readers a happens-before edge, and
// keeps the one-time cost (~0.4ms here) off commands that never evaluate a
// yq expression.
func initYqExpressionParser() {
	yqParserOnce.Do(yqlib.InitExpressionParser)
}

// configureYqLogger configures the yq logger based on Atmos configuration.
// Non-Trace log levels suppress yq's internal diagnostics; Trace
// restores yq's default verbosity (Debug) so users asking for
// everything see everything.
func configureYqLogger(atmosConfig *schema.AtmosConfiguration) {
	defer perf.Track(atmosConfig, "utils.configureYqLogger")()

	desired := yqSilentLevel
	if atmosConfig != nil && atmosConfig.Logs.Level == LogLevelTrace {
		desired = logging.DEBUG
	}
	setYqLogLevel(desired)
}

// setYqLogLevel installs level for yq's logger, writing only when some
// other level is currently installed. Callers may run concurrently.
//
// Skipping the redundant write is what makes this safe rather than merely
// tidy: yq's own reads of the level registry are outside our control, so
// the only way to keep them from racing a write is to not write at all
// once the wanted level is in place.
func setYqLogLevel(level logging.Level) {
	yqLogLevelMu.Lock()
	defer yqLogLevelMu.Unlock()

	if logging.GetLevel(yqLogModule) == level {
		return
	}
	logging.SetLevel(level, yqLogModule)
}

func EvaluateYqExpression(atmosConfig *schema.AtmosConfiguration, data any, yq string) (any, error) {
	defer perf.Track(atmosConfig, "utils.EvaluateYqExpression")()

	// Configure the yq logger based on Atmos configuration
	configureYqLogger(atmosConfig)
	initYqExpressionParser()

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
	if isScalarString(trimmedResult) {
		return trimmedResult, nil
	}

	var node yaml.Node
	err = yaml.Unmarshal([]byte(result), &node)
	if err != nil {
		if !strings.Contains(trimmedResult, "\n") {
			return trimmedResult, nil
		}
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
		if !strings.Contains(trimmedResult, "\n") {
			return trimmedResult, nil
		}
		return nil, fmt.Errorf("EvaluateYqExpression: failed to convert YAML to Go type: %w", err)
	}

	return res, nil
}

// isScalarString checks if the yq result appears to be a simple scalar string value
// that should not be parsed as YAML. This handles edge cases where the YAML parser
// would misinterpret the string (e.g., strings ending with colons).
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
// use at most :: as a separator. Triple or more colons are not matched intentionally.
func keyMatchesOriginalWithColon(key, original string) bool {
	return key+":" == original || key+"::" == original
}

// isMisinterpretedScalar checks if the YAML parser has misinterpreted a scalar string as a map.
// This happens when a string ends with colons (e.g., "value:" or "value::") which YAML
// interprets as a map key with a null value.
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

// processYAMLNode walks a YAML node tree and adjusts the style of scalar
// string nodes that start with `#` so they round-trip without YAML
// re-interpreting them as comments.
//
// The perf.Track defer is on this entry point only; the recursive worker
// (processYAMLNodeInner) deliberately omits it to avoid inflating the
// metric across every tree node — see processCustomTags /
// processCustomTagsInner for the same pattern.
func processYAMLNode(node *yaml.Node) {
	defer perf.Track(nil, "utils.processYAMLNode")()
	processYAMLNodeInner(node)
}

// processYAMLNodeInner is the recursive worker for processYAMLNode. No
// perf.Track here; the outer call wraps the whole walk with one tracked
// frame.
func processYAMLNodeInner(node *yaml.Node) {
	if node == nil {
		return
	}

	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" && strings.HasPrefix(node.Value, "#") {
		node.Style = yaml.SingleQuotedStyle
	}

	for _, child := range node.Content {
		processYAMLNodeInner(child)
	}
}

func EvaluateYqExpressionWithType[T any](atmosConfig *schema.AtmosConfiguration, data T, yq string) (*T, error) {
	defer perf.Track(atmosConfig, "utils.EvaluateYqExpressionWithType")()

	// Configure the yq logger based on Atmos configuration
	configureYqLogger(atmosConfig)
	initYqExpressionParser()

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
