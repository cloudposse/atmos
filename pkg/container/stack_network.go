package container

import (
	"context"
	"hash/fnv"
	"strconv"
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
// stack's shared network. It is <stack>-<name>, e.g. "dev-api" -- readable by
// design, so it intentionally omits the component kind (container, emulator, or
// workflow step): a components.container instance and an emulator sharing both a
// stack and a name is a narrow, user-controlled edge case (rename one of them to
// avoid it) that isn't worth trading away the readable, predictable format for.
func StackNetworkAlias(stack, name string) string {
	defer perf.Track(nil, "container.StackNetworkAlias")()

	return sanitizeNetworkToken(stack + "-" + name)
}

// sanitizeNetworkToken reduces a string to characters valid in a docker/podman
// network name ([a-zA-Z0-9_.-]); any other rune becomes '-'. Distinct inputs that
// require substitution (e.g. a stack name containing "/") get a short stable hash
// suffix of the original, pre-sanitization input appended, so two different
// original values -- like "deploy/prod" and "deploy-prod" -- that would otherwise
// both sanitize to "deploy-prod" no longer collide on the same network/alias.
// Inputs that need no substitution are left exactly as-is: the common case (stack
// and component names using only valid characters) keeps its short, readable form.
func sanitizeNetworkToken(s string) string {
	var b strings.Builder
	substituted := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '.', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
			substituted = true
		}
	}
	if b.Len() == 0 {
		return "default"
	}
	if !substituted {
		return b.String()
	}
	return b.String() + "-" + hashSuffix(s)
}

// hashSuffixBase is the number base used to render hashSuffix's output, chosen for
// a short, all-lowercase-alphanumeric suffix that's still valid in a docker/podman
// network name.
const hashSuffixBase = 36

// hashSuffix returns a short, stable, non-cryptographic hash of s, used purely to
// disambiguate sanitized tokens -- not for any security purpose.
func hashSuffix(s string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s)) // fnv's Write never returns an error.
	return strconv.FormatUint(uint64(h.Sum32()), hashSuffixBase)
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
