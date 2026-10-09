package deferred

import (
	"sync"

	"github.com/cloudposse/atmos/pkg/auth"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
	"github.com/cloudposse/atmos/pkg/ui"
)

// resolveStoreAuth resets shared store credentials and resolves its effective identity.
func resolveStoreAuth(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, name string) error {
	defer perf.Track(ac, "store.deferred.ResolveStoreAuth")()

	s, ok := ac.Stores[name].(store.IdentityAwareStore)
	if !ok || !authdeferred.IsDeferred(ac.AuthManager) {
		return nil
	}
	// Stores are shared between components. Even the same identity name can resolve
	// to different credentials or endpoints under a different effective auth config.
	s.ResetAuthContext()
	// `--identity=false` (or an auth-disabled scope) means Atmos authentication is disabled
	// globally. That includes a store pinned to an explicit `identity:`: the pin is bypassed and the
	// store uses whatever credentials are ambient in the environment. Make that explicit instead of
	// silently ignoring the pin.
	if authdeferred.AuthDisabled(ac.AuthManager) || (info != nil && info.AuthDisabled) {
		warnPinnedIdentityBypassed(ac, name)
		return nil
	}
	identity := ac.StoresConfig[name].Identity
	if identity == "" && info != nil {
		// A store's explicit identity wins; otherwise inherit the caller's
		// explicit CLI identity without changing component or global defaults.
		raw := info.Identity
		if info.RequestedIdentity != nil {
			raw = *info.RequestedIdentity
		}
		requested := cfg.NormalizeIdentityValue(raw)
		if requested != cfg.IdentityFlagSelectValue && requested != cfg.IdentityFlagDisabledValue {
			identity = requested
		}
	}
	resolved, err := authdeferred.Credentials(ac, info, identity).Resolve()
	if err != nil {
		return err
	}
	manager, _ := resolved.AuthManager.(auth.AuthManager)
	injectDeferredStoreContext(s, manager, identity)
	return nil
}

// injectDeferredStoreContext binds resolved credentials without changing caller defaults.
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

// warnFn reports a bypassed pinned identity. It is a seam so tests can capture the warning.
var warnFn = ui.Warningf

// pinnedIdentityWarned records the (store, identity) pairs already reported, so the warning is
// shown once per process even though a store is read many times.
var pinnedIdentityWarned sync.Map

// warnPinnedIdentityBypassed tells the user, once per store, that Atmos authentication is disabled
// so the identity the store is pinned to is not used.
func warnPinnedIdentityBypassed(ac *schema.AtmosConfiguration, name string) {
	pinned := ac.StoresConfig[name].Identity
	if pinned == "" {
		return
	}
	if _, loaded := pinnedIdentityWarned.LoadOrStore(name+"\x00"+pinned, struct{}{}); loaded {
		return
	}
	warnFn("Atmos authentication is disabled (--identity=false), so store `%s` ignores its pinned identity `%s` and uses ambient credentials from the environment", name, pinned)
}

// preserveConfiguredStoreIdentity ensures an explicit identity cannot become ambient credentials
// if a custom resolver returns no usable authentication context while authentication is enabled.
// It does not apply when authentication is explicitly disabled (see resolveStoreAuth).
func preserveConfiguredStoreIdentity(s store.IdentityAwareStore, identity string) {
	if identity != "" {
		s.SetAuthContext(nil, identity)
	}
}
