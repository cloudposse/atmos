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
			return nil, err
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
		return container.PortBinding{}, fmt.Errorf("%w: invalid port type %T", errUtils.ErrInvalidDevcontainerConfig, port)
	}
}

// parsePortString parses a string port configuration.
func parsePortString(portStr string, portsAttributes map[string]PortAttributes) (container.PortBinding, error) {
	defer perf.Track(nil, "devcontainer.parsePortString")()

	parts := strings.Split(portStr, ":")

	switch len(parts) {
	case 1:
		// Simple port: "8080" -> 8080:8080
		port, err := strconv.Atoi(parts[0])
		if err != nil {
			return container.PortBinding{}, fmt.Errorf("%w: invalid port number '%s': %v", errUtils.ErrInvalidDevcontainerConfig, parts[0], err)
		}
		protocol := getProtocol(parts[0], portsAttributes)
		return container.PortBinding{
			ContainerPort: port,
			HostPort:      port,
			Protocol:      protocol,
		}, nil

	case 2:
		// Explicit mapping: "3000:3000"
		hostPort, err := strconv.Atoi(parts[0])
		if err != nil {
			return container.PortBinding{}, fmt.Errorf("%w: invalid host port '%s': %v", errUtils.ErrInvalidDevcontainerConfig, parts[0], err)
		}
		containerPort, err := strconv.Atoi(parts[1])
		if err != nil {
			return container.PortBinding{}, fmt.Errorf("%w: invalid container port '%s': %v", errUtils.ErrInvalidDevcontainerConfig, parts[1], err)
		}
		protocol := getProtocol(parts[1], portsAttributes)
		return container.PortBinding{
			ContainerPort: containerPort,
			HostPort:      hostPort,
			Protocol:      protocol,
		}, nil

	default:
		return container.PortBinding{}, fmt.Errorf("%w: invalid port format '%s'", errUtils.ErrInvalidDevcontainerConfig, portStr)
	}
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
