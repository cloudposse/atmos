package exec

import (
	"fmt"
	"sync"

	log "github.com/cloudposse/atmos/pkg/logger"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

var storeFuncSyncMap = sync.Map{}

func storeFunc(
	atmosConfig *schema.AtmosConfiguration,
	storeName string,
	stack string,
	component string,
	key string,
) (any, error) {
	functionName := fmt.Sprintf("atmos.Store(%s, %s, %s, %s)", storeName, stack, component, key)
	slug := fmt.Sprintf("%s-%s-%s-%s", storeName, stack, component, key)

	log.Debug("Executing template function", "function", functionName)

	// If the result for the component in the stack already exists in the cache, return it
	existing, found := storeFuncSyncMap.Load(slug)
	if found && existing != nil {
		log.Debug("Cache hit for template function", "function", functionName, "result", existing)
		return existing, nil
	}

	// Retrieve the store from atmosConfig
	store := atmosConfig.Stores[storeName]

	if store == nil {
		return nil, fmt.Errorf("%w: %s\nstore '%s' not found", errUtils.ErrInvalidTemplateFunc, functionName, storeName)
	}

	// Retrieve the value from the store
	value, err := store.Get(stack, component, key)
	if err != nil {
		value = nil
	}

	// Cache the result
	storeFuncSyncMap.Store(slug, value)

	log.Debug("Executed template function", "function", functionName, "result", value)

	return value, nil
}
