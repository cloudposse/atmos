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
)

var readProcFile = os.ReadFile

// reachableHostForPublishedPorts returns the host an emulator's published ports
// are reachable at from outside its container -- unrelated to network
// *attachment* (see container.AttachSharedNetwork/CurrentContainerNetwork for
// that), this is purely about guessing a reachable address for reading a
// published port from the caller's side.
func reachableHostForPublishedPorts() string {
	defer perf.Track(nil, "emulator.reachableHostForPublishedPorts")()

	if host := strings.TrimSpace(envString(envEmulatorEndpointHost)); host != "" {
		return host
	}
	if !container.ProcessRunsInContainer() {
		return "localhost"
	}
	if gateway := linuxDefaultGateway(); gateway != "" {
		return gateway
	}
	return "localhost"
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
