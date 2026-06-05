package providers

import (
	"fmt"

	"github.com/go-viper/mapstructure/v2"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/store"
)

// Error format constants shared across store provider implementations.
const (
	errFormat           = "%w: %v"
	errWrapFormat       = "%w: %s"
	errWrapFormatWithID = "%w '%s': %s"
)

// parseOptions decodes a raw options map into the typed options struct for a store backend.
func parseOptions(options map[string]interface{}, target interface{}) error {
	return mapstructure.Decode(options, target)
}

// NewStoreRegistry creates a store registry from the provided stores config,
// instantiating the concrete backend implementation for each configured store.
func NewStoreRegistry(config *store.StoresConfig) (store.StoreRegistry, error) {
	registry := make(store.StoreRegistry)
	for key, storeConfig := range *config {
		s, err := newStore(key, storeConfig)
		if err != nil {
			return nil, err
		}
		registry[key] = s
	}

	return registry, nil
}

// newStore dispatches to the backend-specific constructor based on the store type.
func newStore(key string, storeConfig store.StoreConfig) (store.Store, error) {
	switch storeConfig.Type {
	case "artifactory":
		return buildArtifactoryStore(key, storeConfig)
	case "azure-key-vault":
		return buildAzureKeyVaultStore(storeConfig)
	case "aws-ssm-parameter-store":
		return buildSSMStore(storeConfig)
	case "google-secret-manager", "gsm":
		return buildGSMStore(storeConfig)
	case "redis":
		return buildRedisStore(key, storeConfig)
	default:
		return nil, fmt.Errorf(errWrapFormat, store.ErrStoreTypeNotFound, storeConfig.Type)
	}
}

func buildArtifactoryStore(key string, storeConfig store.StoreConfig) (store.Store, error) {
	var opts ArtifactoryStoreOptions
	if err := parseOptions(storeConfig.Options, &opts); err != nil {
		return nil, fmt.Errorf(errFormat, store.ErrParseArtifactoryOptions, err)
	}

	if storeConfig.Identity != "" {
		log.Warn("Identity-based authentication is not supported for Artifactory stores, identity will be ignored",
			"store", key, "identity", storeConfig.Identity)
	}

	return NewArtifactoryStore(opts)
}

func buildAzureKeyVaultStore(storeConfig store.StoreConfig) (store.Store, error) {
	var opts AzureKeyVaultStoreOptions
	if err := parseOptions(storeConfig.Options, &opts); err != nil {
		return nil, fmt.Errorf("failed to parse Key Vault store options: %w", err)
	}

	return NewAzureKeyVaultStore(opts, storeConfig.Identity)
}

func buildSSMStore(storeConfig store.StoreConfig) (store.Store, error) {
	var opts SSMStoreOptions
	if err := parseOptions(storeConfig.Options, &opts); err != nil {
		return nil, fmt.Errorf(errFormat, store.ErrParseSSMOptions, err)
	}

	return NewSSMStore(opts, storeConfig.Identity)
}

func buildGSMStore(storeConfig store.StoreConfig) (store.Store, error) {
	var opts GSMStoreOptions
	if err := parseOptions(storeConfig.Options, &opts); err != nil {
		return nil, fmt.Errorf("failed to parse Google Secret Manager store options: %w", err)
	}

	return NewGSMStore(opts, storeConfig.Identity)
}

func buildRedisStore(key string, storeConfig store.StoreConfig) (store.Store, error) {
	var opts RedisStoreOptions
	if err := parseOptions(storeConfig.Options, &opts); err != nil {
		return nil, fmt.Errorf(errFormat, store.ErrParseRedisOptions, err)
	}

	if storeConfig.Identity != "" {
		log.Warn("Identity-based authentication is not supported for Redis stores, identity will be ignored",
			"store", key, "identity", storeConfig.Identity)
	}

	return NewRedisStore(opts)
}
