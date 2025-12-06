package devcontainer

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePorts(t *testing.T) {
	tests := []struct {
		name            string
		forwardPorts    []interface{}
		portsAttributes map[string]PortAttributes
		expected        []container.PortBinding
		expectError     bool
	}{
		{
			name:         "empty ports",
			forwardPorts: nil,
			expected:     nil,
			expectError:  false,
		},
		{
			name:         "single integer port",
			forwardPorts: []interface{}{8080},
			expected: []container.PortBinding{
				{ContainerPort: 8080, HostPort: 8080, Protocol: "tcp"},
			},
			expectError: false,
		},
		{
			name:         "multiple integer ports",
			forwardPorts: []interface{}{8080, 3000, 5000},
			expected: []container.PortBinding{
				{ContainerPort: 8080, HostPort: 8080, Protocol: "tcp"},
				{ContainerPort: 3000, HostPort: 3000, Protocol: "tcp"},
				{ContainerPort: 5000, HostPort: 5000, Protocol: "tcp"},
			},
			expectError: false,
		},
		{
			name:         "single string port",
			forwardPorts: []interface{}{"8080"},
			expected: []container.PortBinding{
				{ContainerPort: 8080, HostPort: 8080, Protocol: "tcp"},
			},
			expectError: false,
		},
		{
			name:         "string port with mapping",
			forwardPorts: []interface{}{"8080:3000"},
			expected: []container.PortBinding{
				{ContainerPort: 3000, HostPort: 8080, Protocol: "tcp"},
			},
			expectError: false,
		},
		{
			name:         "mixed integer and string ports",
			forwardPorts: []interface{}{8080, "3000:4000", 5000},
			expected: []container.PortBinding{
				{ContainerPort: 8080, HostPort: 8080, Protocol: "tcp"},
				{ContainerPort: 4000, HostPort: 3000, Protocol: "tcp"},
				{ContainerPort: 5000, HostPort: 5000, Protocol: "tcp"},
			},
			expectError: false,
		},
		{
			name:         "port with custom protocol",
			forwardPorts: []interface{}{8080},
			portsAttributes: map[string]PortAttributes{
				"8080": {Protocol: "udp"},
			},
			expected: []container.PortBinding{
				{ContainerPort: 8080, HostPort: 8080, Protocol: "udp"},
			},
			expectError: false,
		},
		{
			name:         "port with custom protocol and label",
			forwardPorts: []interface{}{"3000"},
			portsAttributes: map[string]PortAttributes{
				"3000": {Label: "Web Server", Protocol: "http"},
			},
			expected: []container.PortBinding{
				{ContainerPort: 3000, HostPort: 3000, Protocol: "http"},
			},
			expectError: false,
		},
		{
			name:         "invalid port type",
			forwardPorts: []interface{}{true},
			expectError:  true,
		},
		{
			name:         "invalid port string",
			forwardPorts: []interface{}{"invalid"},
			expectError:  true,
		},
		{
			name:         "invalid port format too many colons",
			forwardPorts: []interface{}{"8080:3000:5000"},
			expectError:  true,
		},
		{
			name:         "invalid host port",
			forwardPorts: []interface{}{"abc:3000"},
			expectError:  true,
		},
		{
			name:         "invalid container port",
			forwardPorts: []interface{}{"8080:xyz"},
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParsePorts(tt.forwardPorts, tt.portsAttributes)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestParsePort(t *testing.T) {
	tests := []struct {
		name            string
		port            interface{}
		portsAttributes map[string]PortAttributes
		expected        container.PortBinding
		expectError     bool
	}{
		{
			name: "integer port",
			port: 8080,
			expected: container.PortBinding{
				ContainerPort: 8080,
				HostPort:      8080,
				Protocol:      "tcp",
			},
			expectError: false,
		},
		{
			name: "string port simple",
			port: "3000",
			expected: container.PortBinding{
				ContainerPort: 3000,
				HostPort:      3000,
				Protocol:      "tcp",
			},
			expectError: false,
		},
		{
			name: "string port with mapping",
			port: "8080:3000",
			expected: container.PortBinding{
				ContainerPort: 3000,
				HostPort:      8080,
				Protocol:      "tcp",
			},
			expectError: false,
		},
		{
			name: "integer port with custom protocol",
			port: 53,
			portsAttributes: map[string]PortAttributes{
				"53": {Protocol: "udp"},
			},
			expected: container.PortBinding{
				ContainerPort: 53,
				HostPort:      53,
				Protocol:      "udp",
			},
			expectError: false,
		},
		{
			name:        "invalid type",
			port:        []int{8080},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parsePort(tt.port, tt.portsAttributes)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestParsePortString(t *testing.T) {
	tests := []struct {
		name            string
		portStr         string
		portsAttributes map[string]PortAttributes
		expected        container.PortBinding
		expectError     bool
	}{
		{
			name:    "simple port",
			portStr: "8080",
			expected: container.PortBinding{
				ContainerPort: 8080,
				HostPort:      8080,
				Protocol:      "tcp",
			},
			expectError: false,
		},
		{
			name:    "explicit mapping",
			portStr: "8080:3000",
			expected: container.PortBinding{
				ContainerPort: 3000,
				HostPort:      8080,
				Protocol:      "tcp",
			},
			expectError: false,
		},
		{
			name:    "same host and container port",
			portStr: "5000:5000",
			expected: container.PortBinding{
				ContainerPort: 5000,
				HostPort:      5000,
				Protocol:      "tcp",
			},
			expectError: false,
		},
		{
			name:    "port with custom protocol",
			portStr: "3000",
			portsAttributes: map[string]PortAttributes{
				"3000": {Protocol: "http"},
			},
			expected: container.PortBinding{
				ContainerPort: 3000,
				HostPort:      3000,
				Protocol:      "http",
			},
			expectError: false,
		},
		{
			name:        "invalid single port",
			portStr:     "invalid",
			expectError: true,
		},
		{
			name:        "invalid host port",
			portStr:     "abc:3000",
			expectError: true,
		},
		{
			name:        "invalid container port",
			portStr:     "8080:xyz",
			expectError: true,
		},
		{
			name:        "too many colons",
			portStr:     "8080:3000:5000",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parsePortString(tt.portStr, tt.portsAttributes)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestGetProtocol(t *testing.T) {
	tests := []struct {
		name            string
		port            string
		portsAttributes map[string]PortAttributes
		expected        string
	}{
		{
			name:            "no attributes - default tcp",
			port:            "8080",
			portsAttributes: nil,
			expected:        "tcp",
		},
		{
			name:            "empty attributes - default tcp",
			port:            "8080",
			portsAttributes: map[string]PortAttributes{},
			expected:        "tcp",
		},
		{
			name: "custom protocol",
			port: "8080",
			portsAttributes: map[string]PortAttributes{
				"8080": {Protocol: "udp"},
			},
			expected: "udp",
		},
		{
			name: "custom protocol with label",
			port: "3000",
			portsAttributes: map[string]PortAttributes{
				"3000": {Label: "Web", Protocol: "http"},
			},
			expected: "http",
		},
		{
			name: "port not in attributes",
			port: "8080",
			portsAttributes: map[string]PortAttributes{
				"3000": {Protocol: "http"},
			},
			expected: "tcp",
		},
		{
			name: "empty protocol string defaults to tcp",
			port: "8080",
			portsAttributes: map[string]PortAttributes{
				"8080": {Protocol: ""},
			},
			expected: "tcp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getProtocol(tt.port, tt.portsAttributes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatPortBindings(t *testing.T) {
	tests := []struct {
		name     string
		bindings []container.PortBinding
		expected string
	}{
		{
			name:     "empty bindings",
			bindings: nil,
			expected: "-",
		},
		{
			name: "single port same host and container",
			bindings: []container.PortBinding{
				{ContainerPort: 8080, HostPort: 8080, Protocol: "tcp"},
			},
			expected: "8080",
		},
		{
			name: "single port different host and container",
			bindings: []container.PortBinding{
				{ContainerPort: 3000, HostPort: 8080, Protocol: "tcp"},
			},
			expected: "8080:3000",
		},
		{
			name: "multiple ports same",
			bindings: []container.PortBinding{
				{ContainerPort: 8080, HostPort: 8080, Protocol: "tcp"},
				{ContainerPort: 3000, HostPort: 3000, Protocol: "tcp"},
			},
			expected: "8080, 3000",
		},
		{
			name: "multiple ports mixed",
			bindings: []container.PortBinding{
				{ContainerPort: 8080, HostPort: 8080, Protocol: "tcp"},
				{ContainerPort: 4000, HostPort: 3000, Protocol: "tcp"},
				{ContainerPort: 5000, HostPort: 5000, Protocol: "tcp"},
			},
			expected: "8080, 3000:4000, 5000",
		},
		{
			name: "all different mappings",
			bindings: []container.PortBinding{
				{ContainerPort: 3000, HostPort: 8080, Protocol: "tcp"},
				{ContainerPort: 4000, HostPort: 8081, Protocol: "tcp"},
			},
			expected: "8080:3000, 8081:4000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatPortBindings(tt.bindings)
			assert.Equal(t, tt.expected, result)
		})
	}
}
