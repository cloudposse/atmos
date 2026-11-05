package exec

import (
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/templating"
)

// gomplateDatasourceFunc reads a datasource through the process-wide cache.
// The cache key includes the alias's URL and headers, so two stack manifests
// that define the same alias differently never see each other's value.
func gomplateDatasourceFunc(alias string, gomplateData *templating.Data, args ...string) (any, error) {
	log.Debug("atmos.GomplateDatasource(): processing datasource", "alias", alias)

	result, err := templating.CachedDatasource(gomplateData, alias, args...)
	if err != nil {
		return nil, err
	}

	log.Debug("atmos.GomplateDatasource(): processed datasource", "alias", alias)

	return result, nil
}
