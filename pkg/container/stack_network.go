package container

import (
	"context"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// StackNetworkName is the per-stack user-defined network every
// components.container instance and emulator in a stack joins, so peers
// resolve each other by name (container DNS). Derived from the stack alone and
// sanitized to a valid docker/podman network name.
func StackNetworkName(stack string) string {
	defer perf.Track(nil, "container.StackNetworkName")()

	return "atmos-" + sanitizeNetworkToken(stack)
}

// StackNetworkAlias is the DNS alias a component or emulator registers on the
// stack's shared network.
func StackNetworkAlias(stack, name string) string {
	defer perf.Track(nil, "container.StackNetworkAlias")()

	return sanitizeNetworkToken(stack + "-" + name)
}

// sanitizeNetworkToken reduces a string to characters valid in a docker/podman
// network name ([a-zA-Z0-9_.-]); any other rune becomes '-'.
func sanitizeNetworkToken(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '.', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "default"
	}
	return b.String()
}

// HasExplicitNetworkOverride reports whether runArgs already sets an explicit
// --network (e.g. --network=host, --network none, --network container:x, or a
// user's own network name). Callers should skip AttachSharedNetwork when this
// is true and respect the user's choice instead: Docker/Podman reject combining
// a network mode like host/none with an additional --network attachment, so
// injecting the shared network alongside an explicit override would break the
// container create outright, not just be redundant.
func HasExplicitNetworkOverride(runArgs []string) bool {
	defer perf.Track(nil, "container.HasExplicitNetworkOverride")()

	for _, arg := range runArgs {
		if arg == "--network" || strings.HasPrefix(arg, "--network=") {
			return true
		}
	}
	return false
}

// AttachSharedNetwork best-effort joins networks to the stack's shared network
// with name as a DNS alias, so peers (other components.container instances and
// emulators in the same stack) can resolve it by name. Prefers Atmos's own
// current container network when detected (see CurrentContainerNetwork), so a
// job container that starts a sibling container can still reach it; otherwise
// idempotently ensures the dedicated per-stack network. It is a no-op when the
// runtime doesn't implement NetworkEnsurer (e.g. a test mock) or network
// creation fails -- single-container use still works over the default bridge,
// only cross-container name resolution is lost.
func AttachSharedNetwork(ctx context.Context, runtime Runtime, networks *[]NetworkAttachment, stack, name string) {
	defer perf.Track(nil, "container.AttachSharedNetwork")()

	alias := StackNetworkAlias(stack, name)
	if network := CurrentContainerNetwork(ctx, runtime); network != "" {
		*networks = append(*networks, NetworkAttachment{
			Name:    network,
			Aliases: []string{alias},
		})
		return
	}

	ensurer, ok := runtime.(NetworkEnsurer)
	if !ok {
		return
	}
	network := StackNetworkName(stack)
	if err := ensurer.EnsureNetwork(ctx, network); err != nil {
		log.Debug("shared stack network unavailable; containers will not resolve each other by name",
			"network", network, "error", err)
		return
	}
	*networks = append(*networks, NetworkAttachment{
		Name:    network,
		Aliases: []string{alias},
	})
}
