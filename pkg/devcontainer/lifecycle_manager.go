package devcontainer

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// Manager handles devcontainer lifecycle operations with dependency injection.
// It provides methods for managing devcontainers: List, Start, Stop, Attach, Exec,
// Remove, Rebuild, Logs, ShowConfig, and instance management.
type Manager struct {
	configLoader    ConfigLoader
	identityManager IdentityManager
	runtimeDetector RuntimeDetector
}

// Option configures the Manager.
type Option func(*Manager)

// WithConfigLoader sets a custom ConfigLoader.
func WithConfigLoader(loader ConfigLoader) Option {
	defer perf.Track(nil, "devcontainer.WithConfigLoader")()

	return func(m *Manager) {
		m.configLoader = loader
	}
}

// WithIdentityManager sets a custom IdentityManager.
func WithIdentityManager(mgr IdentityManager) Option {
	defer perf.Track(nil, "devcontainer.WithIdentityManager")()

	return func(m *Manager) {
		m.identityManager = mgr
	}
}

// WithRuntimeDetector sets a custom RuntimeDetector.
func WithRuntimeDetector(detector RuntimeDetector) Option {
	defer perf.Track(nil, "devcontainer.WithRuntimeDetector")()

	return func(m *Manager) {
		m.runtimeDetector = detector
	}
}

// NewManager creates a new Manager with default or custom dependencies.
// Use Option functions to provide custom implementations for testing.
func NewManager(opts ...Option) *Manager {
	defer perf.Track(nil, "devcontainer.NewManager")()

	m := &Manager{
		configLoader:    NewConfigLoader(),
		identityManager: NewIdentityManager(),
		runtimeDetector: NewRuntimeDetector(),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}
