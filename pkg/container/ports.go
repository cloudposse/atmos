package container

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const maxPublishedPort = 65535

// PortBindingsFromSchema validates host endpoints and converts them into runtime bindings.
// An omitted host or port zero requests a runtime-assigned port.
func PortBindingsFromSchema(ports []schema.ContainerPort) ([]PortBinding, error) {
	defer perf.Track(nil, "container.PortBindingsFromSchema")()
	if ports == nil {
		return nil, nil
	}
	result := make([]PortBinding, 0, len(ports))
	for i, port := range ports {
		bind, host, err := parsePublishedHost(port.Host)
		if err != nil {
			return nil, fmt.Errorf("ports[%d].host: %w", i, err)
		}
		protocol := port.Protocol
		if protocol == "" {
			protocol = "tcp"
		}
		result = append(result, PortBinding{Bind: bind, HostPort: host, ContainerPort: port.Container, Protocol: protocol})
	}
	return result, nil
}

func parsePublishedHost(value any) (string, int, error) {
	switch v := value.(type) {
	case nil:
		return "", 0, nil
	case int:
		return validatedPublishedPort("", v)
	case float64:
		// JSON decoding into an interface produces float64 values.
		if v >= 0 && v <= maxPublishedPort && v == float64(int(v)) {
			return "", int(v), nil
		}
	case json.Number:
		if n, err := strconv.Atoi(string(v)); err == nil {
			return validatedPublishedPort("", n)
		}
	case string:
		return parsePublishedHostString(v)
	}
	return "", 0, fmt.Errorf("%w: host must be a port number (0-65535) or IP:port", errUtils.ErrContainerRuntimeOperation)
}

func parsePublishedHostString(value string) (string, int, error) {
	// Quoted port numbers are accepted alongside numeric YAML values.
	if !strings.Contains(value, ":") {
		if n, err := strconv.Atoi(value); err == nil {
			return validatedPublishedPort("", n)
		}
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil || net.ParseIP(host) == nil {
		return "", 0, fmt.Errorf("%w: invalid host %q; use IP:port, with brackets around IPv6", errUtils.ErrContainerRuntimeOperation, value)
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		return "", 0, fmt.Errorf("%w: invalid host port in %q", errUtils.ErrContainerRuntimeOperation, value)
	}
	return validatedPublishedPort(host, n)
}

func validatedPublishedPort(host string, port int) (string, int, error) {
	if port < 0 || port > maxPublishedPort {
		return "", 0, fmt.Errorf("%w: host port must be between 0 and 65535", errUtils.ErrContainerRuntimeOperation)
	}
	return host, port, nil
}
