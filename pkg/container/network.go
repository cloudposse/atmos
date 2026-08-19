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

// NetworkConnector is an optional Runtime capability: it attaches an already
// running container to a named network. Used to join Atmos's own current
// container to a stack's dedicated network when CurrentContainerNetwork can't
// reuse its existing network for DNS-alias resolution (e.g. it's only on
// Docker's default "bridge", which doesn't support aliases) — without this,
// Atmos would only be able to guess a reachable address for containers it
// starts, rather than actually being on the same network as them. Runtimes
// that don't implement it simply skip the join; callers then fall back to a
// published-port-based address guess.
type NetworkConnector interface {
	// ConnectNetwork attaches containerID to the named network, registering
	// aliases (if any) as its DNS names on that network. Treats a container
	// already connected to the network as success.
	ConnectNetwork(ctx context.Context, network, containerID string, aliases []string) error
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

// networkConnectResult maps a `network connect` invocation to an idempotent
// result: a container already attached to the network is success, so a repeat
// join (e.g. across separate Atmos processes in the same job container) is a
// no-op rather than an error.
func networkConnectResult(runErr error, output string) error {
	if runErr == nil {
		return nil
	}
	lower := strings.ToLower(output)
	if strings.Contains(lower, "already exists") || strings.Contains(lower, "already connected") || strings.Contains(lower, "already in") {
		return nil
	}
	return fmt.Errorf("%w: connect network: %w: %s", errUtils.ErrContainerRuntimeOperation, runErr, strings.TrimSpace(output))
}
