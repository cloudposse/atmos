// Package deferred composes store lookups with their deferred dependencies.
package deferred

import (
	"errors"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/deferred"
	fnparser "github.com/cloudposse/atmos/pkg/function/parser"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// List/describe store calls return errors to the value walker instead of exiting the
// process, so an unavailable identity can affect one value without aborting output.
func ReadStore(ac *schema.AtmosConfiguration, input, stack string, info *schema.ConfigAndStacksInfo) (any, error) {
	return NewRead(ac, input, stack, info).Resolve()
}

// NewRead defers parsing and evaluation of a YAML store reference.
func NewRead(ac *schema.AtmosConfiguration, input, stack string, info *schema.ConfigAndStacksInfo) deferred.Resolver[any] {
	return deferred.Func[any](func() (any, error) {
		return readStore(ac, input, stack, info)
	})
}

func readStore(ac *schema.AtmosConfiguration, input, stack string, info *schema.ConfigAndStacksInfo) (any, error) {
	defer perf.Track(ac, "store.deferred.ReadStore")()

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
	var lookupErr error
	value, err := WithStoreAuth(ac, info, p.storeName, func() (any, error) {
		var result any
		if getKey {
			result, lookupErr = s.GetKey(p.key)
		} else {
			result, lookupErr = s.Get(p.stack, p.component, p.key)
		}
		return result, nil
	})
	if err != nil {
		return nil, err // Authentication failures must not activate value defaults.
	}
	return deferredStoreResult(ac, p, value, lookupErr)
}

func deferredStoreResult(ac *schema.AtmosConfiguration, p *storeParams, value any, err error) (any, error) {
	if errors.Is(err, errUtils.ErrAuthenticationUnavailable) {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAuthenticationUnavailable, err)
	}
	// A value default must not hide a failed identity or denied access.
	if errors.Is(err, errUtils.ErrAuthenticationFailed) ||
		errors.Is(err, store.ErrAuthContextNotAvailable) ||
		errors.Is(err, store.ErrIdentityNotConfigured) ||
		errors.Is(err, store.ErrPermissionDenied) {
		return nil, err
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

type storeParams struct {
	storeName, stack, component, key, query string
	defaultValue                            *string
}

// StoreOptions identifies the store value requested by a template.
type StoreOptions struct{ Name, Stack, Component, Key string }

// LookupStore retrieves a template's requested value using the same deferred identity policy.
func LookupStore(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, opts StoreOptions) (any, error) {
	return NewValue(ac, info, opts).Resolve()
}

// NewValue defers a store read and its credential dependency until requested.
func NewValue(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, opts StoreOptions) deferred.Resolver[any] {
	return deferred.Func[any](func() (any, error) {
		defer perf.Track(ac, "store.deferred.NewValue.Resolve")()
		return readDeferredStore(ac, &storeParams{storeName: opts.Name, stack: opts.Stack, component: opts.Component, key: opts.Key}, info, false)
	})
}
