package store

import (
	"fmt"

	log "github.com/cloudposse/atmos/pkg/logger"
)

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

// mapLegacyType translates a legacy `type` value to its canonical `kind`. Unknown values are
// returned unchanged so the switch can still match a kind passed directly via `type`.
func mapLegacyType(legacyType string) string {
	switch legacyType {
	case "artifactory":
		return KindArtifactory
	case "azure-key-vault":
		return KindAzureKeyVault
	case "aws-ssm-parameter-store":
		return KindAWSSSM
	case "aws-secrets-manager":
		return KindAWSASM
	case "google-secret-manager", "gsm":
		return KindGCPSecret
	case "hashicorp-vault":
		return KindHashicorpVault
	case "redis":
		return KindRedis
	case "1password", "onepassword":
		return KindOnePassword
	case "keychain", "keyring":
		return KindKeychain
	case "github-actions":
		return KindGitHubActions
	default:
		return legacyType
	}
}

// resolveKind returns the normalized backend kind for a store config. An explicit `kind`
// takes precedence; otherwise the legacy `type` is mapped to a kind.
func resolveKind(storeConfig StoreConfig) string {
	if storeConfig.Kind != "" {
		return mapLegacyType(storeConfig.Kind)
	}
	return mapLegacyType(storeConfig.Type)
}

func NewStoreRegistry(config *StoresConfig) (StoreRegistry, error) {
	registry := make(StoreRegistry)
	for key, storeConfig := range *config {
		s, err := newStore(key, storeConfig)
		if err != nil {
			return nil, err
		}
		// A `secret: true` store writes the sensitive at-rest variant when supported.
		if storeConfig.Secret {
			if sas, ok := s.(SecretAwareStore); ok {
				sas.SetSecret(true)
			}
		}
		registry[key] = s
	}

	return registry, nil
}

// storeBuilder constructs a store from its configuration.
type storeBuilder func(key string, storeConfig StoreConfig) (Store, error)

// storeBuilders maps each normalized backend kind to its constructor. Using a table keeps the
// factory flat and extensible (and avoids a high-complexity switch).
var storeBuilders = map[string]storeBuilder{
	KindArtifactory:    buildArtifactoryStore,
	KindAzureKeyVault:  buildAzureKeyVaultStore,
	KindAWSSSM:         buildSSMStore,
	KindAWSASM:         buildSecretsManagerStore,
	KindGCPSecret:      buildGSMStore,
	KindHashicorpVault: buildVaultStore,
	KindRedis:          buildRedisStore,
	KindOnePassword:    buildOnePasswordStore,
	KindKeychain:       buildKeychainStore,
	KindGitHubActions:  buildGitHubActionsStore,
}

// newStore constructs a single store from its configuration, dispatching on the normalized kind.
func newStore(key string, storeConfig StoreConfig) (Store, error) {
	builder, ok := storeBuilders[resolveKind(storeConfig)]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrStoreTypeNotFound, storeConfig.Type)
	}
	return builder(key, storeConfig)
}

func buildArtifactoryStore(key string, storeConfig StoreConfig) (Store, error) {
	var opts ArtifactoryStoreOptions
	if err := parseOptions(storeConfig.Options, &opts); err != nil {
		return nil, fmt.Errorf(errParseFmt, ErrParseArtifactoryOptions, err)
	}
	warnIdentityIgnored(key, storeConfig, "Artifactory")
	return NewArtifactoryStore(opts)
}

func buildAzureKeyVaultStore(_ string, storeConfig StoreConfig) (Store, error) {
	var opts AzureKeyVaultStoreOptions
	if err := parseOptions(storeConfig.Options, &opts); err != nil {
		return nil, fmt.Errorf(errParseFmt, ErrParseAzureKeyVaultOptions, err)
	}
	return NewAzureKeyVaultStore(opts, storeConfig.Identity)
}

func buildSSMStore(_ string, storeConfig StoreConfig) (Store, error) {
	var opts SSMStoreOptions
	if err := parseOptions(storeConfig.Options, &opts); err != nil {
		return nil, fmt.Errorf(errParseFmt, ErrParseSSMOptions, err)
	}
	return NewSSMStore(opts, storeConfig.Identity)
}

func buildSecretsManagerStore(_ string, storeConfig StoreConfig) (Store, error) {
	var opts SecretsManagerStoreOptions
	if err := parseOptions(storeConfig.Options, &opts); err != nil {
		return nil, fmt.Errorf(errParseFmt, ErrParseSecretsManagerOptions, err)
	}
	return NewSecretsManagerStore(opts, storeConfig.Identity)
}

func buildGSMStore(_ string, storeConfig StoreConfig) (Store, error) {
	var opts GSMStoreOptions
	if err := parseOptions(storeConfig.Options, &opts); err != nil {
		return nil, fmt.Errorf(errParseFmt, ErrParseGSMOptions, err)
	}
	return NewGSMStore(opts, storeConfig.Identity)
}

func buildVaultStore(_ string, storeConfig StoreConfig) (Store, error) {
	var opts VaultStoreOptions
	if err := parseOptions(storeConfig.Options, &opts); err != nil {
		return nil, fmt.Errorf(errParseFmt, ErrParseVaultOptions, err)
	}
	return NewVaultStore(&opts, storeConfig.Identity)
}

func buildOnePasswordStore(key string, storeConfig StoreConfig) (Store, error) {
	var opts OnePasswordStoreOptions
	if err := parseOptions(storeConfig.Options, &opts); err != nil {
		return nil, fmt.Errorf(errParseFmt, ErrParseOnePasswordOptions, err)
	}
	warnIdentityIgnored(key, storeConfig, "1Password")
	return NewOnePasswordStore(&opts)
}

func buildKeychainStore(key string, storeConfig StoreConfig) (Store, error) {
	var opts KeychainStoreOptions
	if err := parseOptions(storeConfig.Options, &opts); err != nil {
		return nil, fmt.Errorf(errParseFmt, ErrParseKeychainOptions, err)
	}
	warnIdentityIgnored(key, storeConfig, "Keychain")
	return NewKeychainStore(opts)
}

func buildGitHubActionsStore(key string, storeConfig StoreConfig) (Store, error) {
	var opts GitHubActionsStoreOptions
	if err := parseOptions(storeConfig.Options, &opts); err != nil {
		return nil, fmt.Errorf(errParseFmt, ErrParseGitHubActionsOptions, err)
	}
	warnIdentityIgnored(key, storeConfig, "GitHub Actions")
	return NewGitHubActionsStore(opts)
}

func buildRedisStore(key string, storeConfig StoreConfig) (Store, error) {
	var opts RedisStoreOptions
	if err := parseOptions(storeConfig.Options, &opts); err != nil {
		return nil, fmt.Errorf(errParseFmt, ErrParseRedisOptions, err)
	}
	warnIdentityIgnored(key, storeConfig, "Redis")
	return NewRedisStore(opts)
}

// warnIdentityIgnored logs a warning when an identity is configured for a store type that does
// not support identity-based authentication.
func warnIdentityIgnored(key string, storeConfig StoreConfig, storeType string) {
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
