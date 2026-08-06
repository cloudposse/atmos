package store

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	pstore "github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
)

// storeScope holds the parsed common flags for a store subcommand. Unlike `atmos secret`,
// store values are not declared per-component, so a store subcommand only needs these facets
// to scope the underlying Store.Set/Get/Delete calls -- it never resolves a component's stack
// config.
type storeScope struct {
	Stack     string
	Component string
	Identity  string
}

// parseStoreScope reads the persistent flags for the current command. Unlike `atmos secret`'s
// parseScope, --stack/--component are optional here: a store key can be scoped to a
// stack/component or left global (pkg/store/providers.getKey collapses an empty stack/component
// cleanly), so nothing prompts or errors when they are omitted.
func parseStoreScope(cmd *cobra.Command) (storeScope, error) {
	v := viper.GetViper()
	if err := storeParser.BindFlagsToViper(cmd, v); err != nil {
		return storeScope{}, err
	}
	return storeScope{
		Stack:     v.GetString(cfg.StackStr),
		Component: v.GetString("component"),
		Identity:  v.GetString(cfg.IdentityFlagName),
	}, nil
}

// loadService initializes Atmos config, authenticates (if an identity is configured or
// requested), wires the store auth-context resolver, and returns a *pstore.Service over the
// resolved store registry. Unlike `atmos secret`, this never resolves a component's stack
// config, so it skips stack processing (processStacks=false) -- atmosConfig.StoresConfig/
// atmosConfig.Stores are populated earlier in InitCliConfig regardless of this flag; it only
// gates full stack discovery and the stacks.base_path/included_paths validation, neither of
// which `atmos store` needs.
func loadService(scope storeScope) (*pstore.Service, error) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
		ComponentFromArg: scope.Component,
		Stack:            scope.Stack,
	}, false)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrFailedToInitConfig).WithCause(err).Err()
	}

	authManager, err := buildStoreAuthManager(&atmosConfig, scope)
	if err != nil {
		return nil, err
	}
	injectStoreAuthResolver(&atmosConfig, authManager, scope)

	return pstore.NewService(atmosConfig.StoresConfig, atmosConfig.Stores), nil
}

// loadServiceForList initializes Atmos config and returns a *pstore.Service, with no
// authentication attempt. `atmos store list` only reads store configuration and capability
// metadata (see pstore.Service.List) -- it never contacts a backend, so it does not need an
// identity. Skipping auth also avoids an unwanted interactive identity prompt when a config
// declares multiple identities with no default (loadService, used by set/get/delete, does need
// auth: those commands read or write a specific store's value).
func loadServiceForList(scope storeScope) (*pstore.Service, error) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
		ComponentFromArg: scope.Component,
		Stack:            scope.Stack,
	}, false)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrFailedToInitConfig).WithCause(err).Err()
	}

	return pstore.NewService(atmosConfig.StoresConfig, atmosConfig.Stores), nil
}

// buildStoreAuthManager authenticates using the root-level auth config. Unlike `atmos secret`,
// this does not merge a specific component's auth overrides (buildAuthManager in
// cmd/secret/shared.go): store commands may run with no component in scope at all, so there is
// no single component config to resolve auth from.
func buildStoreAuthManager(atmosConfig *schema.AtmosConfiguration, scope storeScope) (auth.AuthManager, error) {
	authManager, err := auth.CreateAndAuthenticateManager(scope.Identity, &atmosConfig.Auth, cfg.IdentityFlagSelectValue)
	if err != nil {
		return nil, err
	}
	if authManager != nil {
		if si := authManager.GetStackInfo(); si != nil {
			si.Stack = scope.Stack
		}
	}
	return authManager, nil
}

// injectStoreAuthResolver wires the auth manager into atmosConfig as the store auth-context
// resolver, mirroring cmd/secret/shared.go's injectSecretStoreAuthResolver. It is a no-op when
// either argument is nil.
func injectStoreAuthResolver(atmosConfig *schema.AtmosConfiguration, authManager auth.AuthManager, scope storeScope) {
	if atmosConfig == nil || authManager == nil {
		return
	}
	resolver := authbridge.NewResolver(authManager, authManager.GetStackInfo())
	atmosConfig.Stores.SetAuthContextResolverWithDefaultIdentity(resolver, storeDefaultIdentity(scope, authManager))
}

// storeDefaultIdentity resolves the effective identity for identity-aware stores without their
// own configured identity: an explicit --identity/ATMOS_IDENTITY (when not a select/disabled
// sentinel), otherwise the identity the auth manager actually authenticated.
func storeDefaultIdentity(scope storeScope, authManager auth.AuthManager) string {
	switch scope.Identity {
	case "", cfg.IdentityFlagSelectValue, cfg.IdentityFlagDisabledValue:
		// Fall back to the authenticated chain below.
	default:
		return scope.Identity
	}
	if chain := authManager.GetChain(); len(chain) > 0 {
		return chain[len(chain)-1]
	}
	return ""
}
