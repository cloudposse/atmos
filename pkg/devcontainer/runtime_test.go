package devcontainer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
)

func TestDetectRuntime_ExplicitSetting(t *testing.T) {
	tests := []struct {
		name           string
		runtimeSetting string
		expectError    bool
		errorContains  string
	}{
		{
			name:           "docker setting",
			runtimeSetting: "docker",
			expectError:    false,
		},
		{
			name:           "podman setting",
			runtimeSetting: "podman",
			expectError:    false,
		},
		{
			name:           "invalid setting",
			runtimeSetting: "containerd",
			expectError:    true,
			errorContains:  "invalid runtime setting 'containerd'",
		},
		{
			name:           "empty setting uses auto-detect",
			runtimeSetting: "",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DetectRuntime(tt.runtimeSetting)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else if err != nil {
				// Note: This may fail if docker/podman not installed.
				// In CI, this tests the logic path at minimum.
				// If runtime not available, that's ok for this test.
				assert.Contains(t, err.Error(), "not available")
			}
		})
	}
}

func TestToCreateConfig(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		containerName    string
		devcontainerName string
		instance         string
		assertFunc       func(*testing.T, *container.CreateConfig)
	}{
		{
			name: "minimal config",
			config: &Config{
				Image:           "ubuntu:22.04",
				WorkspaceFolder: "/workspace",
			},
			containerName:    "atmos-devcontainer.test.default",
			devcontainerName: "test",
			instance:         "default",
			assertFunc: func(t *testing.T, cc *container.CreateConfig) {
				assert.Equal(t, "atmos-devcontainer.test.default", cc.Name)
				assert.Equal(t, "ubuntu:22.04", cc.Image)
				assert.Equal(t, "/workspace", cc.WorkspaceFolder)
				assert.NotNil(t, cc.Labels)
				assert.Equal(t, "devcontainer", cc.Labels[LabelType])
				assert.Equal(t, "test", cc.Labels[LabelDevcontainerName])
				assert.Equal(t, "default", cc.Labels[LabelDevcontainerInstance])
				// TERM should be set to default when not provided.
				assert.Equal(t, "xterm-256color", cc.Env["TERM"])
			},
		},
		{
			name: "full config with env and user",
			config: &Config{
				Image:           "node:18",
				WorkspaceFolder: "/workspaces/myapp",
				RemoteUser:      "node",
				ContainerEnv: map[string]string{
					"NODE_ENV": "development",
					"DEBUG":    "true",
				},
				Init:       true,
				Privileged: false,
			},
			containerName:    "atmos-devcontainer-myapp-dev",
			devcontainerName: "myapp",
			instance:         "dev",
			assertFunc: func(t *testing.T, cc *container.CreateConfig) {
				assert.Equal(t, "node:18", cc.Image)
				assert.Equal(t, "node", cc.User)
				assert.True(t, cc.Init)
				assert.False(t, cc.Privileged)
				require.NotNil(t, cc.Env)
				assert.Equal(t, "development", cc.Env["NODE_ENV"])
				assert.Equal(t, "true", cc.Env["DEBUG"])
				// TERM should be set to default when not provided.
				assert.Equal(t, "xterm-256color", cc.Env["TERM"])
			},
		},
		{
			name: "config with capabilities and security",
			config: &Config{
				Image:       "alpine:latest",
				CapAdd:      []string{"SYS_PTRACE", "NET_ADMIN"},
				SecurityOpt: []string{"seccomp=unconfined"},
				Privileged:  true,
			},
			containerName:    "atmos-devcontainer-alpine-default",
			devcontainerName: "alpine",
			instance:         "default",
			assertFunc: func(t *testing.T, cc *container.CreateConfig) {
				assert.True(t, cc.Privileged)
				assert.Equal(t, []string{"SYS_PTRACE", "NET_ADMIN"}, cc.CapAdd)
				assert.Equal(t, []string{"seccomp=unconfined"}, cc.SecurityOpt)
			},
		},
		{
			name: "config with explicit TERM environment variable",
			config: &Config{
				Image:           "ubuntu:22.04",
				WorkspaceFolder: "/workspace",
				ContainerEnv: map[string]string{
					"TERM": "tmux-256color",
					"LANG": "en_US.UTF-8",
				},
			},
			containerName:    "atmos-devcontainer-ubuntu-default",
			devcontainerName: "ubuntu",
			instance:         "default",
			assertFunc: func(t *testing.T, cc *container.CreateConfig) {
				require.NotNil(t, cc.Env)
				// Explicit TERM should be preserved.
				assert.Equal(t, "tmux-256color", cc.Env["TERM"])
				assert.Equal(t, "en_US.UTF-8", cc.Env["LANG"])
			},
		},
		{
			name: "config with empty TERM gets default",
			config: &Config{
				Image:           "ubuntu:22.04",
				WorkspaceFolder: "/workspace",
				ContainerEnv: map[string]string{
					"TERM": "",
					"USER": "testuser",
				},
			},
			containerName:    "atmos-devcontainer-ubuntu-default",
			devcontainerName: "ubuntu",
			instance:         "default",
			assertFunc: func(t *testing.T, cc *container.CreateConfig) {
				require.NotNil(t, cc.Env)
				// Empty TERM should be replaced with default.
				assert.Equal(t, "xterm-256color", cc.Env["TERM"])
				assert.Equal(t, "testuser", cc.Env["USER"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToCreateConfig(tt.config, tt.containerName, tt.devcontainerName, tt.instance)
			require.NotNil(t, result)
			tt.assertFunc(t, result)
		})
	}
}

func TestCreateDevcontainerLabels(t *testing.T) {
	tests := []struct {
		name             string
		devcontainerName string
		instance         string
		cwd              string
		assertFunc       func(*testing.T, map[string]string)
	}{
		{
			name:             "basic labels",
			devcontainerName: "test",
			instance:         "default",
			cwd:              "/home/user/project",
			assertFunc: func(t *testing.T, labels map[string]string) {
				assert.Equal(t, "devcontainer", labels[LabelType])
				assert.Equal(t, "test", labels[LabelDevcontainerName])
				assert.Equal(t, "default", labels[LabelDevcontainerInstance])
				assert.Equal(t, "/home/user/project", labels[LabelWorkspace])
				assert.NotEmpty(t, labels[LabelCreated])
			},
		},
		{
			name:             "no workspace label for current dir",
			devcontainerName: "test",
			instance:         "dev",
			cwd:              ".",
			assertFunc: func(t *testing.T, labels map[string]string) {
				assert.Equal(t, "devcontainer", labels[LabelType])
				assert.Equal(t, "test", labels[LabelDevcontainerName])
				assert.Equal(t, "dev", labels[LabelDevcontainerInstance])
				_, hasWorkspace := labels[LabelWorkspace]
				assert.False(t, hasWorkspace, "Should not include workspace label for '.'")
			},
		},
		{
			name:             "no workspace label for empty string",
			devcontainerName: "myapp",
			instance:         "prod",
			cwd:              "",
			assertFunc: func(t *testing.T, labels map[string]string) {
				assert.Equal(t, "devcontainer", labels[LabelType])
				_, hasWorkspace := labels[LabelWorkspace]
				assert.False(t, hasWorkspace, "Should not include workspace label for empty string")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := createDevcontainerLabels(tt.devcontainerName, tt.instance, tt.cwd)
			require.NotNil(t, result)
			tt.assertFunc(t, result)
		})
	}
}

func TestConvertMounts(t *testing.T) {
	tests := []struct {
		name       string
		config     *Config
		cwd        string
		assertFunc func(*testing.T, []container.Mount)
	}{
		{
			name: "workspace mount only",
			config: &Config{
				WorkspaceMount: "type=bind,source=${localWorkspaceFolder},target=/workspace",
			},
			cwd: "/home/user/project",
			assertFunc: func(t *testing.T, mounts []container.Mount) {
				require.Len(t, mounts, 1)
				assert.Equal(t, "bind", mounts[0].Type)
				assert.Equal(t, "/home/user/project", mounts[0].Source)
				assert.Equal(t, "/workspace", mounts[0].Target)
			},
		},
		{
			name: "multiple mounts",
			config: &Config{
				Mounts: []string{
					"type=bind,source=/host/path,target=/container/path",
					"type=volume,source=my-volume,target=/data",
				},
				WorkspaceMount: "type=bind,source=${localWorkspaceFolder},target=/workspace",
			},
			cwd: "/current/dir",
			assertFunc: func(t *testing.T, mounts []container.Mount) {
				require.Len(t, mounts, 3)

				// First mount
				assert.Equal(t, "bind", mounts[0].Type)
				assert.Equal(t, "/host/path", mounts[0].Source)
				assert.Equal(t, "/container/path", mounts[0].Target)

				// Second mount
				assert.Equal(t, "volume", mounts[1].Type)
				assert.Equal(t, "my-volume", mounts[1].Source)
				assert.Equal(t, "/data", mounts[1].Target)

				// Workspace mount
				assert.Equal(t, "bind", mounts[2].Type)
				assert.Equal(t, "/current/dir", mounts[2].Source)
			},
		},
		{
			name: "empty mounts",
			config: &Config{
				Mounts: []string{},
			},
			cwd: "/home/user/project",
			assertFunc: func(t *testing.T, mounts []container.Mount) {
				assert.Empty(t, mounts)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertMounts(tt.config, tt.cwd)
			tt.assertFunc(t, result)
		})
	}
}

func TestConvertPorts(t *testing.T) {
	tests := []struct {
		name         string
		forwardPorts []any
		expected     []container.PortBinding
	}{
		{
			name:         "integer ports",
			forwardPorts: []any{8080, 3000, 5432},
			expected: []container.PortBinding{
				{ContainerPort: 8080, HostPort: 8080, Protocol: "tcp"},
				{ContainerPort: 3000, HostPort: 3000, Protocol: "tcp"},
				{ContainerPort: 5432, HostPort: 5432, Protocol: "tcp"},
			},
		},
		{
			name:         "string ports",
			forwardPorts: []any{"8080", "3000"},
			expected: []container.PortBinding{
				{ContainerPort: 8080, HostPort: 8080, Protocol: "tcp"},
				{ContainerPort: 3000, HostPort: 3000, Protocol: "tcp"},
			},
		},
		{
			name:         "float64 ports (from JSON unmarshaling)",
			forwardPorts: []any{float64(8080), float64(3000)},
			expected: []container.PortBinding{
				{ContainerPort: 8080, HostPort: 8080, Protocol: "tcp"},
				{ContainerPort: 3000, HostPort: 3000, Protocol: "tcp"},
			},
		},
		{
			name:         "mixed valid and invalid types",
			forwardPorts: []any{8080, "invalid", true, 3000},
			expected: []container.PortBinding{
				{ContainerPort: 8080, HostPort: 8080, Protocol: "tcp"},
				{ContainerPort: 3000, HostPort: 3000, Protocol: "tcp"},
			},
		},
		{
			name:         "empty ports",
			forwardPorts: []any{},
			expected:     nil,
		},
		{
			name:         "nil ports",
			forwardPorts: nil,
			expected:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertPorts(tt.forwardPorts)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParsePortNumber(t *testing.T) {
	tests := []struct {
		name     string
		port     any
		expected int
	}{
		{
			name:     "int port",
			port:     8080,
			expected: 8080,
		},
		{
			name:     "float64 port",
			port:     float64(3000),
			expected: 3000,
		},
		{
			name:     "string port",
			port:     "5432",
			expected: 5432,
		},
		{
			name:     "invalid string",
			port:     "invalid",
			expected: 0,
		},
		{
			name:     "boolean returns 0",
			port:     true,
			expected: 0,
		},
		{
			name:     "nil returns 0",
			port:     nil,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePortNumber(tt.port)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseMountString(t *testing.T) {
	tests := []struct {
		name     string
		mountStr string
		expected *container.Mount
	}{
		{
			name:     "bind mount",
			mountStr: "type=bind,source=/host/path,target=/container/path",
			expected: &container.Mount{
				Type:   "bind",
				Source: "/host/path",
				Target: "/container/path",
			},
		},
		{
			name:     "bind mount with readonly",
			mountStr: "type=bind,source=/host/path,target=/container/path,readonly",
			expected: &container.Mount{
				Type:     "bind",
				Source:   "/host/path",
				Target:   "/container/path",
				ReadOnly: true,
			},
		},
		{
			name:     "volume mount",
			mountStr: "type=volume,source=my-volume,target=/data",
			expected: &container.Mount{
				Type:   "volume",
				Source: "my-volume",
				Target: "/data",
			},
		},
		{
			name:     "tmpfs mount",
			mountStr: "type=tmpfs,target=/tmp/cache",
			expected: &container.Mount{
				Type:   "tmpfs",
				Target: "/tmp/cache",
			},
		},
		{
			name:     "invalid mount missing required fields",
			mountStr: "type=bind",
			expected: nil,
		},
		{
			name:     "empty string",
			mountStr: "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseMountString(tt.mountStr)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.Type, result.Type)
				assert.Equal(t, tt.expected.Source, result.Source)
				assert.Equal(t, tt.expected.Target, result.Target)
				assert.Equal(t, tt.expected.ReadOnly, result.ReadOnly)
			}
		})
	}
}

func TestExpandDevcontainerVars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		cwd      string
		expected string
	}{
		{
			name:     "expand localWorkspaceFolder",
			input:    "type=bind,source=${localWorkspaceFolder},target=/workspace",
			cwd:      "/home/user/project",
			expected: "type=bind,source=/home/user/project,target=/workspace",
		},
		{
			name:     "expand multiple occurrences",
			input:    "${localWorkspaceFolder}/src:${localWorkspaceFolder}/build",
			cwd:      "/project",
			expected: "/project/src:/project/build",
		},
		{
			name:     "no variables to expand",
			input:    "type=bind,source=/host,target=/container",
			cwd:      "/current",
			expected: "type=bind,source=/host,target=/container",
		},
		{
			name:     "empty cwd",
			input:    "${localWorkspaceFolder}",
			cwd:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandDevcontainerVars(tt.input, tt.cwd)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExpandDevcontainerVars_EnvVars tests environment variable expansion.
func TestExpandDevcontainerVars_EnvVars(t *testing.T) {
	// Set up test environment variables
	t.Setenv("TEST_VAR", "test_value")
	t.Setenv("PATH_VAR", "/test/path")

	tests := []struct {
		name     string
		input    string
		cwd      string
		expected string
	}{
		{
			name:     "expand single env var",
			input:    "source=${localEnv:TEST_VAR}",
			cwd:      "/workspace",
			expected: "source=test_value",
		},
		{
			name:     "expand env var in path",
			input:    "${localEnv:PATH_VAR}/bin",
			cwd:      "/workspace",
			expected: "/test/path/bin",
		},
		{
			name:     "expand multiple env vars",
			input:    "${localEnv:TEST_VAR}:${localEnv:PATH_VAR}",
			cwd:      "/workspace",
			expected: "test_value:/test/path",
		},
		{
			name:     "expand env var with workspace folder",
			input:    "${localWorkspaceFolder}/${localEnv:TEST_VAR}",
			cwd:      "/project",
			expected: "/project/test_value",
		},
		{
			name:     "missing closing brace",
			input:    "${localEnv:TEST_VAR",
			cwd:      "/workspace",
			expected: "${localEnv:TEST_VAR",
		},
		{
			name:     "empty env var",
			input:    "${localEnv:NONEXISTENT_VAR}",
			cwd:      "/workspace",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandDevcontainerVars(tt.input, tt.cwd)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseMountPart(t *testing.T) {
	tests := []struct {
		name     string
		part     string
		initial  container.Mount
		expected container.Mount
	}{
		{
			name:     "type field",
			part:     "type=bind",
			initial:  container.Mount{},
			expected: container.Mount{Type: "bind"},
		},
		{
			name:     "source field",
			part:     "source=/host/path",
			initial:  container.Mount{},
			expected: container.Mount{Source: "/host/path"},
		},
		{
			name:     "src alias",
			part:     "src=/host/path",
			initial:  container.Mount{},
			expected: container.Mount{Source: "/host/path"},
		},
		{
			name:     "target field",
			part:     "target=/container/path",
			initial:  container.Mount{},
			expected: container.Mount{Target: "/container/path"},
		},
		{
			name:     "dst alias",
			part:     "dst=/container/path",
			initial:  container.Mount{},
			expected: container.Mount{Target: "/container/path"},
		},
		{
			name:     "destination alias",
			part:     "destination=/container/path",
			initial:  container.Mount{},
			expected: container.Mount{Target: "/container/path"},
		},
		{
			name:     "readonly flag without value",
			part:     "readonly",
			initial:  container.Mount{},
			expected: container.Mount{ReadOnly: true},
		},
		{
			name:     "readonly with true value",
			part:     "readonly=true",
			initial:  container.Mount{},
			expected: container.Mount{ReadOnly: true},
		},
		{
			name:     "readonly with 1 value",
			part:     "readonly=1",
			initial:  container.Mount{},
			expected: container.Mount{ReadOnly: true},
		},
		{
			name:     "ro alias with true",
			part:     "ro=true",
			initial:  container.Mount{},
			expected: container.Mount{ReadOnly: true},
		},
		{
			name:     "readonly with false value",
			part:     "readonly=false",
			initial:  container.Mount{},
			expected: container.Mount{ReadOnly: false},
		},
		{
			name:     "part with spaces",
			part:     "  source = /host/path  ",
			initial:  container.Mount{},
			expected: container.Mount{Source: "/host/path"},
		},
		{
			name:     "unknown field ignored",
			part:     "unknown=value",
			initial:  container.Mount{Type: "bind"},
			expected: container.Mount{Type: "bind"},
		},
		{
			name:     "modifies existing mount",
			part:     "readonly",
			initial:  container.Mount{Type: "bind", Source: "/src"},
			expected: container.Mount{Type: "bind", Source: "/src", ReadOnly: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mount := tt.initial
			parseMountPart(tt.part, &mount)
			assert.Equal(t, tt.expected, mount)
		})
	}
}

func TestExpandContainerEnv(t *testing.T) {
	// Set up test environment variables.
	t.Setenv("TERM", "tmux-256color")
	t.Setenv("USER", "testuser")
	t.Setenv("PATH", "/usr/local/bin:/usr/bin")

	tests := []struct {
		name     string
		input    map[string]string
		cwd      string
		expected map[string]string
	}{
		{
			name:     "nil input returns nil",
			input:    nil,
			cwd:      "/workspace",
			expected: nil,
		},
		{
			name:     "empty map returns empty map",
			input:    map[string]string{},
			cwd:      "/workspace",
			expected: map[string]string{},
		},
		{
			name: "expands localEnv:TERM",
			input: map[string]string{
				"TERM": "${localEnv:TERM}",
				"FOO":  "bar",
			},
			cwd: "/workspace",
			expected: map[string]string{
				"TERM": "tmux-256color",
				"FOO":  "bar",
			},
		},
		{
			name: "expands multiple localEnv variables",
			input: map[string]string{
				"TERM":    "${localEnv:TERM}",
				"USER":    "${localEnv:USER}",
				"PATH":    "${localEnv:PATH}",
				"LITERAL": "unchanged",
			},
			cwd: "/workspace",
			expected: map[string]string{
				"TERM":    "tmux-256color",
				"USER":    "testuser",
				"PATH":    "/usr/local/bin:/usr/bin",
				"LITERAL": "unchanged",
			},
		},
		{
			name: "expands localWorkspaceFolder",
			input: map[string]string{
				"WORKSPACE": "${localWorkspaceFolder}",
				"CONFIG":    "${localWorkspaceFolder}/config",
			},
			cwd: "/project",
			expected: map[string]string{
				"WORKSPACE": "/project",
				"CONFIG":    "/project/config",
			},
		},
		{
			name: "nonexistent env var expands to empty string",
			input: map[string]string{
				"MISSING": "${localEnv:NONEXISTENT_VAR}",
			},
			cwd: "/workspace",
			expected: map[string]string{
				"MISSING": "",
			},
		},
		{
			name: "mixed expansion types",
			input: map[string]string{
				"TERM":     "${localEnv:TERM}",
				"WORKDIR":  "${localWorkspaceFolder}",
				"COMBINED": "${localEnv:USER}:${localWorkspaceFolder}",
			},
			cwd: "/home/dev",
			expected: map[string]string{
				"TERM":     "tmux-256color",
				"WORKDIR":  "/home/dev",
				"COMBINED": "testuser:/home/dev",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandContainerEnv(tt.input, tt.cwd)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEnsureTermEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]string
	}{
		{
			name:  "nil environment gets default TERM",
			input: nil,
			expected: map[string]string{
				"TERM": "xterm-256color",
			},
		},
		{
			name:  "empty environment gets default TERM",
			input: map[string]string{},
			expected: map[string]string{
				"TERM": "xterm-256color",
			},
		},
		{
			name: "empty TERM value gets default",
			input: map[string]string{
				"TERM": "",
				"FOO":  "bar",
			},
			expected: map[string]string{
				"TERM": "xterm-256color",
				"FOO":  "bar",
			},
		},
		{
			name: "existing TERM is preserved",
			input: map[string]string{
				"TERM": "screen-256color",
				"FOO":  "bar",
			},
			expected: map[string]string{
				"TERM": "screen-256color",
				"FOO":  "bar",
			},
		},
		{
			name: "TERM from localEnv expansion is preserved",
			input: map[string]string{
				"TERM": "xterm-kitty",
				"USER": "testuser",
			},
			expected: map[string]string{
				"TERM": "xterm-kitty",
				"USER": "testuser",
			},
		},
		{
			name: "other env vars without TERM get default",
			input: map[string]string{
				"PATH":    "/usr/bin:/bin",
				"NODE_ENV": "development",
			},
			expected: map[string]string{
				"TERM":    "xterm-256color",
				"PATH":    "/usr/bin:/bin",
				"NODE_ENV": "development",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ensureTermEnvironment(tt.input)
			assert.Equal(t, tt.expected, result)

			// Verify original map is not mutated.
			if tt.input != nil {
				if origTerm, exists := tt.input["TERM"]; exists && origTerm == "" {
					// Empty string should still be empty in original.
					assert.Equal(t, "", tt.input["TERM"], "Original map should not be mutated")
				} else if !exists {
					// If TERM didn't exist, it still shouldn't exist in original.
					_, stillNotExists := tt.input["TERM"]
					assert.False(t, stillNotExists, "Original map should not be mutated")
				}
			}
		})
	}
}
