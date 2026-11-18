package authority

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
)

// Options configure an authority when the registry builds it.
type Options struct {
	// HTTPClient makes the authority's requests. Nil means the authority builds its own.
	HTTPClient *http.Client
	// BaseURL replaces the authority's default API address. Tests point it at a fake server.
	BaseURL string
	// UserAgent identifies the caller to the authority. Empty means the authority's default.
	UserAgent string
}

// Factory builds an authority.
type Factory struct {
	// Name is the registry key, which must equal the built authority's Name.
	Name string
	// New builds the authority from options.
	New func(Options) (Authority, error)
}

// Registry maps authority names to factories.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{factories: map[string]Factory{}}
}

// Default is the registry the plugin wiring fills.
var Default = NewRegistry()

// Register adds a factory. It fails when the name is empty, the factory is incomplete, or the name
// is taken.
func (r *Registry) Register(f Factory) error {
	if f.Name == "" || f.New == nil {
		return fmt.Errorf("%w: factory needs a name and a constructor", ErrInvalidRequest)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, taken := r.factories[f.Name]; taken {
		return fmt.Errorf("%w: %q", ErrDuplicateAuthority, f.Name)
	}
	r.factories[f.Name] = f
	return nil
}

// Lookup returns the factory registered under name.
func (r *Registry) Lookup(name string) (Factory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.factories[name]
	return f, ok
}

// New builds the authority registered under name.
func (r *Registry) New(name string, options Options) (Authority, error) {
	f, ok := r.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("%w: %q (known: %v)", ErrUnknownAuthority, name, r.Names())
	}
	a, err := f.New(options)
	if err != nil {
		return nil, fmt.Errorf("build authority %q: %w", name, err)
	}
	return a, nil
}

// Names lists the registered names in order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Register adds a factory to the Default registry.
func Register(f Factory) error { return Default.Register(f) }

// New builds an authority from the Default registry.
func New(name string, options Options) (Authority, error) { return Default.New(name, options) }

// Names lists the Default registry's names.
func Names() []string { return Default.Names() }
