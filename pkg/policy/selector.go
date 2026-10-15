package policy

import (
	"encoding/json"
	"fmt"
	"slices"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/tags"
)

// Subject identifies one instance and its resolved, inherited selector metadata.
// A nil subject means unavailable, not an instance with no labels.
type Subject struct {
	ObjectIdentity
	Metadata SubjectMetadata `json:"metadata"`
	// TemplateLeft identifies unresolved metadata under a configured delimiter.
	TemplateLeft string `json:"template_left,omitempty"`
}

// SubjectMetadata contains only data used for attachment, never executable configuration.
type SubjectMetadata struct {
	Tags   []string          `json:"tags"`
	Labels map[string]string `json:"labels"`
}

// Selector attaches a rule to instances. Fields combine with AND; label entries
// all match. Multiple tags use the same any/all convention as component selection.
type Selector struct {
	Tags           []string          `json:"tags,omitempty" yaml:"tags"`
	TagMode        tags.TagMode      `json:"tag_mode,omitempty" yaml:"-"`
	Labels         map[string]string `json:"labels,omitempty" yaml:"labels"`
	Components     []string          `json:"components,omitempty" yaml:"components"`
	Stacks         []string          `json:"stacks,omitempty" yaml:"stacks"`
	ComponentTypes []string          `json:"component_types,omitempty" yaml:"component_types"`
}

// NewSubject copies already resolved metadata without evaluating templates or functions.
// Malformed and unresolved metadata must not silently detach a policy.
func NewSubject(identity ObjectIdentity, metadata any, delimiters ...string) (*Subject, error) {
	defer perf.Track(nil, "policy.NewSubject")()

	data, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("%w: instance metadata: %w", errUtils.ErrPolicyInputInvalid, err)
	}
	subject := &Subject{ObjectIdentity: identity}
	if len(delimiters) > 0 {
		subject.TemplateLeft = delimiters[0]
	}
	if err := json.Unmarshal(data, &subject.Metadata); err != nil {
		return nil, fmt.Errorf("%w: instance tags and labels must be strings: %w", errUtils.ErrPolicyInputInvalid, err)
	}
	if subject.Metadata.Tags == nil {
		subject.Metadata.Tags = []string{}
	}
	if subject.Metadata.Labels == nil {
		subject.Metadata.Labels = map[string]string{}
	}
	return subject, nil
}

func (s *Selector) validate() error {
	if s == nil {
		return nil
	}
	if s.TagMode != "" && s.TagMode != tags.TagModeAny && s.TagMode != tags.TagModeAll {
		return fmt.Errorf("%w: tag matching mode must be any or all", errUtils.ErrPolicyInvalid)
	}
	if unresolvedMetadata(SubjectMetadata{Tags: s.Tags, Labels: s.Labels}, "{{") {
		return fmt.Errorf("%w: attachment selectors must be literal values", errUtils.ErrPolicyInvalid)
	}
	return nil
}

// Matches applies an attachment selector to exactly one snapshot. Change callers
// must test before and after separately, not union their tags or labels.
func (s *Selector) Matches(subject *Subject) (bool, error) {
	defer perf.Track(nil, "policy.Selector.Matches")()

	if s == nil {
		return true, nil
	}
	if err := s.validate(); err != nil {
		return false, err
	}
	if err := validateSubject(subject); err != nil {
		return false, err
	}
	return matchesNames(s.Components, subject.Name) && matchesNames(s.Stacks, subject.Stack) &&
		matchesNames(s.ComponentTypes, subject.ComponentType) &&
		tags.MatchesTags(subject.Metadata.Tags, s.Tags, s.TagMode) && tags.MatchesLabels(subject.Metadata.Labels, s.Labels), nil
}

func validateSubject(subject *Subject) error {
	if subject == nil || subject.Name == "" || subject.Stack == "" || subject.ComponentType == "" {
		return fmt.Errorf("%w: attachment requires an instance identity and metadata", errUtils.ErrPolicyInputUnavailable)
	}
	if unresolvedMetadata(subject.Metadata, subject.TemplateLeft) {
		return fmt.Errorf("%w: attachment metadata is unresolved", errUtils.ErrPolicyInputUnavailable)
	}
	return nil
}

func matchesNames(names []string, value string) bool {
	return len(names) == 0 || slices.Contains(names, value)
}

func unresolvedMetadata(metadata SubjectMetadata, leftDelimiter string) bool {
	for _, tag := range metadata.Tags {
		if tags.SelectorUnresolved(tag, leftDelimiter) {
			return true
		}
	}
	for key, value := range metadata.Labels {
		if tags.SelectorUnresolved(key, leftDelimiter) || tags.SelectorUnresolved(value, leftDelimiter) {
			return true
		}
	}
	return false
}
