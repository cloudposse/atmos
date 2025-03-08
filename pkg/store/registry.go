package store

import (
	"fmt"
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

			store, err := NewArtifactoryStore(opts)
			if err != nil {
				return nil, err
			}
			registry[key] = store

		case "aws-ssm-parameter-store":
			var opts SSMStoreOptions
			if err := parseOptions(storeConfig.Options, &opts); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrParseSSMOptions, err)
			}

			store, err := NewSSMStore(opts)
			if err != nil {
				return nil, err
			}
			registry[key] = store

		case "google-secret-manager", "gsm":
			var opts GSMStoreOptions
			if err := parseOptions(storeConfig.Options, &opts); err != nil {
				return nil, fmt.Errorf("failed to parse Google Secret Manager store options: %w", err)
			}

			store, err := NewGSMStore(opts)
			if err != nil {
				return nil, err
			}
			registry[key] = store

		case "redis":
			var opts RedisStoreOptions
			if err := parseOptions(storeConfig.Options, &opts); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrParseRedisOptions, err)
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
