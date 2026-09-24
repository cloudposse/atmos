package deferred

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
)

func ResolveStoreAuth(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, name string) error {
	defer perf.Track(ac, "auth.deferred.ResolveStoreAuth")()

	s, ok := ac.Stores[name].(store.IdentityAwareStore)
	if !ok || ac.DeferredAuth == nil {
		return nil
	}
	// Stores are shared between components. Even the same identity name can resolve
	// to different credentials or endpoints under a different effective auth config.
	s.ResetAuthContext()
	if AuthDisabled(ac) || (info != nil && info.AuthDisabled) {
		return nil
	}
	copyInfo := schema.ConfigAndStacksInfo{}
	if info != nil {
		copyInfo = *info
	}
	copyInfo.AuthManager = nil
	copyInfo.AuthContext = nil
	copyConfig := *ac
	identity := ac.StoresConfig[name].Identity
	if identity != "" {
		merged, err := deferredStoreIdentity(ac, &copyInfo, identity)
		if err != nil {
			return err
		}
		copyConfig.Auth = *merged
		copyInfo.ComponentSection = nil
	}
	if err := ResolveAuth(&copyConfig, &copyInfo); err != nil {
		return err
	}
	manager, _ := copyInfo.AuthManager.(auth.AuthManager)
	injectDeferredStoreContext(s, manager, identity)
	return nil
}

func deferredStoreIdentity(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, identity string) (*schema.AuthConfig, error) {
	merged, err := auth.MergeComponentAuthFromConfig(&ac.Auth, info.ComponentSection, ac, cfg.AuthSectionName)
	if err != nil {
		return nil, err
	}
	if _, exists := merged.Identities[identity]; !exists {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrIdentityNotFound, identity)
	}
	for key := range merged.Identities {
		config := merged.Identities[key]
		config.Default = key == identity
		merged.Identities[key] = config
	}
	return merged, nil
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
