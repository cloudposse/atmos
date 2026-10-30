package registry

// HasProvider reports whether the process registry can construct this provider kind.
func HasProvider(kind string) bool {
	defaultRegistry.mu.RLock()
	defer defaultRegistry.mu.RUnlock()
	_, found := defaultRegistry.providers[kind]
	return found
}

// HasIdentity reports whether the process registry can construct this identity kind.
func HasIdentity(kind string) bool {
	defaultRegistry.mu.RLock()
	defer defaultRegistry.mu.RUnlock()
	_, found := defaultRegistry.identities[kind]
	return found
}
