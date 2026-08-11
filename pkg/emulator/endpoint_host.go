package emulator

import (
	"context"
	"encoding/hex"
	"net"
	"os"
	"strings"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	envEmulatorEndpointHost               = "ATMOS_EMULATOR_ENDPOINT_HOST"
	envEmulatorUseCurrentContainerNetwork = "ATMOS_EMULATOR_USE_CURRENT_CONTAINER_NETWORK"
	linuxRouteGatewayBytes                = 4
)

var (
	processRunsInContainer = defaultProcessRunsInContainer
	readProcFile           = os.ReadFile
)

func reachableHostForPublishedPorts() string {
	defer perf.Track(nil, "emulator.reachableHostForPublishedPorts")()

	if host := strings.TrimSpace(envString(envEmulatorEndpointHost)); host != "" {
		return host
	}
	if !processRunsInContainer() {
		return "localhost"
	}
	if gateway := linuxDefaultGateway(); gateway != "" {
		return gateway
	}
	return "localhost"
}

func currentContainerNetwork(ctx context.Context, runtime container.Runtime) string {
	defer perf.Track(nil, "emulator.currentContainerNetwork")()

	if !useCurrentContainerNetwork() || runtime == nil {
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

func useCurrentContainerNetwork() bool {
	defer perf.Track(nil, "emulator.useCurrentContainerNetwork")()

	switch strings.ToLower(strings.TrimSpace(envString(envEmulatorUseCurrentContainerNetwork))) {
	case "1", "true", "yes", "on":
		return processRunsInContainer()
	case "0", "false", "no", "off":
		return false
	}
	return envString("GITHUB_ACTIONS") == "true" && processRunsInContainer()
}

func envString(name string) string {
	defer perf.Track(nil, "emulator.envString")()

	_ = viper.BindEnv(name, name)
	return viper.GetString(name)
}

func firstReachableNetwork(networks []string) string {
	defer perf.Track(nil, "emulator.firstReachableNetwork")()

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

func defaultProcessRunsInContainer() bool {
	defer perf.Track(nil, "emulator.defaultProcessRunsInContainer")()

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
