package git

import (
	"fmt"
	"sort"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
)

// ProviderFactory constructs a Provider instance.
type ProviderFactory func() Provider

var (
	providersMu sync.RWMutex
	providers   = map[string]ProviderFactory{}
)

// RegisterProvider registers a provider factory under a name. Providers
// self-register from init() (e.g. pkg/git/providers/cli), following the
// standard Atmos registry pattern.
func RegisterProvider(name string, factory ProviderFactory) {
	providersMu.Lock()
	defer providersMu.Unlock()
	providers[name] = factory
}

// NewProvider returns a new instance of the named provider.
// An empty name resolves to the default "cli" provider.
func NewProvider(name string) (Provider, error) {
	if name == "" {
		name = DefaultProviderName
	}

	providersMu.RLock()
	factory, ok := providers[name]
	providersMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q (registered: %v)", errUtils.ErrGitProviderNotFound, name, RegisteredProviders())
	}

	return factory(), nil
}

// RegisteredProviders returns the sorted names of registered providers.
func RegisteredProviders() []string {
	providersMu.RLock()
	defer providersMu.RUnlock()

	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}
