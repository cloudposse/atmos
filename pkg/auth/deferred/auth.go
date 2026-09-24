// Package deferred resolves Atmos authentication at the first credential request.
package deferred

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_auth.go -package=deferred

import (
	"encoding/json"
	"fmt"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/deferred"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// AuthFactory authenticates when a list/describe value needs credentials.
type AuthFactory interface {
	Create(*schema.AtmosConfiguration, *schema.AuthConfig, string) (auth.AuthManager, error)
}

type defaultAuthFactory struct{}

func (defaultAuthFactory) Create(ac *schema.AtmosConfiguration, config *schema.AuthConfig, stack string) (auth.AuthManager, error) {
	defer perf.Track(ac, "auth.deferred.defaultAuthFactory.Create")()

	return auth.CreateAndAuthenticateManagerWithAtmosConfigForStack("", config, cfg.IdentityFlagSelectValue, ac, stack)
}

// Results are created on demand and memoize either an authenticated manager or
// a failure. Disabled remains an explicit policy, not an absent manager.
type Manager struct {
	mu          sync.Mutex
	disabled    bool
	factory     AuthFactory
	results     map[string]deferred.Resolver[auth.AuthManager]
	config      *schema.AtmosConfiguration
	baseMu      sync.Mutex
	baseManager auth.AuthManager
	baseError   error
	baseReady   bool
}

// ConfigureAuth defers implicit authentication and records explicit disable.
// False means an explicit identity was requested and the caller must authenticate it.
func ConfigureAuth(ac *schema.AtmosConfiguration, identity string) bool {
	defer perf.Track(ac, "auth.deferred.ConfigureAuth")()

	identity = cfg.NormalizeIdentityValue(identity)
	if identity != "" && identity != cfg.IdentityFlagDisabledValue {
		ac.AuthManager = nil
		return false
	}
	ac.AuthManager = NewManager(AuthOptions{Disabled: identity == cfg.IdentityFlagDisabledValue, Config: ac})
	return true
}

// AuthDisabled distinguishes explicit disable from deferred credentials.
func AuthDisabled(manager any) bool {
	policy, ok := manager.(interface{ Disabled() bool })
	return ok && policy.Disabled()
}

// Disabled reports an explicit request not to use Atmos authentication.
func (r *Manager) Disabled() bool { return r.disabled }

// IsDeferred reports the manager's evaluation policy, without resolving credentials.
func IsDeferred(manager any) bool {
	_, ok := manager.(*Manager)
	return ok
}

func (r *Manager) Resolve(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(ac, "auth.deferred.Manager.Resolve")()

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
		result = deferred.Once(deferred.Func[auth.AuthManager](func() (auth.AuthManager, error) {
			return r.factory.Create(ac, config, info.Stack)
		}))
		r.results[key] = result
	}
	// Authentication may update shared credential files or process state; keep
	// different effective identities serialized as well as memoizing each result.
	manager, err := result.Resolve()
	r.mu.Unlock()
	if err != nil {
		return err
	}
	propagateAuth(info, manager)
	return nil
}

func ResolveAuth(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(ac, "auth.deferred.ResolveAuth")()

	if ac == nil {
		return nil
	}
	manager, ok := ac.AuthManager.(*Manager)
	if !ok {
		return nil
	}
	return manager.Resolve(ac, info)
}

// AuthOptions supplies invocation-local dependencies and the explicit disable policy.
type AuthOptions struct {
	Disabled bool
	Factory  AuthFactory
	Config   *schema.AtmosConfiguration
}

// NewManager creates an invocation-scoped resolver with an empty result cache.
func NewManager(opts AuthOptions) *Manager {
	defer perf.Track(nil, "auth.deferred.NewManager")()

	factory := opts.Factory
	if factory == nil {
		factory = defaultAuthFactory{}
	}
	return &Manager{disabled: opts.Disabled, factory: factory, config: opts.Config, results: make(map[string]deferred.Resolver[auth.AuthManager])}
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
