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
	if isSimpleStringStartingWithHash(trimmedResult) {
		return trimmedResult, nil
	}
	var node yaml.Node
	err = yaml.Unmarshal([]byte(result), &node)
	if err != nil {
		return nil, fmt.Errorf("EvaluateYqExpression: failed to unmarshal result: %w", err)
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

func isSimpleStringStartingWithHash(s string) bool {
	return strings.HasPrefix(s, "#") && !strings.Contains(s, "\n")
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
