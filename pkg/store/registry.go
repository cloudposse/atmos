package store

import (
	"fmt"
	"sync"

	log "github.com/cloudposse/atmos/pkg/logger"
)

// StoreRegistry is a map of store name to a live store implementation.
type StoreRegistry map[string]Store

// Backend kind constants (cloud/thing vocabulary, shared with the secrets subsystem).
const (
	KindArtifactory    = "artifactory"
	KindAzureKeyVault  = "azure/keyvault"
	KindAWSSSM         = "aws/ssm"
	KindAWSASM         = "aws/asm"
	KindGCPSecret      = "gcp/secretmanager"
	KindHashicorpVault = "hashicorp/vault"
	KindRedis          = "redis"
	KindOnePassword    = "onepassword"
	KindKeychain       = "keychain"
	KindGitHubActions  = "github/actions"
)

// secretByDefaultKinds are backends that are secret managers by nature: a store of one of these
// kinds is treated as `secret: true` even when the config omits it (see ApplySecretDefaults).
var secretByDefaultKinds = map[string]bool{
	KindOnePassword:   true,
	KindKeychain:      true,
	KindGitHubActions: true,
}

// isSecretByDefaultKind reports whether a backend kind defaults to a secret store.
func isSecretByDefaultKind(kind string) bool {
	return secretByDefaultKinds[kind]
}

// secretIncapableKinds are backends that cannot encrypt values at rest (e.g. Redis is an
// in-memory cache, Artifactory is an artifact repo). Marking a store of one of these kinds
// `secret: true` is a configuration error: it would store secrets in plaintext. Add a kind
// here when it has no at-rest encryption.
var secretIncapableKinds = map[string]bool{
	KindRedis:       true,
	KindArtifactory: true,
}

// isSecretIncapableKind reports whether a backend kind cannot be used as a secret store
// because it does not encrypt values at rest.
func isSecretIncapableKind(kind string) bool {
	return secretIncapableKinds[kind]
}

// ApplySecretDefaults marks secret-by-default backends (e.g. 1Password) as `secret: true` when
// the config didn't set it. It mutates the config in place so both the store registry and the
// secrets subsystem (which reads StoreConfig.Secret) agree on subsystem membership. Call it once
// after loading the stores config and before building the registry.
func ApplySecretDefaults(config StoresConfig) {
	for key, cfg := range config {
		if !cfg.Secret && isSecretByDefaultKind(resolveKind(cfg)) {
			cfg.Secret = true
			config[key] = cfg
		}
	}
}

// legacyTypeToKind maps each legacy `type` value to its canonical `kind`.
var legacyTypeToKind = map[string]string{
	"artifactory":             KindArtifactory,
	"azure-key-vault":         KindAzureKeyVault,
	"aws-ssm-parameter-store": KindAWSSSM,
	"aws-secrets-manager":     KindAWSASM,
	"google/secretmanager":    KindGCPSecret,
	"google-secret-manager":   KindGCPSecret,
	"gsm":                     KindGCPSecret,
	"hashicorp-vault":         KindHashicorpVault,
	"redis":                   KindRedis,
	"1password":               KindOnePassword,
	"onepassword":             KindOnePassword,
	"keychain":                KindKeychain,
	"keyring":                 KindKeychain,
	"github-actions":          KindGitHubActions,
}

// mapLegacyType translates a legacy `type` value to its canonical `kind`. Unknown values are
// returned unchanged so the caller can still match a kind passed directly via `type`.
func mapLegacyType(legacyType string) string {
	if kind, ok := legacyTypeToKind[legacyType]; ok {
		return kind
	}
	return legacyType
}

// resolveKind returns the normalized backend kind for a store config. An explicit `kind`
// takes precedence; otherwise the legacy `type` is mapped to a kind.
func resolveKind(storeConfig StoreConfig) string {
	if storeConfig.Kind != "" {
		return mapLegacyType(storeConfig.Kind)
	}
	return mapLegacyType(storeConfig.Type)
}

// StoreFactory builds a store backend from its configuration. Provider packages
// register a factory for each backend kind they implement via Register, typically
// from an init() function. The name is the configured store's key and is used
// only for diagnostics (e.g. warnings).
type StoreFactory func(name string, config StoreConfig) (Store, error)

// storeFactories holds the registered factory for each backend kind. It is
// populated at init time by provider packages and read at registry-build time.
// The storeFactoriesMu mutex guards it because Register is exported and may be
// called concurrently with NewStoreRegistry (e.g. late registration from tests
// or optional providers).
var (
	storeFactoriesMu sync.RWMutex
	storeFactories   = map[string]StoreFactory{}
)

// Register associates a backend kind (e.g. KindRedis) with the factory that
// builds it. Provider packages call this from their init() functions, so
// importing a provider package — typically via a blank import of
// pkg/store/providers — makes its store kinds available to NewStoreRegistry.
// Register under the canonical kind; legacy `type` values are mapped to a kind
// by resolveKind before the factory is looked up.
//
// It panics if the same kind is registered twice, which indicates a programming
// error (two factories claiming the same kind).
func Register(kind string, factory StoreFactory) {
	storeFactoriesMu.Lock()
	defer storeFactoriesMu.Unlock()

	if _, exists := storeFactories[kind]; exists {
		panic(fmt.Sprintf("store: factory already registered for kind %q", kind))
	}

	storeFactories[kind] = factory
}

// Reset clears all registered store factories.
//
// WARNING: This function is for TESTING ONLY. It should never be called in
// production code. It lets tests start from a clean registry state.
func Reset() {
	storeFactoriesMu.Lock()
	defer storeFactoriesMu.Unlock()

	storeFactories = map[string]StoreFactory{}
}

// NewStoreRegistry builds a registry of live stores from the provided config,
// resolving each configured store to a canonical kind and looking it up in the
// factories registered by the provider packages. Import the provider package
// (e.g. with a blank import of pkg/store/providers) so the built-in backends are
// registered before this runs.
func NewStoreRegistry(config *StoresConfig) (StoreRegistry, error) {
	registry := make(StoreRegistry)
	for name, storeConfig := range *config {
		kind := resolveKind(storeConfig)

		// Fail fast on a misconfiguration: marking a backend that cannot encrypt at rest as
		// `secret: true` is rejected before the store is ever constructed.
		if storeConfig.Secret && isSecretIncapableKind(kind) {
			return nil, fmt.Errorf("%w: store %q uses backend %q", ErrSecretBackendNotEncrypted, name, kind)
		}

		storeFactoriesMu.RLock()
		factory, ok := storeFactories[kind]
		storeFactoriesMu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrStoreTypeNotFound, kind)
		}

		s, err := factory(name, storeConfig)
		if err != nil {
			return nil, err
		}

		// A `secret: true` store writes the sensitive at-rest variant when supported.
		if storeConfig.Secret {
			if sas, ok := s.(SecretAwareStore); ok {
				sas.SetSecret(true)
			}
		}

		registry[name] = s
	}

	return registry, nil
}

// WarnIdentityIgnored logs a warning when an identity is configured for a store type that does
// not support identity-based authentication. Provider factories call it for non-identity-aware
// backends so a misconfigured `identity` is surfaced rather than silently ignored.
func WarnIdentityIgnored(key string, storeConfig StoreConfig, storeType string) {
	if storeConfig.Identity != "" {
		log.Warn("Identity-based authentication is not supported for this store type, identity will be ignored",
			"store", key, "type", storeType, "identity", storeConfig.Identity)
	}
}

// SetAuthContextResolver injects an auth context resolver into all identity-aware stores
// that have an identity configured. This should be called after authentication is complete
// and before stores are accessed.
func (r StoreRegistry) SetAuthContextResolver(resolver AuthContextResolver) {
	for _, s := range r {
		if ias, ok := s.(IdentityAwareStore); ok {
			// Pass empty identity name — the store already has its identity name from construction.
			// SetAuthContext will only update the resolver, not override a non-empty identity.
			ias.SetAuthContext(resolver, "")
		}
	}
}

// SetAuthContextResolverWithDefaultIdentity injects an auth context resolver into
// identity-aware stores. Stores with their own configured identity keep it; eligible
// store types without a configured identity inherit defaultIdentity.
func (r StoreRegistry) SetAuthContextResolverWithDefaultIdentity(resolver AuthContextResolver, defaultIdentity string) {
	for _, s := range r {
		ias, ok := s.(IdentityAwareStore)
		if !ok {
			continue
		}
		ias.SetAuthContext(resolver, defaultIdentityForStore(s, defaultIdentity))
	}
}

func defaultIdentityForStore(s Store, defaultIdentity string) string {
	if defaultIdentity == "" {
		return ""
	}

	identified, ok := s.(interface {
		IdentityName() string
	})
	if !ok {
		return ""
	}
	if identified.IdentityName() == "" {
		return defaultIdentity
	}

	return ""
}
