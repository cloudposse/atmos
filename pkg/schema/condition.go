package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/go-viper/mapstructure/v2"
)

const (
	ConditionPredicateCI      = "ci"
	ConditionPredicateLocal   = "local"
	ConditionPredicateAlways  = "always"
	ConditionPredicateNever   = "never"
	ConditionPredicateSuccess = "success"
	ConditionPredicateFailure = "failure"
)

var ErrInvalidWhenCondition = errors.New("invalid when condition")

// ConditionContext carries runtime facts used to evaluate a declarative `when`.
type ConditionContext struct {
	CI     bool
	Status string
}

type ConditionPredicateFunc func(ConditionContext) bool

var (
	conditionPredicatesMu sync.RWMutex
	conditionPredicates   = map[string]ConditionPredicateFunc{
		ConditionPredicateCI:      func(ctx ConditionContext) bool { return ctx.CI },
		ConditionPredicateLocal:   func(ctx ConditionContext) bool { return !ctx.CI },
		ConditionPredicateAlways:  func(ConditionContext) bool { return true },
		ConditionPredicateNever:   func(ConditionContext) bool { return false },
		ConditionPredicateSuccess: func(ctx ConditionContext) bool { return ctx.Status == ConditionPredicateSuccess },
		ConditionPredicateFailure: func(ctx ConditionContext) bool { return ctx.Status == ConditionPredicateFailure },
	}
)

// RegisterConditionPredicate adds or replaces a named condition predicate.
func RegisterConditionPredicate(name string, fn ConditionPredicateFunc) {
	conditionPredicatesMu.Lock()
	defer conditionPredicatesMu.Unlock()
	conditionPredicates[strings.ToLower(strings.TrimSpace(name))] = fn
}

// Condition is the normalized representation of a step/hook `when` value.
type Condition struct {
	node *ConditionNode
}

// ConditionNode is a small AST for declarative conditions.
type ConditionNode struct {
	Kind     string          `json:"kind,omitempty"`
	Name     string          `json:"name,omitempty"`
	Children []ConditionNode `json:"children,omitempty"`
}

const (
	conditionKindPredicate = "predicate"
	conditionKindAll       = "all"
	conditionKindAny       = "any"
	conditionKindNot       = "not"
)

// UnmarshalYAML supports scalar, list, and object forms for `when`.
func (c *Condition) UnmarshalYAML(unmarshal func(any) error) error {
	var value any
	if err := unmarshal(&value); err != nil {
		return err
	}
	condition, err := NewCondition(value)
	if err != nil {
		return err
	}
	*c = condition
	return nil
}

// MarshalJSON preserves conditions when command configs are cloned through JSON.
func (c Condition) MarshalJSON() ([]byte, error) {
	if c.node == nil {
		return []byte("null"), nil
	}
	return json.Marshal(c.node.value())
}

// UnmarshalJSON supports JSON config files and internal command cloning.
func (c *Condition) UnmarshalJSON(data []byte) error {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	condition, err := NewCondition(value)
	if err != nil {
		return err
	}
	*c = condition
	return nil
}

// NewCondition normalizes a decoded YAML/mapstructure value.
func NewCondition(value any) (Condition, error) {
	node, err := normalizeCondition(value)
	if err != nil {
		return Condition{}, err
	}
	return Condition{node: node}, nil
}

// MustCondition is a test/helper convenience for constructing conditions.
func MustCondition(value any) Condition {
	condition, err := NewCondition(value)
	if err != nil {
		panic(err)
	}
	return condition
}

func normalizeCondition(value any) (*ConditionNode, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case Condition:
		return v.node, nil
	case string:
		return normalizeConditionString(v), nil
	case []string:
		return normalizeConditionStringSlice(v)
	case []any:
		return normalizeConditionSlice(v)
	case map[string]any:
		return normalizeConditionMap(v)
	case map[any]any:
		return normalizeConditionAnyMap(v)
	default:
		return nil, fmt.Errorf("%w: expected string, list, or map, got %T", ErrInvalidWhenCondition, value)
	}
}

func normalizeConditionString(value string) *ConditionNode {
	name := strings.ToLower(strings.TrimSpace(value))
	if name == "" {
		return nil
	}
	return &ConditionNode{Kind: conditionKindPredicate, Name: name}
}

func normalizeConditionStringSlice(values []string) (*ConditionNode, error) {
	children := make([]ConditionNode, 0, len(values))
	for _, item := range values {
		child, err := normalizeCondition(item)
		if err != nil {
			return nil, err
		}
		if child != nil {
			children = append(children, *child)
		}
	}
	return &ConditionNode{Kind: conditionKindAll, Children: children}, nil
}

func normalizeConditionSlice(values []any) (*ConditionNode, error) {
	children := make([]ConditionNode, 0, len(values))
	for _, item := range values {
		child, err := normalizeCondition(item)
		if err != nil {
			return nil, err
		}
		if child != nil {
			children = append(children, *child)
		}
	}
	return &ConditionNode{Kind: conditionKindAll, Children: children}, nil
}

func normalizeConditionAnyMap(values map[any]any) (*ConditionNode, error) {
	converted := make(map[string]any, len(values))
	for key, item := range values {
		converted[fmt.Sprint(key)] = item
	}
	return normalizeConditionMap(converted)
}

func normalizeConditionMap(values map[string]any) (*ConditionNode, error) {
	if len(values) != 1 {
		return nil, fmt.Errorf("%w: expected exactly one of all, any, or not", ErrInvalidWhenCondition)
	}

	for key, value := range values {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		normalize, ok := conditionOperator(normalizedKey)
		if !ok {
			return nil, fmt.Errorf("%w: unknown operator %q", ErrInvalidWhenCondition, key)
		}
		return normalize(value)
	}
	return nil, nil
}

func conditionOperator(kind string) (func(any) (*ConditionNode, error), bool) {
	switch kind {
	case conditionKindAll:
		return normalizeConditionAll, true
	case conditionKindAny:
		return normalizeConditionAny, true
	case conditionKindNot:
		return normalizeConditionNot, true
	default:
		return nil, false
	}
}

func normalizeConditionAll(value any) (*ConditionNode, error) {
	children, err := normalizeConditionChildren(value)
	if err != nil {
		return nil, err
	}
	return &ConditionNode{Kind: conditionKindAll, Children: children}, nil
}

func normalizeConditionAny(value any) (*ConditionNode, error) {
	children, err := normalizeConditionChildren(value)
	if err != nil {
		return nil, err
	}
	return &ConditionNode{Kind: conditionKindAny, Children: children}, nil
}

func normalizeConditionNot(value any) (*ConditionNode, error) {
	child, err := normalizeCondition(value)
	if err != nil {
		return nil, err
	}
	if child == nil {
		return nil, fmt.Errorf("%w: not requires a condition", ErrInvalidWhenCondition)
	}
	return &ConditionNode{Kind: conditionKindNot, Children: []ConditionNode{*child}}, nil
}

func normalizeConditionChildren(value any) ([]ConditionNode, error) {
	node, err := normalizeCondition(value)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, nil
	}
	if node.Kind == conditionKindAll && node.Name == "" {
		return node.Children, nil
	}
	return []ConditionNode{*node}, nil
}

// IsZero reports whether no condition was configured.
func (c Condition) IsZero() bool {
	return c.node == nil
}

// Evaluate returns whether the condition matches the supplied context. Empty
// conditions match by default.
func (c Condition) Evaluate(ctx ConditionContext) bool {
	if c.node == nil {
		return true
	}
	return c.node.Evaluate(ctx)
}

// EvaluateWithImplicitSuccess applies the hook-specific default: a condition
// that does not mention lifecycle status also requires success.
func (c Condition) EvaluateWithImplicitSuccess(ctx ConditionContext) bool {
	if c.node == nil {
		return ctx.Status == ConditionPredicateSuccess
	}
	if !c.MentionsAny(ConditionPredicateSuccess, ConditionPredicateFailure, ConditionPredicateAlways) && ctx.Status != ConditionPredicateSuccess {
		return false
	}
	return c.Evaluate(ctx)
}

// MentionsAny reports whether any predicate with one of the supplied names is
// present in the condition tree.
func (c Condition) MentionsAny(names ...string) bool {
	if c.node == nil {
		return false
	}
	wanted := make(map[string]struct{}, len(names))
	for _, name := range names {
		wanted[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	return c.node.mentionsAny(wanted)
}

func (n ConditionNode) Evaluate(ctx ConditionContext) bool {
	switch n.Kind {
	case conditionKindPredicate:
		return n.evaluatePredicate(ctx)
	case conditionKindAll:
		return n.evaluateAll(ctx)
	case conditionKindAny:
		return n.evaluateAny(ctx)
	case conditionKindNot:
		return len(n.Children) == 1 && !n.Children[0].Evaluate(ctx)
	default:
		return false
	}
}

func (n ConditionNode) evaluatePredicate(ctx ConditionContext) bool {
	conditionPredicatesMu.RLock()
	fn := conditionPredicates[n.Name]
	conditionPredicatesMu.RUnlock()
	if fn == nil {
		return false
	}
	return fn(ctx)
}

func (n ConditionNode) evaluateAll(ctx ConditionContext) bool {
	for _, child := range n.Children {
		if !child.Evaluate(ctx) {
			return false
		}
	}
	return true
}

func (n ConditionNode) evaluateAny(ctx ConditionContext) bool {
	for _, child := range n.Children {
		if child.Evaluate(ctx) {
			return true
		}
	}
	return false
}

func (n ConditionNode) mentionsAny(names map[string]struct{}) bool {
	if n.Kind == conditionKindPredicate {
		_, ok := names[n.Name]
		return ok
	}
	for _, child := range n.Children {
		if child.mentionsAny(names) {
			return true
		}
	}
	return false
}

func (n ConditionNode) value() any {
	switch n.Kind {
	case conditionKindPredicate:
		return n.Name
	case conditionKindAll:
		values := make([]any, 0, len(n.Children))
		for _, child := range n.Children {
			values = append(values, child.value())
		}
		return map[string]any{conditionKindAll: values}
	case conditionKindAny:
		values := make([]any, 0, len(n.Children))
		for _, child := range n.Children {
			values = append(values, child.value())
		}
		return map[string]any{conditionKindAny: values}
	case conditionKindNot:
		if len(n.Children) == 1 {
			return map[string]any{conditionKindNot: n.Children[0].value()}
		}
	}
	return nil
}

// ConditionDecodeHook lets Viper/mapstructure decode string/list/map values
// into Condition fields.
func ConditionDecodeHook() mapstructure.DecodeHookFunc {
	conditionType := reflect.TypeOf(Condition{})
	return func(_ reflect.Type, t reflect.Type, data any) (any, error) {
		if t != conditionType {
			return data, nil
		}
		return NewCondition(data)
	}
}
