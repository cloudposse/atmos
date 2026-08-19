package emulator

import (
	"encoding/hex"
	"net"
	"os"
	"strings"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	envEmulatorEndpointHost = "ATMOS_EMULATOR_ENDPOINT_HOST"
	linuxRouteGatewayBytes  = 4
	// Docker's documented hostname for reaching the host (and, from a sibling
	// container's perspective, host-published ports) from inside a container.
	// Docker Desktop (macOS/Windows) resolves it automatically; Linux Docker
	// Engine only resolves it when the container was started with
	// `--add-host=host.docker.internal:host-gateway`, which Atmos doesn't
	// control after the fact for its own (already running) container -- hence
	// resolving it defensively via lookupHost before trusting it.
	hostDockerInternal = "host.docker.internal"
)

var (
	readProcFile = os.ReadFile
	// Resolves a hostname; overridable in tests so hostDockerInternal's
	// resolvable-vs-unresolvable branches are exercisable without depending on
	// the test host's actual DNS/hosts-file setup.
	lookupHost = net.LookupHost
)

// reachableHostForPublishedPorts returns the host an emulator's published ports
// are reachable at from outside its container -- unrelated to network
// *attachment* (see container.AttachSharedNetwork/CurrentContainerNetwork for
// that; when reuse or a join succeeds there, Manager.endpoint uses a DNS alias
// and never reaches this function at all), this is purely a last-resort guess
// at a reachable address for reading a published port from the caller's side,
// tried in order: an explicit operator override, host.docker.internal (only if
// it actually resolves -- see hostDockerInternal), the container's own default
// gateway (Docker's classic bridge-gateway IP, correct on a native Linux Docker
// host but not necessarily under Docker Desktop's VM-backed daemon), and
// finally localhost.
func reachableHostForPublishedPorts() string {
	defer perf.Track(nil, "emulator.reachableHostForPublishedPorts")()

	if host := strings.TrimSpace(envString(envEmulatorEndpointHost)); host != "" {
		return host
	}
	if !container.ProcessRunsInContainer() {
		return "localhost"
	}
	if hostDockerInternalResolves() {
		return hostDockerInternal
	}
	if gateway := linuxDefaultGateway(); gateway != "" {
		return gateway
	}
	return "localhost"
}

// hostDockerInternalResolves reports whether hostDockerInternal resolves to at
// least one address from inside the current container -- the signal that it's
// actually usable here, rather than assuming it based on platform alone.
func hostDockerInternalResolves() bool {
	defer perf.Track(nil, "emulator.hostDockerInternalResolves")()

	addrs, err := lookupHost(hostDockerInternal)
	return err == nil && len(addrs) > 0
}

func envString(name string) string {
	defer perf.Track(nil, "emulator.envString")()

	_ = viper.BindEnv(name, name)
	return viper.GetString(name)
}

func linuxDefaultGateway() string {
	defer perf.Track(nil, "emulator.linuxDefaultGateway")()

	data, err := readProcFile("/proc/net/route")
	if err != nil {
		return ""
	}
	return parseLinuxDefaultGateway(string(data))
}

func parseLinuxDefaultGateway(routeTable string) string {
	defer perf.Track(nil, "emulator.parseLinuxDefaultGateway")()

	for _, line := range strings.Split(routeTable, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[1] != "00000000" {
			continue
		}
		gateway, err := hex.DecodeString(fields[2])
		if err != nil || len(gateway) != linuxRouteGatewayBytes || isZeroIPv4(gateway) {
			continue
		}
		return net.IPv4(gateway[3], gateway[2], gateway[1], gateway[0]).String()
	}
	return ""
}

func isZeroIPv4(ip []byte) bool {
	defer perf.Track(nil, "emulator.isZeroIPv4")()

	return len(ip) == linuxRouteGatewayBytes && ip[0] == 0 && ip[1] == 0 && ip[2] == 0 && ip[3] == 0
}
