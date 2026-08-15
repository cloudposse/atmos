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
	hostname, err := os.Hostname()
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
// An explicit envUseCurrentContainerNetwork override always wins; otherwise it
// defaults to whether Atmos itself is detected as running inside a container --
// CurrentContainerNetwork safely returns "" when the network can't actually be
// determined, so callers fall back to the dedicated network automatically.
func PreferCurrentContainerNetwork() bool {
	defer perf.Track(nil, "container.PreferCurrentContainerNetwork")()

	switch strings.ToLower(strings.TrimSpace(envString(envUseCurrentContainerNetwork))) {
	case "1", "true", "yes", "on":
		return ProcessRunsInContainer()
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
// "host", or "none" -- those aren't usable for container-to-container DNS.
func firstReachableNetwork(networks []string) string {
	defer perf.Track(nil, "container.firstReachableNetwork")()

	for _, network := range networks {
		switch network {
		case "", "host", "none":
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

	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
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
