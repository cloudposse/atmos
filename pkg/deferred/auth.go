// Package deferred plans demanded values and resolves authentication at their point of use.
package deferred

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_auth.go -package=deferred

import (
	"encoding/json"
	"fmt"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// AuthFactory authenticates when a list/describe value needs credentials.
type AuthFactory interface {
	Create(*schema.AtmosConfiguration, *schema.AuthConfig, string) (auth.AuthManager, error)
}

type defaultAuthFactory struct{}

func (defaultAuthFactory) Create(ac *schema.AtmosConfiguration, config *schema.AuthConfig, stack string) (auth.AuthManager, error) {
	defer perf.Track(ac, "deferred.defaultAuthFactory.Create")()

	return auth.CreateAndAuthenticateManagerWithAtmosConfigForStack("", config, cfg.IdentityFlagSelectValue, ac, stack)
}

// Entries exist only after resolution: absence means deferred, a manager means
// authenticated, and err means failed. Disabled is an explicit resolver policy.
type deferredAuthResult struct {
	manager auth.AuthManager
	err     error
}

type deferredAuthResolver struct {
	mu       sync.Mutex
	disabled bool
	factory  AuthFactory
	results  map[string]deferredAuthResult
	values   sync.Map
}

// ConfigureAuth defers implicit authentication and records explicit disable.
// False means an explicit identity was requested and the caller must authenticate it.
func ConfigureAuth(ac *schema.AtmosConfiguration, identity string) bool {
	defer perf.Track(ac, "deferred.ConfigureAuth")()

	identity = cfg.NormalizeIdentityValue(identity)
	if identity != "" && identity != cfg.IdentityFlagDisabledValue {
		ac.DeferredAuth = nil
		return false
	}
	ac.DeferredAuth = NewAuthResolver(AuthOptions{Disabled: identity == cfg.IdentityFlagDisabledValue})
	return true
}

// AuthDisabled distinguishes explicit disable from deferred credentials.
func AuthDisabled(ac *schema.AtmosConfiguration) bool {
	defer perf.Track(ac, "deferred.AuthDisabled")()

	r, ok := ac.DeferredAuth.(*deferredAuthResolver)
	return ok && r.disabled
}

func (r *deferredAuthResolver) Resolve(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(ac, "deferred.deferredAuthResolver.Resolve")()

	if info == nil {
		return nil
	}
	if r.disabled || info.AuthDisabled {
		info.AuthDisabled = true
		info.AuthManager = nil
		info.AuthContext = nil
		return nil
	}
	config, err := auth.MergeComponentAuthFromConfig(&ac.Auth, info.ComponentSection, ac, cfg.AuthSectionName)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	// Stack belongs in the key: emulator endpoints may differ for identical identities.
	key := ac.BasePath + ":" + ac.CliConfigPath + ":" + info.Stack + ":" + string(encoded)
	r.mu.Lock()
	result, found := r.results[key]
	if !found {
		result.manager, result.err = r.factory.Create(ac, config, info.Stack)
		r.results[key] = result
	}
	r.mu.Unlock()
	if result.err != nil {
		return result.err
	}
	propagateAuth(info, result.manager)
	return nil
}

func ResolveAuth(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(ac, "deferred.ResolveAuth")()

	if ac == nil || ac.DeferredAuth == nil {
		return nil
	}
	return ac.DeferredAuth.Resolve(ac, info)
}

// AuthOptions supplies invocation-local dependencies and the explicit disable policy.
type AuthOptions struct {
	Disabled bool
	Factory  AuthFactory
}

// NewAuthResolver creates an invocation-scoped resolver with an empty result cache.
func NewAuthResolver(opts AuthOptions) schema.DeferredAuthResolver {
	defer perf.Track(nil, "deferred.NewAuthResolver")()

	factory := opts.Factory
	if factory == nil {
		factory = defaultAuthFactory{}
	}
	return &deferredAuthResolver{disabled: opts.Disabled, factory: factory, results: make(map[string]deferredAuthResult)}
}

func propagateAuth(info *schema.ConfigAndStacksInfo, manager auth.AuthManager) {
	info.AuthManager = nil
	info.AuthContext = nil
	if manager == nil {
		return
	}
	info.AuthManager = manager
	if stack := manager.GetStackInfo(); stack != nil {
		info.AuthContext = stack.AuthContext
	}
}
