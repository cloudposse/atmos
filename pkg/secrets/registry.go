package secrets

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// secretsSectionKey is the component-section key holding secret declarations.
const secretsSectionKey = "secrets"

// varsSectionKey is the key under `secrets` holding the per-name declaration map.
const varsSectionKey = "vars"

// providersSectionKey is the key under `secrets` holding stack/component-defined providers.
const providersSectionKey = "providers"

// ExtractDeclarations reads the `secrets.vars` map from a resolved component section and
// returns the declared secrets keyed by name. Inheritance is already applied by the standard
// Atmos stack-merge pipeline, so this simply reads the merged map. Unknown/zero values are
// tolerated; a declaration with neither `store` nor `sops` is returned with an empty
// BackendName so callers can surface ErrNoBackend at use time.
func ExtractDeclarations(componentSection map[string]any) map[string]Declaration {
	defer perf.Track(nil, "secrets.ExtractDeclarations")()

	out := make(map[string]Declaration)

	secretsSection, ok := componentSection[secretsSectionKey].(map[string]any)
	if !ok {
		return out
	}
	varsSection, ok := secretsSection[varsSectionKey].(map[string]any)
	if !ok {
		return out
	}

	for name, raw := range varsSection {
		decl := Declaration{Name: name, Scope: ScopeInstance}
		if spec, ok := raw.(map[string]any); ok {
			decl.Description = stringField(spec, "description")
			decl.Reference = stringField(spec, "reference")
			decl.Required = boolField(spec, "required")
			decl.Scope = scopeField(spec)
			if store := stringField(spec, "store"); store != "" {
				decl.BackendType = BackendStore
				decl.BackendName = store
			}
			if sops := stringField(spec, "sops"); sops != "" {
				decl.BackendType = BackendSops
				decl.BackendName = sops
			}
		}
		out[name] = decl
	}

	return out
}

// LookupDeclaration returns the declaration for a secret name from a component section.
func LookupDeclaration(componentSection map[string]any, name string) (Declaration, bool) {
	defer perf.Track(nil, "secrets.LookupDeclaration")()

	decls := ExtractDeclarations(componentSection)
	decl, ok := decls[name]
	return decl, ok
}

// stringField reads a string field from a spec map, tolerating absent keys.
func stringField(spec map[string]any, key string) string {
	if v, ok := spec[key].(string); ok {
		return v
	}
	return ""
}

// boolField reads a bool field from a spec map, tolerating absent keys.
func boolField(spec map[string]any, key string) bool {
	if v, ok := spec[key].(bool); ok {
		return v
	}
	return false
}

// scopeField reads the derived `scope` tag stamped onto a declaration by the stack processor,
// defaulting to ScopeInstance when absent or unrecognized. The tag is normally injected by
// position (top-level `secrets:` → stack; component `secrets:` → instance), but a directly
// constructed section may carry an explicit value.
func scopeField(spec map[string]any) Scope {
	if stringField(spec, "scope") == string(ScopeStack) {
		return ScopeStack
	}
	return ScopeInstance
}
