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
// stack-scoped, and a stack-level declaration can't be instance-scoped.
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
		if existing, ok := spec[scopeFieldKey].(string); ok && existing != "" && existing != string(scope) {
			return nil, fmt.Errorf("%w: secret %q declares scope %q but its position implies %q",
				ErrScopeConflict, name, existing, scope)
		}
		newSpec := make(map[string]any, len(spec)+1)
		for sk, sv := range spec {
			newSpec[sk] = sv
		}
		newSpec[scopeFieldKey] = string(scope)
		newVars[name] = newSpec
	}
	out[varsSectionKey] = newVars
	return out, nil
}
