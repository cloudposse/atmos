// Package yq provides utilities for evaluating YQ expressions on YAML data.
// https://github.com/mikefarah/yq
// https://mikefarah.gitbook.io/yq
// https://mikefarah.gitbook.io/yq/recipes
// https://mikefarah.gitbook.io/yq/operators/pipe
package yq

import (
	"fmt"
	"strings"

	"github.com/mikefarah/yq/v4/pkg/yqlib"
	"gopkg.in/op/go-logging.v1"
	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// LogLevelTrace is the trace log level used to determine verbosity.
const LogLevelTrace = "Trace"

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
	defer perf.Track(atmosConfig, "yq.configureYqLogger")()

	// Only use the default (chatty) logger when atmosConfig is not nil and log level is Trace.
	// In all other cases, use the no-op logging backend.
	if atmosConfig == nil || atmosConfig.Logs.Level != LogLevelTrace {
		logger := yqlib.GetLogger()
		backend := logBackend{}
		logger.SetBackend(backend)
	}
}

// EvaluateExpression evaluates a YQ expression against the provided data.
func EvaluateExpression(atmosConfig *schema.AtmosConfiguration, data any, expression string) (any, error) {
	defer perf.Track(atmosConfig, "yq.EvaluateExpression")()

	// Configure the yq logger based on Atmos configuration.
	configureYqLogger(atmosConfig)

	evaluator := yqlib.NewStringEvaluator()

	yamlData, err := convertToYAML(data)
	if err != nil {
		return nil, fmt.Errorf("EvaluateExpression: failed to convert data to YAML: %w", err)
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

	result, err := evaluator.Evaluate(expression, yamlData, encoder, decoder)
	if err != nil {
		return nil, fmt.Errorf("EvaluateExpression: failed to evaluate YQ expression '%s': %w", expression, err)
	}

	trimmedResult := strings.TrimSpace(result)
	if isSimpleStringStartingWithHash(trimmedResult) {
		return trimmedResult, nil
	}

	var node yaml.Node
	err = yaml.Unmarshal([]byte(result), &node)
	if err != nil {
		return nil, fmt.Errorf("EvaluateExpression: failed to unmarshal result: %w", err)
	}

	processYAMLNode(&node)
	resultBytes, err := yaml.Marshal(&node)
	if err != nil {
		return nil, fmt.Errorf("EvaluateExpression: failed to marshal processed node: %w", err)
	}

	res, err := unmarshalYAML[any](string(resultBytes))
	if err != nil {
		return nil, fmt.Errorf("EvaluateExpression: failed to convert YAML to Go type: %w", err)
	}

	return res, nil
}

// EvaluateExpressionWithType evaluates a YQ expression and returns a typed result.
func EvaluateExpressionWithType[T any](atmosConfig *schema.AtmosConfiguration, data T, expression string) (*T, error) {
	defer perf.Track(atmosConfig, "yq.EvaluateExpressionWithType")()

	// Configure the yq logger based on Atmos configuration.
	configureYqLogger(atmosConfig)

	evaluator := yqlib.NewStringEvaluator()

	yamlData, err := convertToYAML(data)
	if err != nil {
		return nil, fmt.Errorf("EvaluateExpressionWithType: failed to convert data to YAML: %w", err)
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

	result, err := evaluator.Evaluate(expression, yamlData, encoder, decoder)
	if err != nil {
		return nil, fmt.Errorf("EvaluateExpressionWithType: failed to evaluate YQ expression '%s': %w", expression, err)
	}

	res, err := unmarshalYAML[T](result)
	if err != nil {
		return nil, fmt.Errorf("EvaluateExpressionWithType: failed to convert YAML to Go type: %w", err)
	}

	return &res, nil
}

func isSimpleStringStartingWithHash(s string) bool {
	return strings.HasPrefix(s, "#") && !strings.Contains(s, "\n")
}

func processYAMLNode(node *yaml.Node) {
	defer perf.Track(nil, "yq.processYAMLNode")()

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

// convertToYAML converts any Go type to a YAML string.
func convertToYAML(data any) (string, error) {
	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("convertToYAML: failed to marshal to YAML: %w", err)
	}
	return string(yamlBytes), nil
}

// unmarshalYAML unmarshals a YAML string to a typed Go value.
func unmarshalYAML[T any](yamlStr string) (T, error) {
	var result T
	err := yaml.Unmarshal([]byte(yamlStr), &result)
	if err != nil {
		return result, fmt.Errorf("unmarshalYAML: failed to unmarshal YAML: %w", err)
	}
	return result, nil
}
