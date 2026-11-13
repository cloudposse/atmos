package container

import (
	"encoding/json"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPortBindingsFromSchema(t *testing.T) {
	tests := []struct {
		name    string
		host    any
		bind    string
		port    int
		invalid bool
	}{
		{name: "omitted"},
		{name: "numeric", host: 8080, port: 8080},
		{name: "quoted numeric", host: "8080", port: 8080},
		{name: "IPv4", host: "127.0.0.1:8080", bind: "127.0.0.1", port: 8080},
		{name: "IPv4 wildcard", host: "0.0.0.0:8080", bind: "0.0.0.0", port: 8080},
		{name: "IPv6", host: "[::1]:8080", bind: "::1", port: 8080},
		{name: "IPv6 wildcard", host: "[::]:8080", bind: "::", port: 8080},
		{name: "auto IPv4", host: "127.0.0.1:0", bind: "127.0.0.1"},
		{name: "auto IPv6", host: "[::1]:0", bind: "::1"},
		{name: "JSON number", host: json.Number("8080"), port: 8080},
		{name: "JSON float", host: float64(8080), port: 8080},
		{name: "fraction", host: 1.5, invalid: true},
		{name: "negative", host: -1, invalid: true},
		{name: "overflow", host: 65536, invalid: true},
		{name: "address overflow", host: "127.0.0.1:65536", invalid: true},
		{name: "hostname", host: "localhost:8080", invalid: true},
		{name: "unbracketed IPv6", host: "::1:8080", invalid: true},
		{name: "missing port", host: "127.0.0.1:", invalid: true},
		{name: "missing IP", host: ":8080", invalid: true},
		{name: "invalid IP", host: "999.0.0.1:8080", invalid: true},
		{name: "boolean", host: true, invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PortBindingsFromSchema([]schema.ContainerPort{{Host: tt.host, Container: 3000}})
			if tt.invalid {
				require.Error(t, err)
				require.Contains(t, err.Error(), "ports[0].host")
				return
			}
			require.NoError(t, err)
			require.Equal(t, []PortBinding{{Bind: tt.bind, HostPort: tt.port, ContainerPort: 3000, Protocol: "tcp"}}, got)
		})
	}
}

func TestPortHostDecoding(t *testing.T) {
	for _, input := range []string{`{"host":8080,"container":3000}`, `{"host":"127.0.0.1:8080","container":3000}`, `{"host":"[::1]:8080","container":3000}`} {
		t.Run(input, func(t *testing.T) {
			var fromJSON, fromYAML schema.ContainerPort
			require.NoError(t, json.Unmarshal([]byte(input), &fromJSON))
			require.NoError(t, yaml.Unmarshal([]byte(input), &fromYAML))
			jsonPorts, err := PortBindingsFromSchema([]schema.ContainerPort{fromJSON})
			require.NoError(t, err)
			yamlPorts, err := PortBindingsFromSchema([]schema.ContainerPort{fromYAML})
			require.NoError(t, err)
			require.Equal(t, jsonPorts, yamlPorts)
			var raw map[string]any
			require.NoError(t, json.Unmarshal([]byte(input), &raw))
			var decoded schema.ContainerPort
			require.NoError(t, mapstructure.Decode(raw, &decoded))
			mapPorts, err := PortBindingsFromSchema([]schema.ContainerPort{decoded})
			require.NoError(t, err)
			require.Equal(t, jsonPorts, mapPorts)
			require.Equal(t, 8080, jsonPorts[0].HostPort)
		})
	}
}
