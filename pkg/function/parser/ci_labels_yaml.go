package parser

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	fntag "github.com/cloudposse/atmos/pkg/function/tag"
	"github.com/cloudposse/atmos/pkg/perf"
)

// EncodeCILabels validates tagged YAML arguments and encodes a deferred function expression.
// Sequence arguments must be preserved explicitly because their node.Value is empty.
func EncodeCILabels(node *yaml.Node) (string, error) {
	defer perf.Track(nil, "parser.EncodeCILabels")()

	args := strings.TrimSpace(node.Value)
	switch node.Kind {
	case yaml.ScalarNode:
		// Only the bare form is a scalar; a quoted or plain scalar such as "[preview]" would be
		// re-parsed as a list by ParseCILabels, hiding that the YAML argument is not a sequence.
		if args != "" {
			return "", fmt.Errorf("%w: !ci.labels requires no arguments or a nonempty list of strings, got the scalar %q", errUtils.ErrInvalidArguments, args)
		}
	case yaml.SequenceNode:
		sequence := *node
		sequence.Tag = "!!seq"
		encoded, err := yaml.Marshal(&sequence)
		if err != nil {
			return "", fmt.Errorf("%w: !ci.labels: %w", errUtils.ErrInvalidArguments, err)
		}
		args = string(encoded)
	default:
		return "", fmt.Errorf("%w: !ci.labels requires no arguments or a nonempty list of strings", errUtils.ErrInvalidArguments)
	}
	labels, err := ParseCILabels(args)
	if err != nil {
		return "", err
	}
	if labels != nil {
		// JSON is a YAML flow sequence and preserves quoting, whitespace, and punctuation.
		encoded, err := json.Marshal(labels)
		if err != nil {
			return "", fmt.Errorf("%w: !ci.labels: %w", errUtils.ErrInvalidArguments, err)
		}
		args = string(encoded)
	}
	return strings.TrimSpace("!" + fntag.CILabels + " " + args), nil
}
