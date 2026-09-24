package deferred

import (
	"errors"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	fnparser "github.com/cloudposse/atmos/pkg/function/parser"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// List/describe store calls return errors to the value walker instead of exiting the
// process, so an unavailable identity can affect one value without aborting output.
func ReadStore(ac *schema.AtmosConfiguration, input, stack string, info *schema.ConfigAndStacksInfo) (any, error) {
	defer perf.Track(ac, "deferred.ReadStore")()

	getKey := strings.HasPrefix(input, u.AtmosYamlFuncStoreGet+" ")
	tag := u.AtmosYamlFuncStore
	if getKey {
		tag = u.AtmosYamlFuncStoreGet
	}
	args := strings.TrimSpace(strings.TrimPrefix(input, tag))
	var p storeParams
	if getKey {
		parsed, parseErr := fnparser.ParseStoreGet(args)
		if parseErr != nil {
			return nil, parseErr
		}
		p = storeParams{storeName: parsed.Store, key: parsed.Key, query: parsed.Query, defaultValue: parsed.Default}
	} else {
		parsed, parseErr := fnparser.ParseStore(args)
		if parseErr != nil {
			return nil, parseErr
		}
		p = storeParams{storeName: parsed.Store, stack: parsed.Stack, component: parsed.Component, key: parsed.Key, query: parsed.Query, defaultValue: parsed.Default}
	}
	if p.stack == "" {
		p.stack = stack
	}
	return readDeferredStore(ac, &p, info, getKey)
}

func readDeferredStore(ac *schema.AtmosConfiguration, p *storeParams, info *schema.ConfigAndStacksInfo, getKey bool) (any, error) {
	s := ac.Stores[p.storeName]
	if s == nil {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrStoreNotFound, p.storeName)
	}
	if ac.StoresConfig[p.storeName].Secret {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrStoreIsSecret, p.storeName)
	}
	if err := ResolveStoreAuth(ac, info, p.storeName); err != nil {
		return nil, err
	}
	var value any
	var err error
	if getKey {
		value, err = s.GetKey(p.key)
	} else {
		value, err = s.Get(p.stack, p.component, p.key)
	}
	return deferredStoreResult(ac, p, value, err)
}

func deferredStoreResult(ac *schema.AtmosConfiguration, p *storeParams, value any, err error) (any, error) {
	if errors.Is(err, errUtils.ErrAuthenticationUnavailable) {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAuthenticationUnavailable, err)
	}
	if err != nil {
		return deferredStoreDefault(p.defaultValue, err)
	}
	if value == nil && p.defaultValue != nil {
		return *p.defaultValue, nil
	}
	if p.query != "" {
		return u.EvaluateYqExpression(ac, value, p.query)
	}
	return value, nil
}

func deferredStoreDefault(value *string, err error) (any, error) {
	if value != nil {
		return *value, nil
	}
	return nil, err
}

func ResolveStoreAuth(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, name string) error {
	defer perf.Track(ac, "deferred.ResolveStoreAuth")()

	s, ok := ac.Stores[name].(store.IdentityAwareStore)
	if !ok || ac.DeferredAuth == nil {
		return nil
	}
	if AuthDisabled(ac) || (info != nil && info.AuthDisabled) {
		s.SetAuthContext(nil, "")
		return nil
	}
	copyInfo := schema.ConfigAndStacksInfo{}
	if info != nil {
		copyInfo = *info
	}
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
		return
	}
	chain := manager.GetChain()
	if identity == "" && len(chain) > 0 {
		identity = chain[len(chain)-1]
	}
	info := manager.GetStackInfo()
	if info == nil {
		return
	}
	s.SetAuthContext(authbridge.NewResolvedContext(info.AuthContext), identity)
}

type storeParams struct {
	storeName, stack, component, key, query string
	defaultValue                            *string
}

// StoreOptions identifies the store value requested by a template.
type StoreOptions struct{ Name, Stack, Component, Key string }

// LookupStore retrieves a template's requested value using the same deferred identity policy.
func LookupStore(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, opts StoreOptions) (any, error) {
	defer perf.Track(ac, "deferred.LookupStore")()
	return readDeferredStore(ac, &storeParams{storeName: opts.Name, stack: opts.Stack, component: opts.Component, key: opts.Key}, info, false)
}
