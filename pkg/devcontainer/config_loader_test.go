package devcontainer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
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

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		dcName      string
		expectError bool
		errorMsg    string
		assert      func(t *testing.T, config *Config, settings *Settings)
	}{
		{
			name: "valid devcontainer with settings",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: map[string]any{
						"test-alpine": map[string]any{
							"settings": map[string]any{
								"runtime": "podman",
							},
							"spec": map[string]any{
								"image":           "alpine:latest",
								"workspacefolder": "/workspace",
								"remoteuser":      "root",
							},
						},
					},
				},
			},
			dcName:      "test-alpine",
			expectError: false,
			assert: func(t *testing.T, config *Config, settings *Settings) {
				assert.NotNil(t, config)
				assert.NotNil(t, settings)
				assert.Equal(t, "alpine:latest", config.Image)
				assert.Equal(t, "/workspace", config.WorkspaceFolder)
				assert.Equal(t, "root", config.RemoteUser)
				assert.Equal(t, "podman", settings.Runtime)
			},
		},
		{
			name: "valid devcontainer without settings",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: map[string]any{
						"test-ubuntu": map[string]any{
							"spec": map[string]any{
								"image":      "ubuntu:22.04",
								"remoteuser": "vscode",
							},
						},
					},
				},
			},
			dcName:      "test-ubuntu",
			expectError: false,
			assert: func(t *testing.T, config *Config, settings *Settings) {
				assert.NotNil(t, config)
				assert.NotNil(t, settings)
				assert.Equal(t, "ubuntu:22.04", config.Image)
				assert.Equal(t, "vscode", config.RemoteUser)
				assert.Equal(t, "", settings.Runtime) // Empty when not specified
			},
		},
		{
			name: "no devcontainers configured",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: nil,
				},
			},
			dcName:      "test",
			expectError: true,
			errorMsg:    "no devcontainers configured",
		},
		{
			name: "devcontainer not found",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: map[string]any{
						"existing": map[string]any{
							"spec": map[string]any{
								"image": "alpine:latest",
							},
						},
					},
				},
			},
			dcName:      "nonexistent",
			expectError: true,
			errorMsg:    "not found",
		},
		{
			name: "missing spec section",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: map[string]any{
						"invalid": map[string]any{
							"settings": map[string]any{
								"runtime": "docker",
							},
							// Missing 'spec' section
						},
					},
				},
			},
			dcName:      "invalid",
			expectError: true,
			errorMsg:    "missing 'spec' section",
		},
		{
			name: "spec is not a map",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: map[string]any{
						"invalid": map[string]any{
							"spec": "not a map",
						},
					},
				},
			},
			dcName:      "invalid",
			expectError: true,
			errorMsg:    "must be a map",
		},
		{
			name: "validation fails - no image or build",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: map[string]any{
						"invalid": map[string]any{
							"spec": map[string]any{
								"name": "test",
								// Missing both image and build
							},
						},
					},
				},
			},
			dcName:      "invalid",
			expectError: true,
			errorMsg:    "must specify either 'image' or 'build'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, settings, err := LoadConfig(tt.atmosConfig, tt.dcName)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, config)
				assert.Nil(t, settings)
			} else {
				require.NoError(t, err)
				tt.assert(t, config, settings)
			}
		})
	}
}

func TestLoadAllConfigs(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		expectError bool
		assert      func(t *testing.T, configs map[string]*Config)
	}{
		{
			name: "multiple valid devcontainers",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: map[string]any{
						"alpine": map[string]any{
							"spec": map[string]any{
								"image": "alpine:latest",
							},
						},
						"ubuntu": map[string]any{
							"spec": map[string]any{
								"image": "ubuntu:22.04",
							},
						},
						"python": map[string]any{
							"spec": map[string]any{
								"image": "python:3.11",
							},
						},
					},
				},
			},
			expectError: false,
			assert: func(t *testing.T, configs map[string]*Config) {
				assert.Len(t, configs, 3)
				assert.NotNil(t, configs["alpine"])
				assert.NotNil(t, configs["ubuntu"])
				assert.NotNil(t, configs["python"])
				assert.Equal(t, "alpine:latest", configs["alpine"].Image)
				assert.Equal(t, "ubuntu:22.04", configs["ubuntu"].Image)
				assert.Equal(t, "python:3.11", configs["python"].Image)
			},
		},
		{
			name: "no devcontainers configured",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: nil,
				},
			},
			expectError: false,
			assert: func(t *testing.T, configs map[string]*Config) {
				assert.Empty(t, configs)
			},
		},
		{
			name: "empty devcontainers map",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: map[string]any{},
				},
			},
			expectError: false,
			assert: func(t *testing.T, configs map[string]*Config) {
				assert.Empty(t, configs)
			},
		},
		{
			name: "one valid one invalid",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: map[string]any{
						"valid": map[string]any{
							"spec": map[string]any{
								"image": "alpine:latest",
							},
						},
						"invalid": map[string]any{
							// Missing spec section
						},
					},
				},
			},
			expectError: true, // Should fail on the invalid one
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs, err := LoadAllConfigs(tt.atmosConfig)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				tt.assert(t, configs)
			}
		})
	}
}

func TestExtractSettings(t *testing.T) {
	tests := []struct {
		name        string
		devMap      map[string]any
		dcName      string
		expectError bool
		errorMsg    string
		assert      func(t *testing.T, settings *Settings)
	}{
		{
			name: "settings with runtime",
			devMap: map[string]any{
				"settings": map[string]any{
					"runtime": "podman",
				},
			},
			dcName:      "test",
			expectError: false,
			assert: func(t *testing.T, settings *Settings) {
				assert.Equal(t, "podman", settings.Runtime)
			},
		},
		{
			name:        "no settings section",
			devMap:      map[string]any{},
			dcName:      "test",
			expectError: false,
			assert: func(t *testing.T, settings *Settings) {
				assert.Equal(t, "", settings.Runtime)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings, err := extractSettings(tt.devMap, tt.dcName)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				tt.assert(t, settings)
			}
		})
	}
}

func TestExtractAndValidateSpec(t *testing.T) {
	tests := []struct {
		name        string
		devMap      map[string]any
		dcName      string
		expectError bool
		errorMsg    string
		assert      func(t *testing.T, config *Config)
	}{
		{
			name: "valid spec with image",
			devMap: map[string]any{
				"spec": map[string]any{
					"image": "alpine:latest",
				},
			},
			dcName:      "test",
			expectError: false,
			assert: func(t *testing.T, config *Config) {
				assert.Equal(t, "alpine:latest", config.Image)
				assert.Equal(t, "test", config.Name)   // Name defaults to dcName
				assert.True(t, config.OverrideCommand) // Defaults to true
			},
		},
		{
			name: "spec with explicit name",
			devMap: map[string]any{
				"spec": map[string]any{
					"name":  "custom-name",
					"image": "ubuntu:22.04",
				},
			},
			dcName:      "test",
			expectError: false,
			assert: func(t *testing.T, config *Config) {
				assert.Equal(t, "custom-name", config.Name) // Uses spec name
			},
		},
		{
			name: "spec with overridecommand false",
			devMap: map[string]any{
				"spec": map[string]any{
					"image":           "alpine:latest",
					"overridecommand": false,
				},
			},
			dcName:      "test",
			expectError: false,
			assert: func(t *testing.T, config *Config) {
				assert.False(t, config.OverrideCommand) // Explicitly set to false
			},
		},
		{
			name: "missing spec section",
			devMap: map[string]any{
				"settings": map[string]any{},
			},
			dcName:      "test",
			expectError: true,
			errorMsg:    "missing 'spec' section",
		},
		{
			name: "spec is not a map",
			devMap: map[string]any{
				"spec": "not a map",
			},
			dcName:      "test",
			expectError: true,
			errorMsg:    "must be a map",
		},
		{
			name: "spec fails validation - no image or build",
			devMap: map[string]any{
				"spec": map[string]any{
					"name": "test-no-image",
					// Missing both image and build
				},
			},
			dcName:      "test",
			expectError: true,
			errorMsg:    "must specify either 'image' or 'build'",
		},
		{
			name: "spec with build but missing dockerfile",
			devMap: map[string]any{
				"spec": map[string]any{
					"build": map[string]any{
						"context": ".",
						// Missing dockerfile
					},
				},
			},
			dcName:      "test",
			expectError: true,
			errorMsg:    "dockerfile is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := extractAndValidateSpec(tt.devMap, tt.dcName)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				tt.assert(t, config)
			}
		})
	}
}

func TestGetDevcontainerMap(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		dcName      string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid devcontainer exists",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: map[string]any{
						"test": map[string]any{
							"spec": map[string]any{
								"image": "alpine:latest",
							},
						},
					},
				},
			},
			dcName:      "test",
			expectError: false,
		},
		{
			name: "no devcontainers configured",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: nil,
				},
			},
			dcName:      "test",
			expectError: true,
			errorMsg:    "no devcontainers configured",
		},
		{
			name: "devcontainer not found",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: map[string]any{
						"existing": map[string]any{},
					},
				},
			},
			dcName:      "nonexistent",
			expectError: true,
			errorMsg:    "not found",
		},
		{
			name: "devcontainer is not a map",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: map[string]any{
						"invalid": "not a map",
					},
				},
			},
			dcName:      "invalid",
			expectError: true,
			errorMsg:    "must be a map",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			devMap, err := getDevcontainerMap(tt.atmosConfig, tt.dcName)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, devMap)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, devMap)
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config with image",
			config: &Config{
				Name:  "test",
				Image: "alpine:latest",
			},
			expectError: false,
		},
		{
			name: "valid config with dockerfile",
			config: &Config{
				Name: "test",
				Build: &Build{
					Dockerfile: "Dockerfile",
				},
			},
			expectError: false,
		},
		{
			name:        "missing name",
			config:      &Config{Image: "alpine:latest"},
			expectError: true,
			errorMsg:    "name is required",
		},
		{
			name:        "missing both image and dockerfile",
			config:      &Config{Name: "test"},
			expectError: true,
			errorMsg:    "must specify either 'image' or 'build'",
		},
		{
			name: "build missing dockerfile",
			config: &Config{
				Name:  "test",
				Build: &Build{},
			},
			expectError: true,
			errorMsg:    "dockerfile is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
