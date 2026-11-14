package artifactwire

import (
	"context"

	"github.com/cloudposse/atmos/pkg/artifact/compose"
	"github.com/cloudposse/atmos/pkg/artifact/discovery/catalog"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/bootstrap"
	"github.com/cloudposse/atmos/pkg/schema"
)

// NewCatalogs wires the catalog maintenance service of the configured repositories with the same
// plugin-aware registry authentication as [NewService]. Building it performs no network access,
// credential resolution, or directory creation.
func NewCatalogs(config *schema.AtmosConfiguration) (catalog.Maintainer, error) {
	defer perf.Track(config, "artifactwire.NewCatalogs")()

	composition, err := compose.New(
		config,
		compose.WithRegistryPreparation(func(ctx context.Context) (func(), error) {
			return bootstrap.PrepareAvailable(ctx, config)
		}),
	)
	if err != nil {
		return nil, err
	}
	return composition.Catalogs, nil
}
