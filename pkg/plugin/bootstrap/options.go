package bootstrap

import (
	"github.com/cloudposse/atmos/pkg/api"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/auth"
)

// Option configures Atmos-owned runtime dependencies at the composition root.
type Option func(*adapter.Services)

// WithInteraction supplies browser and interactivity services for embedders and tests.
// Plugin package settings cannot override this decision.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func WithInteraction(browser api.Browser, available func() bool) Option {
	return func(services *adapter.Services) { services.Browser, services.Interactive = browser, available }
}
