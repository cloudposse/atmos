package cloudformation

import (
	"context"
	"fmt"
	"strings"

	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

// presentedStackOutputs keeps redaction at the presentation boundary. Dependency
// resolution continues to use describeStackOutputs and receives the original values.
func presentedStackOutputs(ctx context.Context, client CloudFormationClient, stackName string) (map[string]any, error) {
	outputs, parameters, err := describeStackOutputValues(ctx, client, stackName)
	if err != nil || len(outputs) == 0 || !iolib.MaskingEnabled() {
		return outputs, err
	}
	body, err := getDeployedTemplate(ctx, client, stackName, false)
	if err != nil {
		return nil, fmt.Errorf("cannot safely display outputs: reading deployed NoEcho metadata requires cloudformation:GetTemplate: %w", err)
	}
	return maskStackOutputs(body, parameters, outputs)
}

type outputSensitivityTemplate struct {
	Parameters map[string]struct {
		NoEcho  any    `yaml:"NoEcho"`
		Default string `yaml:"Default"`
	} `yaml:"Parameters"`
	Resources  map[string]yaml.Node `yaml:"Resources"`
	Conditions map[string]yaml.Node `yaml:"Conditions"`
	Outputs    map[string]yaml.Node `yaml:"Outputs"`
}

func maskStackOutputs(body string, parameters []cfntypes.Parameter, outputs map[string]any) (map[string]any, error) {
	doc, err := parseOutputSensitivity(body)
	if err != nil {
		return nil, err
	}

	sensitive := registerOutputSecrets(doc, parameters)
	// Resource attributes and conditions may depend indirectly on a NoEcho value.
	// Propagate sensitivity to a fixed point; over-redaction is safer than leaking it.
	for changed := true; changed; {
		changed = propagateOutputSensitivity(doc.Resources, sensitive)
		changed = propagateOutputSensitivity(doc.Conditions, sensitive) || changed
	}
	masker := iolib.GetContext().Masker()
	presented := make(map[string]any, len(outputs))
	for key, value := range outputs {
		expression, known := doc.Outputs[key]
		if !known || nodeReferencesSensitive(&expression, sensitive, make(map[*yaml.Node]bool)) {
			presented[key] = masker.Replacement()
		} else {
			presented[key] = masker.Mask(fmt.Sprint(value))
		}
	}
	return presented, nil
}

func registerOutputSecrets(doc *outputSensitivityTemplate, parameters []cfntypes.Parameter) map[string]bool {
	sensitive := make(map[string]bool)
	for name, parameter := range doc.Parameters {
		if isTruthy(parameter.NoEcho) {
			sensitive[name] = true
			iolib.RegisterSecretValue(parameter.Default)
		}
	}
	for _, parameter := range parameters {
		if sensitive[stringValue(parameter.ParameterKey)] {
			value := stringValue(parameter.ParameterValue)
			if value != "****" {
				iolib.RegisterSecretValue(value)
			}
		}
	}
	return sensitive
}

func propagateOutputSensitivity(nodes map[string]yaml.Node, sensitive map[string]bool) bool {
	changed := false
	for name := range nodes {
		node := nodes[name]
		if !sensitive[name] && nodeReferencesSensitive(&node, sensitive, make(map[*yaml.Node]bool)) {
			sensitive[name] = true
			changed = true
		}
	}
	return changed
}

// Inspect both short YAML tags and long JSON intrinsics without evaluating them.
// Literal identifiers are conservatively treated as references, including Sub's
// interpolations and GetAtt's dotted syntax. Alias cycles cannot recurse forever.
func nodeReferencesSensitive(node *yaml.Node, sensitive map[string]bool, seen map[*yaml.Node]bool) bool {
	if node == nil || seen[node] {
		return false
	}
	seen[node] = true
	for name := range sensitive {
		if referencesIdentifier(node.Value, name) {
			return true
		}
	}
	if nodeReferencesSensitive(node.Alias, sensitive, seen) {
		return true
	}
	for _, child := range node.Content {
		if nodeReferencesSensitive(child, sensitive, seen) {
			return true
		}
	}
	return false
}

func parseOutputSensitivity(body string) (*outputSensitivityTemplate, error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(body), &root); err != nil {
		// Parser diagnostics can contain template literals. Do not echo them.
		return nil, fmt.Errorf("%w: cannot safely display outputs: deployed template is not valid YAML/JSON", errUtils.ErrInvalidAwsCloudFormationSettings)
	}
	var doc outputSensitivityTemplate
	if len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%w: cannot safely display outputs: deployed template must be an object", errUtils.ErrInvalidAwsCloudFormationSettings)
	}
	if err := root.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%w: cannot safely display outputs: invalid deployed sensitivity metadata", errUtils.ErrInvalidAwsCloudFormationSettings)
	}
	return &doc, nil
}

func referencesIdentifier(value, name string) bool {
	return value == name || strings.HasPrefix(value, name+".") || strings.Contains(value, "${"+name+"}") || strings.Contains(value, "${"+name+".")
}
