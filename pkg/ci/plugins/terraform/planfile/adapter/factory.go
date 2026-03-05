package adapter

import (
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	"github.com/cloudposse/atmos/pkg/perf"
)

// NewStoreFactory returns a planfile.StoreFactory that creates adapter stores
// wrapping the given artifact backend. This enables registry integration so that
// the adapter can be registered as a planfile store type.
func NewStoreFactory(artifactBackend artifact.Store) planfile.StoreFactory {
	defer perf.Track(nil, "adapter.NewStoreFactory")()

	return func(_ planfile.StoreOptions) (planfile.Store, error) {
		return NewStore(artifactBackend), nil
	}
}
