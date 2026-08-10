package container

import (
	"context"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

// NetworkEnsurer is an optional Runtime capability: it idempotently creates a
// user-defined container network. Emulators in the same stack join one such
// network so they can reach each other by component name (container DNS) — e.g. a
// GitOps controller running in the k3s emulator resolving the Gitea emulator at
// `http://gitserver:3000`. Runtimes that don't implement it simply skip the
// shared network; containers then fall back to the default bridge, where
// cross-container name resolution is unavailable (single-emulator use is
// unaffected, since host port publishing still works).
type NetworkEnsurer interface {
	// EnsureNetwork creates the named user-defined network, treating an
	// already-existing network as success.
	EnsureNetwork(ctx context.Context, name string) error
}

// networkCreateResult maps a `network create` invocation to an idempotent result:
// an "already exists" failure is success, so repeated `up`s are no-ops.
func networkCreateResult(runErr error, output string) error {
	if runErr == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(output), "already exists") {
		return nil
	}
	return fmt.Errorf("%w: create network: %w: %s", errUtils.ErrContainerRuntimeOperation, runErr, strings.TrimSpace(output))
}
