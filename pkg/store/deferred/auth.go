package deferred

import (
	"github.com/cloudposse/atmos/pkg/auth"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
)

func resolveStoreAuth(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, name string) error {
	defer perf.Track(ac, "store.deferred.ResolveStoreAuth")()

	s, ok := ac.Stores[name].(store.IdentityAwareStore)
	if !ok || !authdeferred.IsDeferred(ac.AuthManager) {
		return nil
	}
	// Stores are shared between components. Even the same identity name can resolve
	// to different credentials or endpoints under a different effective auth config.
	s.ResetAuthContext()
	if authdeferred.AuthDisabled(ac.AuthManager) || (info != nil && info.AuthDisabled) {
		return nil
	}
	identity := ac.StoresConfig[name].Identity
	resolved, err := authdeferred.Credentials(ac, info, identity).Resolve()
	if err != nil {
		return err
	}
	manager, _ := resolved.AuthManager.(auth.AuthManager)
	injectDeferredStoreContext(s, manager, identity)
	return nil
}

func injectDeferredStoreContext(s store.IdentityAwareStore, manager auth.AuthManager, identity string) {
	if manager == nil {
		preserveConfiguredStoreIdentity(s, identity)
		return
	}
	info := manager.GetStackInfo()
	if info == nil {
		preserveConfiguredStoreIdentity(s, identity)
		return
	}
	chain := manager.GetChain()
	if identity == "" && len(chain) > 0 {
		identity = chain[len(chain)-1]
	}
	s.SetAuthContext(authbridge.NewResolvedContext(info.AuthContext), identity)
}

// An explicitly configured identity must not silently become ambient credentials
// if a custom resolver returns no usable authentication context.
func preserveConfiguredStoreIdentity(s store.IdentityAwareStore, identity string) {
	if identity != "" {
		s.SetAuthContext(nil, identity)
	}
}
