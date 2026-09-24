// Package deferred composes secret lookups with their deferred dependencies.
package deferred

import (
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/deferred"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
)

// NewValue defers the entire lookup, including preparation of store or cloud
// credentials. Masked values retain validation without contacting a backend.
func NewValue(ac *schema.AtmosConfiguration, input, stack string, info *schema.ConfigAndStacksInfo) deferred.Resolver[any] {
	defer perf.Track(ac, "secrets.deferred.NewValue")()

	return deferred.Func[any](func() (any, error) {
		defer perf.Track(ac, "secrets.deferred.NewValue.Resolve")()
		if authdeferred.IsDeferred(ac.AuthManager) && info != nil && !info.SecretsMaskOnly {
			if err := PrepareSecretAuth(ac, input, info); err != nil {
				return nil, err
			}
		}
		return secrets.Resolve(ac, input, stack, info)
	})
}
