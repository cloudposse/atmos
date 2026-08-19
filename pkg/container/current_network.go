package container

import (
	"context"
	"os"
	"strings"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

// envUseCurrentContainerNetwork opts into (or out of) preferring Atmos's own
// current container network over creating a dedicated one. Kept under its
// original "EMULATOR" name for backward compatibility even though this now
// also governs components.container -- only the Go symbols generalized.
const envUseCurrentContainerNetwork = "ATMOS_EMULATOR_USE_CURRENT_CONTAINER_NETWORK"

var (
	// ProcessRunsInContainer reports whether the current Atmos process itself is
	// running inside a container (Docker/Podman/containerd/Kubernetes). Overridable
	// in tests.
	ProcessRunsInContainer = defaultProcessRunsInContainer
	readProcFile           = os.ReadFile
	// Wraps os.Hostname, overridable in tests so the "hostname can't be
	// determined" fallback in CurrentContainerNetwork is exercisable without
	// depending on an actual OS-level failure.
	currentHostname = os.Hostname
	// Wraps the os.Stat-based marker-file check used by
	// defaultProcessRunsInContainer, overridable in tests so the
	// /.dockerenv and /run/.containerenv short-circuits are exercisable
	// without depending on whether the test host itself is containerized.
	markerFileExists = defaultMarkerFileExists
)

// CurrentContainerNetwork returns the network Atmos's own process is attached
// to, when it is itself running in a container and PreferCurrentContainerNetwork
// allows it -- so a job container that starts a sibling container can attach it
// to the same network and reach it. Returns "" whenever the network can't be
// determined (not containerized, opted out, no hostname, inspect failure, or no
// usable network); callers treat that as a signal to fall back to a dedicated network.
func CurrentContainerNetwork(ctx context.Context, runtime Runtime) string {
	defer perf.Track(nil, "container.CurrentContainerNetwork")()

	if !PreferCurrentContainerNetwork() || runtime == nil {
		return ""
	}
	hostname, err := currentHostname()
	if err != nil || hostname == "" {
		return ""
	}
	info, err := runtime.Inspect(ctx, hostname)
	if err != nil || info == nil {
		return ""
	}
	return firstReachableNetwork(info.Networks)
}

// PreferCurrentContainerNetwork reports whether callers should try
// CurrentContainerNetwork before falling back to a dedicated per-stack network.
// An explicit envUseCurrentContainerNetwork override always wins -- an enabled
// override returns true directly, without also requiring ProcessRunsInContainer's
// heuristic to agree, since the user has explicitly asserted the current-container-
// network path should be used. Without an override, it defaults to whether Atmos
// itself is detected as running inside a container -- CurrentContainerNetwork
// safely returns "" when the network can't actually be determined (e.g. a wrong
// override on a non-containerized host), so callers fall back to the dedicated
// network automatically either way.
func PreferCurrentContainerNetwork() bool {
	defer perf.Track(nil, "container.PreferCurrentContainerNetwork")()

	switch strings.ToLower(strings.TrimSpace(envString(envUseCurrentContainerNetwork))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return ProcessRunsInContainer()
}

func envString(name string) string {
	defer perf.Track(nil, "container.envString")()

	_ = viper.BindEnv(name, name)
	return viper.GetString(name)
}

// firstReachableNetwork returns the first network name that isn't empty,
// "host", "none", or "bridge" -- those aren't usable for container-to-container
// DNS. "bridge" specifically is Docker's default network, which (unlike a
// user-defined bridge network) doesn't support embedded DNS/--network-alias
// resolution at all, so picking it would make AttachSharedNetwork report
// success while name resolution silently never works.
func firstReachableNetwork(networks []string) string {
	defer perf.Track(nil, "container.firstReachableNetwork")()

	for _, network := range networks {
		switch network {
		case "", "host", "none", "bridge":
			continue
		default:
			return network
		}
	}
	return ""
}

// defaultProcessRunsInContainer detects containerization via the conventional
// marker files/cgroup hints, without relying on any single container runtime's
// specific behavior.
func defaultProcessRunsInContainer() bool {
	defer perf.Track(nil, "container.defaultProcessRunsInContainer")()

	if markerFileExists("/.dockerenv") {
		return true
	}
	if markerFileExists("/run/.containerenv") {
		return true
	}

	data, err := readProcFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	cgroup := string(data)
	for _, marker := range []string{"docker", "containerd", "kubepods", "libpod"} {
		if strings.Contains(cgroup, marker) {
			return true
		}
	}
	return false
}

// defaultMarkerFileExists reports whether path exists on disk, used to detect
// the conventional /.dockerenv and /run/.containerenv container marker files.
func defaultMarkerFileExists(path string) bool {
	defer perf.Track(nil, "container.defaultMarkerFileExists")()

	_, err := os.Stat(path)
	return err == nil
}
