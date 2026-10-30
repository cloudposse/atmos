package devcontainer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
)

func TestCreateConfigPreservesPortMappingsAndProtocols(t *testing.T) {
	config := &Config{
		ForwardPorts:    []any{"19581:8080", float64(19583), "19584:443"},
		PortsAttributes: map[string]PortAttributes{"8080": {Protocol: "udp"}, "19583": {Protocol: "udp"}, "443": {Protocol: "https"}},
	}
	actual := ToCreateConfig(config, "test", "test", "default")
	require.Equal(t, []container.PortBinding{
		{HostPort: 19581, ContainerPort: 8080, Protocol: "udp"},
		{HostPort: 19583, ContainerPort: 19583, Protocol: "udp"},
		{HostPort: 19584, ContainerPort: 443, Protocol: "tcp"},
	}, actual.Ports)
}
