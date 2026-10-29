package policy

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ErrorInput describes one failed invocation, not an entire component pipeline.
type ErrorInput struct {
	Event         string         `json:"event"`
	ComponentType string         `json:"component_type"`
	Command       string         `json:"command"`
	Component     string         `json:"component"`
	Stack         string         `json:"stack"`
	Stage         string         `json:"stage"`
	Attempt       int            `json:"attempt"`
	ExitCode      int            `json:"exit_code"`
	Output        string         `json:"output"`
	Stdout        string         `json:"stdout"`
	Stderr        string         `json:"stderr"`
	Error         ExecutionError `json:"error"`
}

// ExecutionError contains SDK or subprocess error facts without exposing arguments.
type ExecutionError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

var errorEventPattern = regexp.MustCompile(`^[a-z][a-z0-9_/-]*\.([a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)*|\*)\.error$`)

// IsErrorEvent accepts concrete error events and a component's subcommand wildcard.
//
//nolint:lintroller // Pure event validation.
func IsErrorEvent(kind InputKind) bool {
	return errorEventPattern.MatchString(string(kind))
}

// MatchesKind selects a rule once even when several of its event selectors match.
//
//nolint:lintroller // Pure selector lookup.
func (r *Rule) MatchesKind(kind InputKind) bool {
	if matchesEvent(r.On, kind) {
		return true
	}
	for _, event := range r.Events {
		if matchesEvent(event, kind) {
			return true
		}
	}
	return false
}

func matchesEvent(selector, event InputKind) bool {
	if selector == event {
		return true
	}
	if !IsErrorEvent(selector) || !IsErrorEvent(event) || strings.Contains(string(event), "*") {
		return false
	}
	component, command, _ := strings.Cut(string(selector), ".")
	return command == "*.error" && strings.HasPrefix(string(event), component+".")
}

// ErrorEvent normalizes aliases to the same canonical names as lifecycle hooks.
// Custom providers use their registered component name and actual subcommand.
//
//nolint:lintroller // Pure alias normalization.
func ErrorEvent(componentType, command string) InputKind {
	command = strings.Join(strings.Fields(command), ".")
	if alias := errorCommandAliases[componentType][command]; alias != "" {
		command = alias
	}
	if componentType == "terraform" {
		command = strings.ReplaceAll(command, "workspace-", "workspace.")
	}
	return InputKind(componentType + "." + command + ".error")
}

var errorCommandAliases = map[string]map[string]string{
	"terraform":          {"deploy": "apply"},
	"helmfile":           {"sync": "apply", "deploy": "apply"},
	"helm":               {"deploy": "apply", "plan": "diff", "render": "template", "destroy": "delete"},
	"kubernetes":         {"deploy": "apply", "plan": "diff"},
	"aws/cloudformation": {"deploy": "apply", "plan": "diff"},
}

type eventSelectors []InputKind

//nolint:lintroller // Pure YAML validation and decoding.
func (s *eventSelectors) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		*s = []InputKind{InputKind(node.Value)}
		return nil
	}
	if node.Kind != yaml.SequenceNode || len(node.Content) == 0 {
		return fmt.Errorf("%w: on must be a string or nonempty list of error events", errUtils.ErrPolicyInvalid)
	}
	for _, child := range node.Content {
		if child.Kind != yaml.ScalarNode || child.Tag != "!!str" || !IsErrorEvent(InputKind(child.Value)) {
			return fmt.Errorf("%w: on entries must be component.subcommand.error selectors", errUtils.ErrPolicyInvalid)
		}
		*s = append(*s, InputKind(child.Value))
	}
	return nil
}

func (p *planPolicyItem) resolveEvents() error {
	if len(p.Events) == 0 {
		return nil
	}
	p.On = p.Events[0]
	if len(p.Events) == 1 && !IsErrorEvent(p.On) {
		p.Events = nil
		return nil
	}
	for i, event := range p.Events {
		if !IsErrorEvent(event) {
			return fmt.Errorf("%w: event lists require component.subcommand.error selectors", errUtils.ErrPolicyInvalid)
		}
		component, rest, _ := strings.Cut(string(event), ".")
		p.Events[i] = ErrorEvent(component, strings.TrimSuffix(rest, ".error"))
	}
	p.On = p.Events[0]
	return nil
}

func (p *planPolicyItem) resolveRetry() (*schema.RetryConfig, error) {
	if p.Action != "retry" {
		return nil, nil
	}
	if _, exists := p.With["conditions"]; exists {
		return nil, fmt.Errorf("%w: use rule, not with.conditions, to select retryable errors", errUtils.ErrPolicyInvalid)
	}
	config, err := schema.DecodeRetryConfig(p.With)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrPolicyInvalid, err)
	}
	if config == nil || config.MaxAttempts == nil {
		return nil, fmt.Errorf("%w: retry policies require with.max_attempts", errUtils.ErrPolicyInvalid)
	}
	if err := retry.Validate(config); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrPolicyInvalid, err)
	}
	switch config.BackoffStrategy {
	case "", schema.BackoffConstant, schema.BackoffLinear, schema.BackoffExponential:
	default:
		return nil, fmt.Errorf("%w: unknown backoff_strategy", errUtils.ErrPolicyInvalid)
	}
	return config, nil
}
