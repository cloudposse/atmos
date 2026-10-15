package utils

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/function/parser"
)

// handleCILabelsTag preserves sequence arguments as a scalar for post-merge evaluation.
// The generic deferral only retains node.Value, which is empty on sequence nodes.
func handleCILabelsTag(_ TagContext, node *yaml.Node, val string) (bool, error) {
	args := val
	switch node.Kind {
	case yaml.ScalarNode:
		// Scalar arguments are validated by the same parser as the function registry.
	case yaml.SequenceNode:
		sequence := *node
		sequence.Tag = "!!seq"
		encoded, err := yaml.Marshal(&sequence)
		if err != nil {
			return false, fmt.Errorf("%w: !ci.labels: %w", errUtils.ErrInvalidArguments, err)
		}
		args = string(encoded)
	default:
		return false, fmt.Errorf("%w: !ci.labels requires no arguments or a nonempty list of strings", errUtils.ErrInvalidArguments)
	}
	labels, err := parser.ParseCILabels(args)
	if err != nil {
		return false, err
	}
	if labels != nil {
		// JSON is a YAML flow sequence and preserves quoting, whitespace, and punctuation.
		encoded, err := json.Marshal(labels)
		if err != nil {
			return false, fmt.Errorf("%w: !ci.labels: %w", errUtils.ErrInvalidArguments, err)
		}
		args = string(encoded)
	}
	node.Kind = yaml.ScalarNode
	node.Tag = "!!str"
	node.Value = strings.TrimSpace(AtmosYamlFuncCILabels + " " + args)
	node.Content = nil
	return true, nil
}
