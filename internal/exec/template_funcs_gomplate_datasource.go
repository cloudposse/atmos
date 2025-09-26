package exec

import (
	"sync"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/hairyhenderson/gomplate/v3/data"
)

var gomplateDatasourceFuncSyncMap = sync.Map{}

func gomplateDatasourceFunc(alias string, gomplateData *data.Data, args ...string) (any, error) {
	log.Debug("atmos.GomplateDatasource(): processing datasource", "alias", alias)

	// If the result for the alias already exists in the cache, return it
	existingResult, found := gomplateDatasourceFuncSyncMap.Load(alias)
	if found && existingResult != nil {
		return existingResult, nil
	}

	result, err := gomplateData.Datasource(alias, args...)
	if err != nil {
		return nil, err
	}

	// Cache the result
	gomplateDatasourceFuncSyncMap.Store(alias, result)

	log.Debug("atmos.GomplateDatasource(): processed datasource", "alias", alias, "result", result)

	return result, nil
}
