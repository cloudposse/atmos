package condition

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/go-viper/mapstructure/v2"
	"github.com/google/cel-go/cel"
)

const (
	// PredicateCI matches when Atmos is running in a detected CI environment.
	PredicateCI = "ci"
	// PredicateLocal matches when Atmos is not running in a detected CI environment.
	PredicateLocal = "local"
	// PredicateAlways always matches.
	PredicateAlways = "always"
	// PredicateNever never matches.
	PredicateNever = "never"
	// PredicateSuccess matches a successful lifecycle status.
	PredicateSuccess = "success"
	// PredicateFailure matches a failed lifecycle status.
	PredicateFailure = "failure"

	CELTag = "!cel"
)

// ErrInvalidWhenCondition is returned when a `when` value cannot be normalized or evaluated.
var ErrInvalidWhenCondition = errors.New("invalid when condition")

// Context carries runtime facts used to evaluate a declarative `when`.
type Context struct {
	CI        bool
	Status    string
	Stack     string
	Component string
	Workflow  string
	Step      string
	Hook      string
	Event     string
	Env       map[string]string
}

// PredicateFunc evaluates a named condition predicate against runtime facts.
type PredicateFunc func(Context) bool

var (
	predicatesMu sync.RWMutex
	predicates   = map[string]PredicateFunc{
		PredicateCI:      func(ctx Context) bool { return ctx.CI },
		PredicateLocal:   func(ctx Context) bool { return !ctx.CI },
		PredicateAlways:  func(Context) bool { return true },
		PredicateNever:   func(Context) bool { return false },
		PredicateSuccess: func(ctx Context) bool { return ctx.Status == PredicateSuccess },
		PredicateFailure: func(ctx Context) bool { return ctx.Status == PredicateFailure },
	}

	celEnvOnce sync.Once
	celEnv     *cel.Env
	celEnvErr  error
)

// RegisterPredicate adds or replaces a named condition predicate.
//
//nolint:lintroller // This package cannot import perf because schema aliases condition.
func RegisterPredicate(name string, fn PredicateFunc) {
	predicatesMu.Lock()
	defer predicatesMu.Unlock()
	predicates[strings.ToLower(strings.TrimSpace(name))] = fn
}

// Condition is the normalized representation of a step/hook `when` value.
type Condition struct {
	node *Node
}

// Node is a small AST for declarative conditions.
type Node struct {
	Kind     string `json:"kind,omitempty"`
	Name     string `json:"name,omitempty"`
	Expr     string `json:"expr,omitempty"`
	Children []Node `json:"children,omitempty"`

	celAst *cel.Ast
}

const (
	kindPredicate = "predicate"
	kindCEL       = "cel"
	kindAll       = "all"
	kindAny       = "any"
	kindNot       = "not"
)

// UnmarshalYAML supports scalar, list, and object forms for `when`.
//
//nolint:lintroller // This package cannot import perf because schema aliases condition.
func (c *Condition) UnmarshalYAML(unmarshal func(any) error) error {
	var value any
	if err := unmarshal(&value); err != nil {
		return err
	}
	condition, err := New(value)
	if err != nil {
		return err
	}
	*c = condition
	return nil
}

// MarshalJSON preserves conditions when command configs are cloned through JSON.
//
//nolint:lintroller // This package cannot import perf because schema aliases condition.
func (c Condition) MarshalJSON() ([]byte, error) {
	if c.node == nil {
		return []byte("null"), nil
	}
	return json.Marshal(c.node.value())
}

// UnmarshalJSON supports JSON config files and internal command cloning.
//
//nolint:lintroller // This package cannot import perf because schema aliases condition.
func (c *Condition) UnmarshalJSON(data []byte) error {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	condition, err := New(value)
	if err != nil {
		return err
	}
	*c = condition
	return nil
}

// New normalizes a decoded YAML/mapstructure value.
//
//nolint:lintroller // This package cannot import perf because schema aliases condition.
func New(value any) (Condition, error) {
	node, err := normalize(value)
	if err != nil {
		return Condition{}, err
	}
	return Condition{node: node}, nil
}

// Must is a test/helper convenience for constructing conditions.
//
//nolint:lintroller // This package cannot import perf because schema aliases condition.
func Must(value any) Condition {
	condition, err := New(value)
	if err != nil {
		panic(err)
	}
	return condition
}

func normalize(value any) (*Node, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case Condition:
		return v.node, nil
	case string:
		return normalizeString(v)
	case []string:
		return normalizeStringSlice(v)
	case []any:
		return normalizeSlice(v)
	case map[string]any:
		return normalizeMap(v)
	case map[any]any:
		return normalizeAnyMap(v)
	default:
		return nil, fmt.Errorf("%w: expected string, list, or map, got %T", ErrInvalidWhenCondition, value)
	}
}

func normalizeString(value string) (*Node, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return nil, nil
	}

	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, CELTag) {
		return normalizeCEL(strings.TrimSpace(text[len(CELTag):]))
	}
	if isPredicate(lower) {
		return &Node{Kind: kindPredicate, Name: lower}, nil
	}
	return normalizeCEL(text)
}

func normalizeCEL(expr string) (*Node, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("%w: %s requires an expression", ErrInvalidWhenCondition, CELTag)
	}
	ast, err := compileCEL(expr)
	if err != nil {
		return nil, err
	}
	return &Node{Kind: kindCEL, Expr: expr, celAst: ast}, nil
}

func normalizeStringSlice(values []string) (*Node, error) {
	children := make([]Node, 0, len(values))
	for _, item := range values {
		child, err := normalize(item)
		if err != nil {
			return nil, err
		}
		if child != nil {
			children = append(children, *child)
		}
	}
	return &Node{Kind: kindAll, Children: children}, nil
}

func normalizeSlice(values []any) (*Node, error) {
	children := make([]Node, 0, len(values))
	for _, item := range values {
		child, err := normalize(item)
		if err != nil {
			return nil, err
		}
		if child != nil {
			children = append(children, *child)
		}
	}
	return &Node{Kind: kindAll, Children: children}, nil
}

func normalizeAnyMap(values map[any]any) (*Node, error) {
	converted := make(map[string]any, len(values))
	for key, item := range values {
		converted[fmt.Sprint(key)] = item
	}
	return normalizeMap(converted)
}

func normalizeMap(values map[string]any) (*Node, error) {
	if len(values) != 1 {
		return nil, fmt.Errorf("%w: expected exactly one of all, any, or not", ErrInvalidWhenCondition)
	}

	for key, value := range values {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		normalize, ok := operator(normalizedKey)
		if !ok {
			return nil, fmt.Errorf("%w: unknown operator %q", ErrInvalidWhenCondition, key)
		}
		return normalize(value)
	}
	return nil, nil
}

func operator(kind string) (func(any) (*Node, error), bool) {
	switch kind {
	case kindAll:
		return normalizeAll, true
	case kindAny:
		return normalizeAny, true
	case kindNot:
		return normalizeNot, true
	default:
		return nil, false
	}
}

func normalizeAll(value any) (*Node, error) {
	children, err := normalizeChildren(value)
	if err != nil {
		return nil, err
	}
	return &Node{Kind: kindAll, Children: children}, nil
}

func normalizeAny(value any) (*Node, error) {
	children, err := normalizeChildren(value)
	if err != nil {
		return nil, err
	}
	return &Node{Kind: kindAny, Children: children}, nil
}

func normalizeNot(value any) (*Node, error) {
	child, err := normalize(value)
	if err != nil {
		return nil, err
	}
	if child == nil {
		return nil, fmt.Errorf("%w: not requires a condition", ErrInvalidWhenCondition)
	}
	return &Node{Kind: kindNot, Children: []Node{*child}}, nil
}

func normalizeChildren(value any) ([]Node, error) {
	node, err := normalize(value)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, nil
	}
	if node.Kind == kindAll && node.Name == "" {
		return node.Children, nil
	}
	return []Node{*node}, nil
}

// DecodeHook lets Viper/mapstructure decode string/list/map values into Condition fields.
//
//nolint:lintroller // This package cannot import perf because schema aliases condition.
func DecodeHook() mapstructure.DecodeHookFunc {
	conditionType := reflect.TypeOf(Condition{})
	return func(_ reflect.Type, t reflect.Type, data any) (any, error) {
		if t != conditionType {
			return data, nil
		}
		return New(data)
	}
}

func isPredicate(name string) bool {
	predicatesMu.RLock()
	defer predicatesMu.RUnlock()
	return predicates[name] != nil
}
