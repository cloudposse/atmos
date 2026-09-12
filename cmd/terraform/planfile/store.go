package planfile

import (
	_ "github.com/cloudposse/atmos/pkg/ci/artifact/github" // Register github artifact store.
	_ "github.com/cloudposse/atmos/pkg/ci/artifact/local"  // Register local artifact store.
	_ "github.com/cloudposse/atmos/pkg/ci/artifact/s3"     // Register s3 artifact store.
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile/adapter"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// createStore creates a planfile store from configuration.
// Precedence: explicit --store flag > `planfiles.default` > `planfiles.priority` >
// a single named store > environment detection > local storage.
func createStore(atmosConfig *schema.AtmosConfiguration, storeName string) (planfile.Store, error) {
	store, _, err := resolveStore(atmosConfig, storeName)
	return store, err
}

// resolveStore constructs the planfile store and also returns the candidate it was
// built from, so callers that need the resolved backend options (e.g. `list`, which
// reads owner/repo for its GitHub columns) see the store actually in use rather than
// re-deriving the selection.
func resolveStore(atmosConfig *schema.AtmosConfiguration, storeName string) (planfile.Store, planfile.StoreCandidate, error) {
	defer perf.Track(atmosConfig, "planfile.resolveStore")()

	candidates, err := planfile.ResolveStoreCandidates(atmosConfig, storeName)
	if err != nil {
		return nil, planfile.StoreCandidate{}, err
	}

	return adapter.NewStoreFromCandidates(candidates, nil)
}
