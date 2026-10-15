package parser

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ParseCILabels accepts no arguments (list mode) or a nonempty YAML sequence of literal strings.
// A nil result distinguishes list mode from membership matching.
func ParseCILabels(input string) ([]string, error) {
	defer perf.Track(nil, "parser.ParseCILabels")()

	if strings.TrimSpace(input) == "" {
		return nil, nil
	}
	decoder := yaml.NewDecoder(strings.NewReader(input))
	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%w: !ci.labels requires a nonempty list of strings: %w", errUtils.ErrInvalidArguments, err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: !ci.labels expects a single list", errUtils.ErrInvalidArguments)
	}
	if len(doc.Content) != 1 {
		return nil, fmt.Errorf("%w: !ci.labels requires a nonempty list of strings", errUtils.ErrInvalidArguments)
	}
	return parseCILabelsSequence(doc.Content[0])
}

func parseCILabelsSequence(sequence *yaml.Node) ([]string, error) {
	if sequence.Kind != yaml.SequenceNode || sequence.Tag != "!!seq" || len(sequence.Content) == 0 {
		return nil, fmt.Errorf("%w: !ci.labels requires a nonempty list of strings", errUtils.ErrInvalidArguments)
	}
	labels := make([]string, 0, len(sequence.Content))
	for _, item := range sequence.Content {
		if item.Kind != yaml.ScalarNode || item.Tag != "!!str" {
			return nil, fmt.Errorf("%w: !ci.labels labels must be literal strings", errUtils.ErrInvalidArguments)
		}
		labels = append(labels, item.Value)
	}
	return labels, nil
}
