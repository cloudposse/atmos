package exec

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	stackdeferred "github.com/cloudposse/atmos/pkg/stack/deferred"
	"github.com/cloudposse/atmos/pkg/store"
	storedeferred "github.com/cloudposse/atmos/pkg/store/deferred"
)

var storeFuncSyncMap = sync.Map{}

func deferredStoreFunc(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, opts storedeferred.StoreOptions) (any, error) {
	lookup := func() (any, error) { return storedeferred.LookupStore(ac, info, opts) }
	cacheKey, ok := deferredStoreCacheKey(ac, info, opts)
	if !ok {
		return lookup()
	}
	return stackdeferred.StoreValue(ac, cacheKey, lookup)
}

// Only pointer-backed stores are cached. Their address identifies the live
// backend, so separate registries cannot share values within an invocation.
func deferredStoreCacheKey(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, opts storedeferred.StoreOptions) (string, bool) {
	backend := ac.Stores[opts.Name]
	if backend == nil {
		return "", false
	}
	backendValue := reflect.ValueOf(backend)
	if backendValue.Kind() != reflect.Pointer || backendValue.IsNil() {
		return "", false
	}
	storeConfig := ac.StoresConfig[opts.Name]
	if storeConfig.Secret {
		return "", false
	}
	var stack string
	var section map[string]any
	var disabled bool
	if info != nil {
		stack, section, disabled = info.Stack, info.ComponentSection, info.AuthDisabled
	}
	effectiveAuth, err := auth.MergeComponentAuthFromConfig(&ac.Auth, section, ac, cfg.AuthSectionName)
	if err != nil {
		return "", false
	}
	encoded, err := json.Marshal(struct {
		BasePath, ConfigPath, AuthStack string
		AuthDisabled                    bool
		Auth                            *schema.AuthConfig
		Store                           store.StoreConfig
		Lookup                          storedeferred.StoreOptions
	}{ac.BasePath, ac.CliConfigPath, stack, disabled || authdeferred.AuthDisabled(ac.AuthManager), effectiveAuth, storeConfig, opts})
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("%x:%x", backendValue.Pointer(), sha256.Sum256(encoded)), true
}

func storeFunc(
	atmosConfig *schema.AtmosConfiguration,
	storeName string,
	stack string,
	component string,
	key string,
) (any, error) {
	functionName := fmt.Sprintf("atmos.Store(%s, %s, %s, %s)", storeName, stack, component, key)
	slug := fmt.Sprintf("%s-%s-%s-%s", storeName, stack, component, key)

	log.Debug("Executing template function", "function", functionName)

	if cfg, ok := atmosConfig.StoresConfig[storeName]; ok && cfg.Secret {
		return nil, fmt.Errorf("%w: store %q (in %s)", errUtils.ErrStoreIsSecret, storeName, functionName)
	}

	// If the result for the component in the stack already exists in the cache, return it
	existing, found := storeFuncSyncMap.Load(slug)
	if found && existing != nil {
		log.Debug("Cache hit for template function", "function", functionName, "result", existing)
		return existing, nil
	}

	// Retrieve the store from atmosConfig
	store := atmosConfig.Stores[storeName]

	if store == nil {
		return nil, fmt.Errorf("%w: %s\nstore '%s' not found", errUtils.ErrInvalidTemplateFunc, functionName, storeName)
	}

	// Retrieve the value from the store
	value, err := store.Get(stack, component, key)
	if err != nil {
		value = nil
	}

	// Cache the result
	storeFuncSyncMap.Store(slug, value)

	log.Debug("Executed template function", "function", functionName, "result", value)

	return value, nil
}
