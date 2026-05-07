package artifact

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// EnvironmentChecker determines whether a storage backend is available
// in the current environment (e.g., AWS credentials present, GitHub token set).
type EnvironmentChecker interface {
	// IsAvailable returns true if the backend can be used in the current environment.
	IsAvailable(ctx context.Context) bool
}

// SelectStore selects the first available backend from the priority list,
// or uses the explicit storeName override if provided.
// Returns the store name and configured Store, or an error if none is available.
func SelectStore(
	ctx context.Context,
	priority []string,
	stores map[string]StoreOptions,
	checkers map[string]EnvironmentChecker,
	storeName string,
	atmosConfig *schema.AtmosConfiguration,
) (Store, error) {
	defer perf.Track(atmosConfig, "artifact.SelectStore")()

	// Explicit override: use the named store directly.
	if storeName != "" {
		opts, ok := stores[storeName]
		if !ok {
			return nil, fmt.Errorf("%w: store %q not configured", errUtils.ErrArtifactStoreNotFound, storeName)
		}
		opts.AtmosConfig = atmosConfig
		return NewStore(opts)
	}

	// Priority-based selection: try each backend in order.
	for _, name := range priority {
		// Check if the backend is available via its environment checker.
		if checker, ok := checkers[name]; ok {
			if !checker.IsAvailable(ctx) {
				continue
			}
		}

		opts, ok := stores[name]
		if !ok {
			continue
		}
		opts.AtmosConfig = atmosConfig
		return NewStore(opts)
	}

	return nil, fmt.Errorf("%w: no available artifact store found in priority list %v", errUtils.ErrArtifactStoreNotFound, priority)
}
