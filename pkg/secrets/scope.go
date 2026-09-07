package secrets

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
)

// scopeFieldKey is the key under a declaration's spec map holding its (derived) scope.
const scopeFieldKey = "scope"

// TagScope returns a copy of a `secrets:` section with the derived scope stamped onto every
// declaration under `secrets.vars`. Stamping happens by declaration position before the standard
// Atmos deep-merge, so "most-specific wins" resolves stack-vs-instance overrides for free (a
// component-level `instance` tag overrides an inherited stack-level `stack` tag) and
// ExtractDeclarations reads the resolved scope after the merge.
//
// The input section is NOT mutated — the stack-level (global) section is shared across every
// component in a stack, so mutating it in place would race under concurrent stack processing.
//
// A declaration carrying an explicit `scope` that conflicts with the positional scope returns
// ErrScopeConflict, enforcing the one-way rule: an instance-declared secret can never be
// stack-scoped, and a stack-level declaration can't be instance-scoped. An explicit
// `scope: global` is exempt — it is strictly more shared than either position implies, so it is
// honored wherever the declaration appears (typically a catalog fragment imported anywhere).
func TagScope(section map[string]any, scope Scope) (map[string]any, error) {
	defer perf.Track(nil, "secrets.TagScope")()

	if len(section) == 0 {
		return section, nil
	}

	out := make(map[string]any, len(section))
	for k, v := range section {
		out[k] = v
	}

	varsRaw, ok := section[varsSectionKey].(map[string]any)
	if !ok {
		return out, nil
	}

	newVars := make(map[string]any, len(varsRaw))
	for name, raw := range varsRaw {
		spec, ok := raw.(map[string]any)
		if !ok {
			newVars[name] = raw
			continue
		}
		stamped, err := stampDeclarationScope(name, spec, scope)
		if err != nil {
			return nil, err
		}
		newVars[name] = stamped
	}
	out[varsSectionKey] = newVars
	return out, nil
}

// stampDeclarationScope returns a copy of one declaration spec with the positional scope stamped,
// enforcing the one-way conflict rule. An explicit `scope: global` is exempt and survives the
// stamp; any other explicit scope must match the positional one.
func stampDeclarationScope(name string, spec map[string]any, scope Scope) (map[string]any, error) {
	existing, _ := spec[scopeFieldKey].(string)
	if existing != "" && existing != string(scope) && existing != string(ScopeGlobal) {
		return nil, fmt.Errorf("%w: secret %q declares scope %q but its position implies %q",
			ErrScopeConflict, name, existing, scope)
	}
	newSpec := make(map[string]any, len(spec))
	for sk, sv := range spec {
		newSpec[sk] = sv
	}
	// An explicit global scope survives the positional stamp; everything else takes it.
	if existing != string(ScopeGlobal) {
		newSpec[scopeFieldKey] = string(scope)
	}
	return newSpec, nil
}
