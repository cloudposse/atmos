package secrets

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	fnparser "github.com/cloudposse/atmos/pkg/function/parser"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// SelectorEvaluator evaluates the raw value found at path inside a component section (for example
// ["secrets", "vars", "API_KEY", "store"]) and returns the evaluated value. It is how a declaration
// whose `store:`, `sops:` or provider definition is a YAML-function selector (such as
// `!aws.cloudformation.output <component> <stack> <output>`) is turned into a concrete backend.
//
// The evaluator is supplied by the caller because evaluation needs the stack-processing and
// authentication machinery that pkg/secrets must not depend on: the `atmos secret` CLI resolves a
// selector with its authenticated scope, and the `!secret` YAML function resolves it with the
// processing context of the component being described.
type SelectorEvaluator func(path []string, raw any) (any, error)

// Option configures a Service or a single Resolve call.
type Option func(*options)

type options struct {
	evaluator SelectorEvaluator
}

// WithSelectorEvaluator supplies the evaluator used to resolve backend selectors lazily, only for
// the declarations that are actually used. Results are memoized per path.
func WithSelectorEvaluator(evaluator SelectorEvaluator) Option {
	defer perf.Track(nil, "secrets.WithSelectorEvaluator")()

	return func(o *options) {
		o.evaluator = memoizeSelectorEvaluator(evaluator)
	}
}

func applyOptions(opts []Option) options {
	var o options
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}

// memoizeSelectorEvaluator caches successful evaluations by path so a command that touches the same
// declaration repeatedly (status, then set) evaluates its selector once. Failures are not cached.
func memoizeSelectorEvaluator(evaluator SelectorEvaluator) SelectorEvaluator {
	if evaluator == nil {
		return nil
	}
	var mu sync.Mutex
	cache := make(map[string]any)
	return func(path []string, raw any) (any, error) {
		key := strings.Join(path, "\x00")
		mu.Lock()
		cached, ok := cache[key]
		mu.Unlock()
		if ok {
			return cached, nil
		}
		value, err := evaluator(path, raw)
		if err != nil {
			return nil, err
		}
		mu.Lock()
		cache[key] = value
		mu.Unlock()
		return value, nil
	}
}

// IsSelector reports whether a declaration's `store:`/`sops:` value is a YAML-function selector
// (such as `!aws.cloudformation.output ...`) that must be evaluated before it names a backend.
func IsSelector(value string) bool {
	defer perf.Track(nil, "secrets.IsSelector")()

	return providers.IsSelector(value)
}

// containsSelector reports whether any string inside a (possibly nested) value is a selector.
func containsSelector(value any) bool {
	switch v := value.(type) {
	case string:
		return IsSelector(v)
	case map[string]any:
		for _, child := range v {
			if containsSelector(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if containsSelector(child) {
				return true
			}
		}
	}
	return false
}

// providerRequest carries what is needed to turn a declaration into a provider, including the
// evaluation context for lazily resolving backend selectors.
type providerRequest struct {
	atmosConfig *schema.AtmosConfiguration
	section     map[string]any
	stack       string
	component   string
	evaluator   SelectorEvaluator
}

// provider resolves the backend provider for a declaration. A selector in the declaration (or in
// the SOPS provider definition it names) is evaluated here, for this declaration only, so one
// unresolvable selector never affects a sibling declaration.
func (r *providerRequest) provider(decl *Declaration) (providers.Provider, error) {
	if decl.BackendType == "" {
		return nil, ErrNoBackend
	}
	name := decl.BackendName
	if IsSelector(name) {
		resolved, err := r.backendName(decl)
		if err != nil {
			return nil, err
		}
		name = resolved
	}
	sectionProviders := ExtractProviders(r.section)
	if decl.BackendType == BackendSops {
		resolved, err := r.providerDefinitions(decl, sectionProviders, name)
		if err != nil {
			return nil, err
		}
		sectionProviders = resolved
	}
	return providers.New(r.atmosConfig, string(decl.BackendType), name, sectionProviders)
}

// backendName evaluates the declaration's `store:`/`sops:` selector to a concrete backend name.
func (r *providerRequest) backendName(decl *Declaration) (string, error) {
	field := string(decl.BackendType)
	path := []string{secretsSectionKey, varsSectionKey, decl.Name, field}
	value, err := r.evaluate(path, decl.BackendName)
	if err != nil {
		return "", r.unresolved(decl.Name, field, decl.BackendName, err)
	}
	name, ok := value.(string)
	if !ok || strings.TrimSpace(name) == "" || IsSelector(name) {
		return "", r.unresolved(decl.Name, field, decl.BackendName, fmt.Errorf("%w (got %v)", ErrSelectorResult, value))
	}
	return strings.TrimSpace(name), nil
}

// providerDefinitions evaluates selectors inside the named SOPS provider definition (for example a
// `spec.file` produced by a CloudFormation output) and returns a provider map with the evaluated
// definition substituted. The input map is never mutated.
func (r *providerRequest) providerDefinitions(decl *Declaration, sectionProviders map[string]any, name string) (map[string]any, error) {
	def, ok := sectionProviders[name]
	if !ok || !containsSelector(def) {
		return sectionProviders, nil
	}
	path := []string{secretsSectionKey, providersSectionKey, name}
	value, err := r.evaluate(path, def)
	if err != nil {
		return nil, r.unresolved(decl.Name, providersSectionKey+"."+name, selectorText(def), err)
	}
	evaluated, ok := value.(map[string]any)
	if !ok || containsSelector(evaluated) {
		return nil, r.unresolved(decl.Name, providersSectionKey+"."+name, selectorText(def), fmt.Errorf("%w (got %v)", ErrSelectorResult, value))
	}
	out := make(map[string]any, len(sectionProviders))
	for k, v := range sectionProviders {
		out[k] = v
	}
	out[name] = evaluated
	return out, nil
}

// evaluate runs the selector evaluator, or reports that none is available.
func (r *providerRequest) evaluate(path []string, raw any) (any, error) {
	if r.evaluator == nil {
		return nil, ErrSelectorEvaluatorUnavailable
	}
	return r.evaluator(path, raw)
}

// unresolved builds the error for a selector that could not be resolved. It names the declaration,
// the component, the stack and the selector, and hints at the usual cause: a producer that has not
// been deployed yet.
func (r *providerRequest) unresolved(name, field, selector string, cause error) error {
	b := errUtils.Build(ErrSelectorUnresolved).
		WithCausef("secret %q field %q in component %q of stack %q uses selector %q: %w",
			name, field, r.component, r.stack, selector, cause)
	producer, producerStack, hasProducer := cloudFormationProducer(selector, r.stack)
	switch {
	case errors.Is(cause, errUtils.ErrAwsCloudFormationStackNotFound) || errors.Is(cause, errUtils.ErrAwsCloudFormationStackNotDeployed):
		if hasProducer {
			b = b.WithHintf("Deploy the producer component first: `atmos aws cloudformation deploy %s --stack %s`", producer, producerStack)
		} else {
			b = b.WithHint("Deploy the producer component first, then retry")
		}
	case errors.Is(cause, errUtils.ErrInvalidComponent):
		if hasProducer {
			b = b.WithHintf("Check that the producer component `%s` exists in stack `%s`", producer, producerStack)
		} else {
			b = b.WithHint("Check that the selector's producer component exists in its stack")
		}
	case errors.Is(cause, ErrSelectorEvaluatorUnavailable):
		b = b.WithHint("Selectors are evaluated with credentials; run a command that authenticates (for example `atmos secret list --verify`)")
	default:
		b = b.WithHint("Check the selector's producer component, stack and output name, or use a literal store/sops name")
	}
	return b.Err()
}

// cloudFormationProducer extracts the producer component and stack from an
// `!aws.cloudformation.output <component> [stack] <output>` selector. The stack defaults to the
// consuming component's stack when omitted.
func cloudFormationProducer(selector, defaultStack string) (string, string, bool) {
	selector = strings.TrimSpace(selector)
	if !strings.HasPrefix(selector, u.AtmosYamlFuncAwsCloudFormationOutput) {
		return "", "", false
	}
	parsed, err := fnparser.ParseTerraform(strings.TrimSpace(strings.TrimPrefix(selector, u.AtmosYamlFuncAwsCloudFormationOutput)))
	if err != nil || parsed.Component == "" {
		return "", "", false
	}
	stack := parsed.Stack
	if stack == "" {
		stack = defaultStack
	}
	return parsed.Component, stack, true
}

// selectorText renders the selector(s) found in a raw value for error messages.
func selectorText(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var found []string
		for _, key := range keys {
			if containsSelector(v[key]) {
				found = append(found, key+"="+selectorText(v[key]))
			}
		}
		return strings.Join(found, ", ")
	case []any:
		var found []string
		for _, child := range v {
			if containsSelector(child) {
				found = append(found, selectorText(child))
			}
		}
		return strings.Join(found, ", ")
	}
	return ""
}
