package manifest

import (
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Definition describes a registered manifest kind: its identity, the JSON
// Schema generated from its spec prototype, and the compiled validator.
type Definition struct {
	Kind       string
	APIVersion string
	schemaJSON string
	compiled   *jsonschema.Schema
}

// SchemaJSON returns the generated JSON Schema document for this kind.
// The schema covers the full envelope (apiVersion, kind, metadata, spec)
// and can be exported for IDE completion or SchemaStore publication.
func (d *Definition) SchemaJSON() string {
	defer perf.Track(nil, "manifest.Definition.SchemaJSON")()

	return d.schemaJSON
}

// kindRegistry is the global thread-safe registry of manifest kinds.
type kindRegistry struct {
	mu          sync.RWMutex
	definitions map[string]*Definition
}

var registry = &kindRegistry{
	definitions: make(map[string]*Definition),
}

// Register adds a manifest kind to the registry. The spec prototype is a
// (typically zero-valued) instance of the kind's spec struct; its JSON
// Schema is generated immediately so schema errors surface at startup
// rather than at first load.
//
// Re-registering an existing kind replaces it (last registration wins).
func Register(kind, apiVersion string, specPrototype any) error {
	defer perf.Track(nil, "manifest.Register")()

	if kind == "" {
		return errUtils.ErrManifestKindEmpty
	}
	if specPrototype == nil {
		return errUtils.Build(errUtils.ErrManifestPrototypeNil).
			WithExplanationf("Cannot register manifest kind `%s` without a spec prototype", kind).
			Err()
	}
	if apiVersion == "" {
		apiVersion = DefaultAPIVersion
	}

	schemaJSON, err := generateEnvelopeSchema(kind, apiVersion, specPrototype)
	if err != nil {
		return err
	}

	compiled, err := compileSchema(kind, schemaJSON)
	if err != nil {
		return err
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.definitions[kind] = &Definition{
		Kind:       kind,
		APIVersion: apiVersion,
		schemaJSON: schemaJSON,
		compiled:   compiled,
	}
	return nil
}

// MustRegister registers a manifest kind and panics on failure.
// Intended for package init functions where registration errors are
// programming errors.
func MustRegister(kind, apiVersion string, specPrototype any) {
	defer perf.Track(nil, "manifest.MustRegister")()

	if err := Register(kind, apiVersion, specPrototype); err != nil {
		panic(err)
	}
}

// GetDefinition returns the definition for a registered kind.
func GetDefinition(kind string) (*Definition, bool) {
	defer perf.Track(nil, "manifest.GetDefinition")()

	registry.mu.RLock()
	defer registry.mu.RUnlock()

	def, ok := registry.definitions[kind]
	return def, ok
}

// Kinds returns all registered manifest kinds sorted alphabetically.
func Kinds() []string {
	defer perf.Track(nil, "manifest.Kinds")()

	registry.mu.RLock()
	defer registry.mu.RUnlock()

	kinds := make([]string, 0, len(registry.definitions))
	for kind := range registry.definitions {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

// Reset clears the registry (for testing only).
func Reset() {
	defer perf.Track(nil, "manifest.Reset")()

	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.definitions = make(map[string]*Definition)
}

// kindsHint formats the registered kinds for use in error hints.
func kindsHint() string {
	kinds := Kinds()
	if len(kinds) == 0 {
		return "no manifest kinds are registered"
	}
	return "Registered kinds: `" + strings.Join(kinds, "`, `") + "`"
}
