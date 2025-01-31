package exec

import (
	"fmt"
	"sync"

	u "github.com/cloudposse/atmos/pkg/utils"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/hairyhenderson/gomplate/v3/data"
)

var gomplateDatasourceFuncSyncMap = sync.Map{}

func gomplateDatasourceFunc(atmosConfig schema.AtmosConfiguration, alias string, gomplateData *data.Data, args ...string) (any, error) {
	u.LogTrace(fmt.Sprintf("atmos.GomplateDatasource(): processing datasource alias '%s'", alias))

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

	u.LogTrace(fmt.Sprintf("atmos.GomplateDatasource(): processed datasource alias '%s'.\nResult: '%v'", alias, result))

	return result, nil
}
