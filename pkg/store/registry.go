package store

import (
	"fmt"

	log "github.com/cloudposse/atmos/pkg/logger"
)

type StoreRegistry map[string]Store

func NewStoreRegistry(config *StoresConfig) (StoreRegistry, error) {
	registry := make(StoreRegistry)
	for key, storeConfig := range *config {
		switch storeConfig.Type {
		case "artifactory":
			var opts ArtifactoryStoreOptions
			if err := parseOptions(storeConfig.Options, &opts); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrParseArtifactoryOptions, err)
			}

			if storeConfig.Identity != "" {
				log.Warn("Identity-based authentication is not supported for Artifactory stores, identity will be ignored",
					"store", key, "identity", storeConfig.Identity)
			}

			store, err := NewArtifactoryStore(opts)
			if err != nil {
				return nil, err
			}
			registry[key] = store

		case "azure-key-vault":
			var opts AzureKeyVaultStoreOptions
			if err := parseOptions(storeConfig.Options, &opts); err != nil {
				return nil, fmt.Errorf("failed to parse Key Vault store options: %w", err)
			}

			store, err := NewAzureKeyVaultStore(opts, storeConfig.Identity)
			if err != nil {
				return nil, err
			}
			registry[key] = store

		case "aws-ssm-parameter-store":
			var opts SSMStoreOptions
			if err := parseOptions(storeConfig.Options, &opts); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrParseSSMOptions, err)
			}

			store, err := NewSSMStore(opts, storeConfig.Identity)
			if err != nil {
				return nil, err
			}
			registry[key] = store

		case "google-secret-manager", "gsm":
			var opts GSMStoreOptions
			if err := parseOptions(storeConfig.Options, &opts); err != nil {
				return nil, fmt.Errorf("failed to parse Google Secret Manager store options: %w", err)
			}

			store, err := NewGSMStore(opts, storeConfig.Identity)
			if err != nil {
				return nil, err
			}
			registry[key] = store

		case "redis":
			var opts RedisStoreOptions
			if err := parseOptions(storeConfig.Options, &opts); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrParseRedisOptions, err)
			}

			if storeConfig.Identity != "" {
				log.Warn("Identity-based authentication is not supported for Redis stores, identity will be ignored",
					"store", key, "identity", storeConfig.Identity)
			}

			store, err := NewRedisStore(opts)
			if err != nil {
				return nil, err
			}
			registry[key] = store

		default:
			return nil, fmt.Errorf("%w: %s", ErrStoreTypeNotFound, storeConfig.Type)
		}
	}

	return registry, nil
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
