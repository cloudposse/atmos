package devcontainer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeserializeStringFields(t *testing.T) {
	tests := []struct {
		name     string
		specMap  map[string]any
		expected func(*Config) bool
	}{
		{
			name: "all string fields present",
			specMap: map[string]any{
				"name":            "test-container",
				"image":           "ubuntu:22.04",
				"workspacefolder": "/workspace",
				"workspacemount":  "type=bind,source=${localWorkspaceFolder},target=/workspace",
				"remoteuser":      "vscode",
				"userenvprobe":    "loginInteractiveShell",
			},
			expected: func(c *Config) bool {
				return c.Name == "test-container" &&
					c.Image == "ubuntu:22.04" &&
					c.WorkspaceFolder == "/workspace" &&
					c.WorkspaceMount == "type=bind,source=${localWorkspaceFolder},target=/workspace" &&
					c.RemoteUser == "vscode" &&
					c.UserEnvProbe == "loginInteractiveShell"
			},
		},
		{
			name:    "empty specMap",
			specMap: map[string]any{},
			expected: func(c *Config) bool {
				return c.Name == "" && c.Image == "" && c.WorkspaceFolder == ""
			},
		},
		{
			name: "wrong types ignored",
			specMap: map[string]any{
				"name":  123,
				"image": true,
			},
			expected: func(c *Config) bool {
				return c.Name == "" && c.Image == ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{}
			deserializeStringFields(tt.specMap, config)
			assert.True(t, tt.expected(config), "Config fields did not match expected values")
		})
	}
}

func TestDeserializeBuildSection(t *testing.T) {
	tests := []struct {
		name     string
		specMap  map[string]any
		expected *Build
	}{
		{
			name: "full build section",
			specMap: map[string]any{
				"build": map[string]any{
					"dockerfile": "Dockerfile.dev",
					"context":    ".",
					"args": map[string]any{
						"NODE_VERSION": "18",
						"VARIANT":      "bullseye",
					},
				},
			},
			expected: &Build{
				Dockerfile: "Dockerfile.dev",
				Context:    ".",
				Args: map[string]string{
					"NODE_VERSION": "18",
					"VARIANT":      "bullseye",
				},
			},
		},
		{
			name:     "no build section",
			specMap:  map[string]any{},
			expected: nil,
		},
		{
			name: "empty build section",
			specMap: map[string]any{
				"build": map[string]any{},
			},
			expected: &Build{
				Args: map[string]string{}, // getStringMap returns empty map, not nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{}
			deserializeBuildSection(tt.specMap, config)
			assert.Equal(t, tt.expected, config.Build)
		})
	}
}

func TestDeserializeArrayFields(t *testing.T) {
	tests := []struct {
		name     string
		specMap  map[string]any
		expected func(*Config) bool
	}{
		{
			name: "all array fields present",
			specMap: map[string]any{
				"mounts": []any{
					"type=bind,source=/host/path,target=/container/path",
					"type=volume,source=my-volume,target=/data",
				},
				"runargs": []any{
					"--privileged",
					"--security-opt=seccomp=unconfined",
				},
				"capadd": []any{
					"SYS_PTRACE",
					"NET_ADMIN",
				},
				"securityopt": []any{
					"seccomp=unconfined",
				},
			},
			expected: func(c *Config) bool {
				return len(c.Mounts) == 2 &&
					len(c.RunArgs) == 2 &&
					len(c.CapAdd) == 2 &&
					len(c.SecurityOpt) == 1 &&
					c.Mounts[0] == "type=bind,source=/host/path,target=/container/path" &&
					c.RunArgs[0] == "--privileged" &&
					c.CapAdd[0] == "SYS_PTRACE" &&
					c.SecurityOpt[0] == "seccomp=unconfined"
			},
		},
		{
			name:    "empty specMap",
			specMap: map[string]any{},
			expected: func(c *Config) bool {
				return c.Mounts == nil && c.RunArgs == nil && c.CapAdd == nil && c.SecurityOpt == nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{}
			deserializeArrayFields(tt.specMap, config)
			assert.True(t, tt.expected(config), "Config array fields did not match expected values")
		})
	}
}

func TestDeserializeBooleanFields(t *testing.T) {
	tests := []struct {
		name     string
		specMap  map[string]any
		expected func(*Config) bool
	}{
		{
			name: "all boolean fields true",
			specMap: map[string]any{
				"overridecommand": true,
				"init":            true,
				"privileged":      true,
			},
			expected: func(c *Config) bool {
				return c.OverrideCommand && c.Init && c.Privileged
			},
		},
		{
			name: "all boolean fields false",
			specMap: map[string]any{
				"overridecommand": false,
				"init":            false,
				"privileged":      false,
			},
			expected: func(c *Config) bool {
				return !c.OverrideCommand && !c.Init && !c.Privileged
			},
		},
		{
			name:    "empty specMap defaults to false",
			specMap: map[string]any{},
			expected: func(c *Config) bool {
				return !c.OverrideCommand && !c.Init && !c.Privileged
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{}
			deserializeBooleanFields(tt.specMap, config)
			assert.True(t, tt.expected(config), "Config boolean fields did not match expected values")
		})
	}
}

func TestDeserializeForwardPorts(t *testing.T) {
	tests := []struct {
		name     string
		specMap  map[string]any
		expected []any
	}{
		{
			name: "integer ports",
			specMap: map[string]any{
				"forwardports": []any{8080, 3000, 5432},
			},
			expected: []any{8080, 3000, 5432},
		},
		{
			name: "string ports",
			specMap: map[string]any{
				"forwardports": []any{"8080:8080", "3000:3001"},
			},
			expected: []any{"8080:8080", "3000:3001"},
		},
		{
			name: "mixed types",
			specMap: map[string]any{
				"forwardports": []any{8080, "3000:3001"},
			},
			expected: []any{8080, "3000:3001"},
		},
		{
			name:     "no forwardports",
			specMap:  map[string]any{},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{}
			deserializeForwardPorts(tt.specMap, config)
			assert.Equal(t, tt.expected, config.ForwardPorts)
		})
	}
}

func TestDeserializePortsAttributes(t *testing.T) {
	tests := []struct {
		name     string
		specMap  map[string]any
		expected map[string]PortAttributes
	}{
		{
			name: "with label and protocol",
			specMap: map[string]any{
				"portsattributes": map[string]any{
					"3000": map[string]any{
						"label":    "Application",
						"protocol": "https",
					},
					"8080": map[string]any{
						"label":    "API Server",
						"protocol": "http",
					},
				},
			},
			expected: map[string]PortAttributes{
				"3000": {Label: "Application", Protocol: "https"},
				"8080": {Label: "API Server", Protocol: "http"},
			},
		},
		{
			name: "with only label",
			specMap: map[string]any{
				"portsattributes": map[string]any{
					"3000": map[string]any{
						"label": "Web Server",
					},
				},
			},
			expected: map[string]PortAttributes{
				"3000": {Label: "Web Server", Protocol: ""},
			},
		},
		{
			name: "with only protocol",
			specMap: map[string]any{
				"portsattributes": map[string]any{
					"3000": map[string]any{
						"protocol": "https",
					},
				},
			},
			expected: map[string]PortAttributes{
				"3000": {Label: "", Protocol: "https"},
			},
		},
		{
			name:     "missing portsattributes key",
			specMap:  map[string]any{},
			expected: nil,
		},
		{
			name: "invalid portsattributes type",
			specMap: map[string]any{
				"portsattributes": "not a map",
			},
			expected: nil,
		},
		{
			name: "invalid port attributes type",
			specMap: map[string]any{
				"portsattributes": map[string]any{
					"3000": "not a map",
				},
			},
			expected: map[string]PortAttributes{},
		},
		{
			name: "empty portsattributes",
			specMap: map[string]any{
				"portsattributes": map[string]any{},
			},
			expected: map[string]PortAttributes{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{}
			deserializePortsAttributes(tt.specMap, config)
			assert.Equal(t, tt.expected, config.PortsAttributes)
		})
	}
}

func TestDeserializeContainerEnv(t *testing.T) {
	tests := []struct {
		name     string
		specMap  map[string]any
		expected map[string]string
	}{
		{
			name: "string values",
			specMap: map[string]any{
				"containerenv": map[string]any{
					"NODE_ENV":    "development",
					"DEBUG":       "true",
					"API_VERSION": "v1",
				},
			},
			expected: map[string]string{
				"NODE_ENV":    "development",
				"DEBUG":       "true",
				"API_VERSION": "v1",
			},
		},
		{
			name: "only string values preserved",
			specMap: map[string]any{
				"containerenv": map[string]any{
					"NODE_ENV": "production",
					"NAME":     "test",
				},
			},
			expected: map[string]string{
				"NODE_ENV": "production",
				"NAME":     "test",
			},
		},
		{
			name:     "no containerenv",
			specMap:  map[string]any{},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{}
			deserializeContainerEnv(tt.specMap, config)
			assert.Equal(t, tt.expected, config.ContainerEnv)
		})
	}
}

func TestToStringSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    []any
		expected []string
	}{
		{
			name:     "all strings",
			input:    []any{"foo", "bar", "baz"},
			expected: []string{"foo", "bar", "baz"},
		},
		{
			name:     "mixed types",
			input:    []any{"foo", 123, true},
			expected: []string{"foo"},
		},
		{
			name:     "empty slice",
			input:    []any{},
			expected: []string{},
		},
		{
			name:     "nil slice",
			input:    nil,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toStringSlice(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetString(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]any
		key      string
		expected string
	}{
		{
			name:     "key exists with string value",
			m:        map[string]any{"foo": "bar"},
			key:      "foo",
			expected: "bar",
		},
		{
			name:     "key does not exist",
			m:        map[string]any{"foo": "bar"},
			key:      "baz",
			expected: "",
		},
		{
			name:     "key exists with non-string value",
			m:        map[string]any{"foo": 123},
			key:      "foo",
			expected: "",
		},
		{
			name:     "nil map",
			m:        nil,
			key:      "foo",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getString(tt.m, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetStringMap(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]any
		key      string
		expected map[string]string
	}{
		{
			name: "key exists with map[string]any",
			m: map[string]any{
				"args": map[string]any{
					"NODE_VERSION": "18",
					"VARIANT":      "bullseye",
				},
			},
			key: "args",
			expected: map[string]string{
				"NODE_VERSION": "18",
				"VARIANT":      "bullseye",
			},
		},
		{
			name: "key does not exist",
			m: map[string]any{
				"other": "value",
			},
			key:      "args",
			expected: map[string]string{}, // getStringMap returns empty map, not nil
		},
		{
			name: "key exists with wrong type",
			m: map[string]any{
				"args": "not a map",
			},
			key:      "args",
			expected: map[string]string{}, // getStringMap returns empty map, not nil
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getStringMap(tt.m, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeserializeSpec(t *testing.T) {
	tests := []struct {
		name    string
		specMap map[string]any
		dcName  string
		assert  func(*testing.T, *Config)
	}{
		{
			name: "complete valid spec",
			specMap: map[string]any{
				"name":            "my-devcontainer",
				"image":           "mcr.microsoft.com/devcontainers/typescript-node:18",
				"workspacefolder": "/workspaces/myproject",
				"forwardports":    []any{3000, 8080},
				"containerenv": map[string]any{
					"NODE_ENV": "development",
				},
			},
			dcName: "test",
			assert: func(t *testing.T, c *Config) {
				assert.Equal(t, "my-devcontainer", c.Name)
				assert.Equal(t, "mcr.microsoft.com/devcontainers/typescript-node:18", c.Image)
				assert.Equal(t, "/workspaces/myproject", c.WorkspaceFolder)
				assert.Equal(t, []any{3000, 8080}, c.ForwardPorts)
				assert.Equal(t, map[string]string{"NODE_ENV": "development"}, c.ContainerEnv)
			},
		},
		{
			name:    "minimal spec",
			specMap: map[string]any{},
			dcName:  "minimal",
			assert: func(t *testing.T, c *Config) {
				assert.NotNil(t, c)
				assert.Equal(t, "", c.Name)
				assert.Equal(t, "", c.Image)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := deserializeSpec(tt.specMap, tt.dcName)
			require.NoError(t, err)
			require.NotNil(t, config)
			tt.assert(t, config)
		})
	}
}
