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

	// UnwrapScalar=false preserves YAML type information for scalar values.
	// When true, yq strips surrounding quotes (e.g. `"true"` becomes bare `true`), causing
	// the downstream YAML parser to lose the original Go type: a string "true" would be
	// decoded as bool true, a string "42" as int 42, and so on.
	// Setting UnwrapScalar=false keeps the yq encoder's quoting intact so that the
	// YAML round-trip correctly reconstructs the original types.
	pref := yqlib.YamlPreferences{
		Indent:                      2,
		ColorsEnabled:               false,
		LeadingContentPreProcessing: true,
		PrintDocSeparators:          true,
		UnwrapScalar:                false,
		EvaluateTogether:            false,
	}

	encoder := yqlib.NewYamlEncoder(pref)
	decoder := yqlib.NewYamlDecoder(pref)

	result, err := evaluator.Evaluate(yq, yamlData, encoder, decoder)
	if err != nil {
		return nil, fmt.Errorf("EvaluateYqExpression: failed to evaluate YQ expression '%s': %w", yq, err)
	}

	trimmedResult := strings.TrimSpace(result)

	// isScalarString and isMisinterpretedScalar are safety-net guards that were
	// necessary when UnwrapScalar=true caused the YAML parser to misinterpret scalar
	// strings (e.g., ARNs or IPv6 addresses ending with "::").  With UnwrapScalar=false
	// yq now emits properly quoted scalars so these cases are handled by the standard
	// YAML round-trip below.
	//
	// Important: with PrintDocSeparators=true the output always starts with "---\n",
	// making trimmedResult a multi-line string for all non-empty results.  Both
	// isScalarString and isMisinterpretedScalar return false for multi-line input, so
	// these branches are structurally unreachable under the current configuration.
	// They are kept as a defensive fallback in case the output format changes.
	if isScalarString(trimmedResult) {
		return trimmedResult, nil
	}

	var node yaml.Node
	err = yaml.Unmarshal([]byte(result), &node)
	if err != nil {
		return nil, fmt.Errorf("EvaluateYqExpression: failed to unmarshal result: %w", err)
	}

	// Defensive fallback: detect strings the YAML parser might still misinterpret as maps.
	if isMisinterpretedScalar(&node, trimmedResult) {
		return trimmedResult, nil
	}

	processYAMLNode(&node)

	// Thread the caller's atmosConfig through processCustomTags so that custom
	// Atmos YAML tags embedded in the yq output are resolved with the correct
	// configuration rather than a zero-value config.  When atmosConfig is nil
	// (e.g., in unit tests), fall back to an empty config to satisfy the non-nil
	// contract of processCustomTags.
	cfg := atmosConfig
	if cfg == nil {
		cfg = &schema.AtmosConfiguration{}
	}

	if err := processCustomTags(cfg, &node, ""); err != nil {
		return nil, fmt.Errorf("EvaluateYqExpression: failed to process custom tags: %w", err)
	}

	// Decode directly from the processed yaml.Node, avoiding an unnecessary
	// intermediate marshal/unmarshal round-trip.
	var res any
	if err := node.Decode(&res); err != nil {
		return nil, fmt.Errorf("EvaluateYqExpression: failed to decode YAML node: %w", err)
	}

	return res, nil
}

// isScalarString was the primary guard when UnwrapScalar=true was used in
// EvaluateYqExpression. It detected scalar strings that the downstream YAML
// parser would misinterpret (e.g., "#comment" stripped as a comment, or
// "arn:...::password::" misread as a YAML map).
//
// Note: with the current configuration (PrintDocSeparators=true,
// UnwrapScalar=false), yq always emits a document-separator line ("---\n")
// before the value, so trimmedResult always contains "\n" for non-empty
// output. Because every isScalarString branch either requires the absence of
// "\n" (the colon-suffix and "#"-prefix paths) or explicitly rejects
// multi-line input, this function returns false for all normal yq output
// under the current settings. It is retained as a defensive fallback in case
// the yq output format changes or a future caller uses different preferences.
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
// A node is null when it carries the !!null tag, or when its value is empty and
// the tag is not !!str (an explicit empty string is a valid non-null value).
func isYAMLNullValue(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && (node.Tag == "!!null" || (node.Value == "" && node.Tag != "!!str"))
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
//
// Note: with UnwrapScalar=false, yq quotes colon-suffixed strings (e.g., "arn:...::") so they
// are parsed as ScalarNodes by yaml.Unmarshal, not MappingNodes.  As a result, this function
// returns false for all normal yq output under the current configuration.  It is retained as
// a defensive fallback for unexpected edge cases.
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

	// UnwrapScalar=true is intentional here: the result is decoded into a strongly-typed
	// Go struct T (e.g., schema.AtmosConfiguration).  For struct decoding, YAML type
	// coercion is desirable — the yaml.v3 decoder correctly maps bare `true`/`false` to
	// bool fields, bare integers to int fields, and so on.  Preserving quotes (as done in
	// EvaluateYqExpression) is only necessary when the return type is `any`, where the
	// decoder must infer the Go type from the YAML tag.
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
