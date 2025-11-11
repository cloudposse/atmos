package devcontainer

import (
	"fmt"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ParsePorts parses the forwardPorts configuration into PortBinding structs.
// Supports both simple port numbers and explicit host:container mappings.
func ParsePorts(forwardPorts []interface{}, portsAttributes map[string]PortAttributes) ([]container.PortBinding, error) {
	defer perf.Track(nil, "devcontainer.ParsePorts")()

	if len(forwardPorts) == 0 {
		return nil, nil
	}

	var bindings []container.PortBinding

	for _, port := range forwardPorts {
		binding, err := parsePort(port, portsAttributes)
		if err != nil {
			return nil, errUtils.Build(err).
				WithHint("Verify port configuration in `atmos.yaml`").
				WithHint("See DevContainer spec: https://containers.dev/implementors/json_reference/#general-properties").
				Err()
		}
		bindings = append(bindings, binding)
	}

	return bindings, nil
}

// parsePort parses a single port configuration.
func parsePort(port interface{}, portsAttributes map[string]PortAttributes) (container.PortBinding, error) {
	defer perf.Track(nil, "devcontainer.parsePort")()

	switch v := port.(type) {
	case int:
		// Simple port mapping: 8080 -> 8080:8080
		protocol := getProtocol(strconv.Itoa(v), portsAttributes)
		return container.PortBinding{
			ContainerPort: v,
			HostPort:      v,
			Protocol:      protocol,
		}, nil

	case string:
		// Explicit mapping: "3000:3000" or just "3000"
		return parsePortString(v, portsAttributes)

	default:
		return container.PortBinding{}, errUtils.Build(errUtils.ErrInvalidDevcontainerConfig).
			WithExplanationf("Invalid port type: %T (expected int or string)", port).
			WithHint("Ports must be specified as integers (e.g., `8080`) or strings (e.g., `\"3000:3000\"`)").
			WithHint("See DevContainer spec: https://containers.dev/implementors/json_reference/#general-properties").
			WithExample(`Valid port configurations:
  forwardPorts:
    - 8080           # Integer port
    - "3000:3000"    # String mapping
    - 5432           # Integer port`).
			WithExitCode(2).
			Err()
	}
}

// parsePortString parses a string port configuration.
func parsePortString(portStr string, portsAttributes map[string]PortAttributes) (container.PortBinding, error) {
	defer perf.Track(nil, "devcontainer.parsePortString")()

	parts := strings.Split(portStr, ":")

	switch len(parts) {
	case 1:
		return parseSinglePort(parts[0], portsAttributes)
	case 2:
		return parsePortMapping(parts[0], parts[1], portStr, portsAttributes)
	default:
		return container.PortBinding{}, buildInvalidPortFormatError(portStr)
	}
}

// parseSinglePort parses a single port number.
func parseSinglePort(portStr string, portsAttributes map[string]PortAttributes) (container.PortBinding, error) {
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return container.PortBinding{}, errUtils.Build(errUtils.ErrInvalidDevcontainerConfig).
			WithExplanationf("Invalid port number `%s`", portStr).
			WithHint("Port must be a valid integer between 1 and 65535").
			WithHint("See DevContainer spec: https://containers.dev/implementors/json_reference/#general-properties").
			WithContext("port_string", portStr).
			WithExitCode(2).
			Err()
	}
	protocol := getProtocol(portStr, portsAttributes)
	return container.PortBinding{ContainerPort: port, HostPort: port, Protocol: protocol}, nil
}

// parsePortMapping parses a host:container port mapping.
func parsePortMapping(hostStr, containerStr, fullStr string, portsAttributes map[string]PortAttributes) (container.PortBinding, error) {
	hostPort, err := strconv.Atoi(hostStr)
	if err != nil {
		return container.PortBinding{}, errUtils.Build(errUtils.ErrInvalidDevcontainerConfig).
			WithExplanationf("Invalid host port `%s` in port mapping `%s`", hostStr, fullStr).
			WithHint("Host port must be a valid integer between 1 and 65535").
			WithHint("Format should be `host:container` (e.g., `\"8080:3000\"`)").
			WithHint("See DevContainer spec: https://containers.dev/implementors/json_reference/#general-properties").
			WithContext("port_string", fullStr).
			WithContext("host_port", hostStr).
			WithExitCode(2).
			Err()
	}
	containerPort, err := strconv.Atoi(containerStr)
	if err != nil {
		return container.PortBinding{}, errUtils.Build(errUtils.ErrInvalidDevcontainerConfig).
			WithExplanationf("Invalid container port `%s` in port mapping `%s`", containerStr, fullStr).
			WithHint("Container port must be a valid integer between 1 and 65535").
			WithHint("Format should be `host:container` (e.g., `\"8080:3000\"`)").
			WithHint("See DevContainer spec: https://containers.dev/implementors/json_reference/#general-properties").
			WithContext("port_string", fullStr).
			WithContext("container_port", containerStr).
			WithExitCode(2).
			Err()
	}
	protocol := getProtocol(containerStr, portsAttributes)
	return container.PortBinding{ContainerPort: containerPort, HostPort: hostPort, Protocol: protocol}, nil
}

// buildInvalidPortFormatError creates an error for invalid port format.
func buildInvalidPortFormatError(portStr string) error {
	return errUtils.Build(errUtils.ErrInvalidDevcontainerConfig).
		WithExplanationf("Invalid port format `%s`", portStr).
		WithHint("Port format must be either a single port (e.g., `\"8080\"`) or a host:container mapping (e.g., `\"8080:3000\"`)").
		WithHint("See DevContainer spec: https://containers.dev/implementors/json_reference/#general-properties").
		WithExample(`Valid port formats:
  forwardPorts:
    - 8080           # Maps 8080:8080
    - "3000:3000"    # Maps 3000:3000
    - "8080:80"      # Maps host 8080 to container 80`).
		WithContext("port_string", portStr).
		WithExitCode(2).
		Err()
}

// getProtocol gets the protocol for a port from portsAttributes, defaults to "tcp".
func getProtocol(port string, portsAttributes map[string]PortAttributes) string {
	defer perf.Track(nil, "devcontainer.getProtocol")()

	if portsAttributes == nil {
		return "tcp"
	}

	if attrs, ok := portsAttributes[port]; ok && attrs.Protocol != "" {
		return attrs.Protocol
	}

	return "tcp"
}

// FormatPortBindings formats port bindings for display.
func FormatPortBindings(bindings []container.PortBinding) string {
	defer perf.Track(nil, "devcontainer.FormatPortBindings")()

	if len(bindings) == 0 {
		return "-"
	}

	var ports []string
	for _, binding := range bindings {
		if binding.HostPort == binding.ContainerPort {
			ports = append(ports, strconv.Itoa(binding.ContainerPort))
		} else {
			ports = append(ports, fmt.Sprintf("%d:%d", binding.HostPort, binding.ContainerPort))
		}
	}

	return strings.Join(ports, ", ")
}
