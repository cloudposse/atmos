// Package deferred composes secret lookups with their deferred dependencies.
package deferred

import (
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/deferred"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
	storedeferred "github.com/cloudposse/atmos/pkg/store/deferred"
)

// NewValue defers the entire lookup, including preparation of store or cloud
// credentials. Masked values retain validation without contacting a backend.
func NewValue(ac *schema.AtmosConfiguration, input, stack string, info *schema.ConfigAndStacksInfo) deferred.Resolver[any] {
	defer perf.Track(ac, "secrets.deferred.NewValue")()

	return deferred.Func[any](func() (any, error) {
		defer perf.Track(ac, "secrets.deferred.NewValue.Resolve")()
		// SecretsAuth and store bindings are lookup-local, even when stack workers
		// share configuration. Store-backed SOPS age keys use the same scoped registry.
		config := *ac
		if authdeferred.IsDeferred(ac.AuthManager) && info != nil && !info.SecretsMaskOnly {
			config.Stores = storedeferred.ScopedStores(ac, info)
			if err := PrepareSecretAuth(&config, input, info); err != nil {
				return nil, err
			}
		}
		return secrets.Resolve(&config, input, stack, info)
	})
}
