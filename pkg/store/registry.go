package store

import "fmt"

type StoreRegistry map[string]Store

func NewStoreRegistry(config *StoresConfig) (StoreRegistry, error) {
	registry := make(StoreRegistry)
	for key, storeConfig := range *config {
		switch storeConfig.Type {
		case "aws-ssm-parameter-store":
			var opts SSMStoreOptions
			if err := parseOptions(storeConfig.Options, &opts); err != nil {
				return nil, fmt.Errorf("failed to parse SSM store options: %w", err)
			}

			store, err := NewSSMStore(opts)
			if err != nil {
				return nil, err
			}
			registry[key] = store

		case "in-memory":
			store, err := NewInMemoryStore(storeConfig.Options)
			if err != nil {
				return nil, err
			}
			registry[key] = store

		default:
			return nil, fmt.Errorf("store type %s not found", storeConfig.Type)
		}
	}

	return registry, nil
}
