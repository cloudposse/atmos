package container

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildCreateArgs(t *testing.T) {
	tests := []struct {
		name     string
		config   *CreateConfig
		expected []string
	}{
		{
			name: "minimal config",
			config: &CreateConfig{
				Name:  "test-container",
				Image: "ubuntu:22.04",
			},
			expected: []string{"create", "--name", "test-container", "-it", "ubuntu:22.04"},
		},
		{
			name: "config with labels and env",
			config: &CreateConfig{
				Name:  "test-container",
				Image: "ubuntu:22.04",
				Labels: map[string]string{
					"app":     "test",
					"version": "1.0",
				},
				Env: map[string]string{
					"NODE_ENV": "production",
					"DEBUG":    "false",
				},
			},
			expected: []string{
				"create", "--name", "test-container", "-it",
				"--label", "app=test",
				"--label", "version=1.0",
				"-e", "NODE_ENV=production",
				"-e", "DEBUG=false",
				"ubuntu:22.04",
			},
		},
		{
			name: "config with mounts and ports",
			config: &CreateConfig{
				Name:  "test-container",
				Image: "ubuntu:22.04",
				Mounts: []Mount{
					{Type: "bind", Source: "/host/path", Target: "/container/path"},
					{Type: "volume", Source: "my-volume", Target: "/data", ReadOnly: true},
				},
				Ports: []PortBinding{
					{HostPort: 8080, ContainerPort: 8080, Protocol: "tcp"},
					{HostPort: 3000, ContainerPort: 3000, Protocol: "tcp"},
				},
			},
			expected: []string{
				"create", "--name", "test-container", "-it",
				"--mount", "type=bind,source=/host/path,target=/container/path",
				"--mount", "type=volume,source=my-volume,target=/data,readonly",
				"-p", "8080:8080/tcp",
				"-p", "3000:3000/tcp",
				"ubuntu:22.04",
			},
		},
		{
			name: "config with user and workspace",
			config: &CreateConfig{
				Name:            "test-container",
				Image:           "ubuntu:22.04",
				User:            "node",
				WorkspaceFolder: "/workspaces/app",
			},
			expected: []string{
				"create", "--name", "test-container", "-it",
				"--user", "node",
				"-w", "/workspaces/app",
				"ubuntu:22.04",
			},
		},
		{
			name: "config with runtime flags",
			config: &CreateConfig{
				Name:       "test-container",
				Image:      "ubuntu:22.04",
				Init:       true,
				Privileged: true,
				CapAdd:     []string{"SYS_PTRACE", "NET_ADMIN"},
				SecurityOpt: []string{
					"seccomp=unconfined",
					"apparmor=unconfined",
				},
			},
			expected: []string{
				"create", "--name", "test-container", "-it",
				"--init",
				"--privileged",
				"--cap-add", "SYS_PTRACE",
				"--cap-add", "NET_ADMIN",
				"--security-opt", "seccomp=unconfined",
				"--security-opt", "apparmor=unconfined",
				"ubuntu:22.04",
			},
		},
		{
			name: "config with override command",
			config: &CreateConfig{
				Name:            "test-container",
				Image:           "ubuntu:22.04",
				OverrideCommand: true,
			},
			expected: []string{
				"create", "--name", "test-container", "-it",
				"--entrypoint", "/bin/sh",
				"ubuntu:22.04",
				"-c", "sleep infinity",
			},
		},
		{
			name: "config with run args",
			config: &CreateConfig{
				Name:    "test-container",
				Image:   "ubuntu:22.04",
				RunArgs: []string{"--rm", "--network=host"},
			},
			expected: []string{
				"create", "--name", "test-container", "-it",
				"--rm", "--network=host",
				"ubuntu:22.04",
			},
		},
		{
			name: "comprehensive config with all options",
			config: &CreateConfig{
				Name:            "test-container",
				Image:           "node:18",
				Init:            true,
				Privileged:      false,
				CapAdd:          []string{"SYS_PTRACE"},
				SecurityOpt:     []string{"seccomp=unconfined"},
				Labels:          map[string]string{"type": "devcontainer"},
				Env:             map[string]string{"NODE_ENV": "development"},
				Mounts:          []Mount{{Type: "bind", Source: "/src", Target: "/workspace"}},
				Ports:           []PortBinding{{HostPort: 3000, ContainerPort: 3000, Protocol: "tcp"}},
				User:            "node",
				WorkspaceFolder: "/workspace",
				RunArgs:         []string{"--network=bridge"},
				OverrideCommand: true,
			},
			expected: []string{
				"create", "--name", "test-container", "-it",
				"--init",
				"--cap-add", "SYS_PTRACE",
				"--security-opt", "seccomp=unconfined",
				"--label", "type=devcontainer",
				"-e", "NODE_ENV=development",
				"--mount", "type=bind,source=/src,target=/workspace",
				"-p", "3000:3000/tcp",
				"--user", "node",
				"-w", "/workspace",
				"--network=bridge",
				"--entrypoint", "/bin/sh",
				"node:18",
				"-c", "sleep infinity",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildCreateArgs(tt.config)

			// For configs with maps (labels, env), we need to check presence
			// rather than exact order since map iteration is non-deterministic.
			if len(tt.config.Labels) > 0 || len(tt.config.Env) > 0 {
				// Verify all expected args are present.
				for _, expectedArg := range tt.expected {
					assert.Contains(t, result, expectedArg)
				}
				// Verify length matches (no extra args).
				assert.Equal(t, len(tt.expected), len(result))
			} else {
				// For configs without maps, order is deterministic.
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestAddRuntimeFlags(t *testing.T) {
	tests := []struct {
		name     string
		config   *CreateConfig
		expected []string
	}{
		{
			name:     "no runtime flags",
			config:   &CreateConfig{},
			expected: []string{},
		},
		{
			name: "init flag only",
			config: &CreateConfig{
				Init: true,
			},
			expected: []string{"--init"},
		},
		{
			name: "privileged flag only",
			config: &CreateConfig{
				Privileged: true,
			},
			expected: []string{"--privileged"},
		},
		{
			name: "single capability",
			config: &CreateConfig{
				CapAdd: []string{"SYS_PTRACE"},
			},
			expected: []string{"--cap-add", "SYS_PTRACE"},
		},
		{
			name: "multiple capabilities",
			config: &CreateConfig{
				CapAdd: []string{"SYS_PTRACE", "NET_ADMIN", "SYS_ADMIN"},
			},
			expected: []string{
				"--cap-add", "SYS_PTRACE",
				"--cap-add", "NET_ADMIN",
				"--cap-add", "SYS_ADMIN",
			},
		},
		{
			name: "security options",
			config: &CreateConfig{
				SecurityOpt: []string{"seccomp=unconfined", "apparmor=unconfined"},
			},
			expected: []string{
				"--security-opt", "seccomp=unconfined",
				"--security-opt", "apparmor=unconfined",
			},
		},
		{
			name: "all runtime flags",
			config: &CreateConfig{
				Init:        true,
				Privileged:  true,
				CapAdd:      []string{"SYS_PTRACE"},
				SecurityOpt: []string{"seccomp=unconfined"},
			},
			expected: []string{
				"--init",
				"--privileged",
				"--cap-add", "SYS_PTRACE",
				"--security-opt", "seccomp=unconfined",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addRuntimeFlags([]string{}, tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAddMetadata(t *testing.T) {
	tests := []struct {
		name     string
		config   *CreateConfig
		expected []string
	}{
		{
			name:     "no metadata",
			config:   &CreateConfig{},
			expected: []string{},
		},
		{
			name: "labels only",
			config: &CreateConfig{
				Labels: map[string]string{
					"app":     "test",
					"version": "1.0",
				},
			},
			expected: []string{
				"--label", "app=test",
				"--label", "version=1.0",
			},
		},
		{
			name: "env only",
			config: &CreateConfig{
				Env: map[string]string{
					"NODE_ENV": "production",
					"PORT":     "3000",
				},
			},
			expected: []string{
				"-e", "NODE_ENV=production",
				"-e", "PORT=3000",
			},
		},
		{
			name: "labels and env",
			config: &CreateConfig{
				Labels: map[string]string{
					"type": "devcontainer",
				},
				Env: map[string]string{
					"DEBUG": "true",
				},
			},
			expected: []string{
				"--label", "type=devcontainer",
				"-e", "DEBUG=true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addMetadata([]string{}, tt.config)

			// Map iteration is non-deterministic, so check presence.
			for _, expectedArg := range tt.expected {
				assert.Contains(t, result, expectedArg)
			}
			assert.Equal(t, len(tt.expected), len(result))
		})
	}
}

func TestAddResourceBindings(t *testing.T) {
	tests := []struct {
		name     string
		config   *CreateConfig
		expected []string
	}{
		{
			name:     "no resource bindings",
			config:   &CreateConfig{},
			expected: []string{},
		},
		{
			name: "mount without readonly",
			config: &CreateConfig{
				Mounts: []Mount{
					{Type: "bind", Source: "/host", Target: "/container"},
				},
			},
			expected: []string{
				"--mount", "type=bind,source=/host,target=/container",
			},
		},
		{
			name: "mount with readonly",
			config: &CreateConfig{
				Mounts: []Mount{
					{Type: "bind", Source: "/host", Target: "/container", ReadOnly: true},
				},
			},
			expected: []string{
				"--mount", "type=bind,source=/host,target=/container,readonly",
			},
		},
		{
			name: "multiple mounts",
			config: &CreateConfig{
				Mounts: []Mount{
					{Type: "bind", Source: "/src", Target: "/workspace"},
					{Type: "volume", Source: "my-vol", Target: "/data", ReadOnly: true},
				},
			},
			expected: []string{
				"--mount", "type=bind,source=/src,target=/workspace",
				"--mount", "type=volume,source=my-vol,target=/data,readonly",
			},
		},
		{
			name: "port bindings",
			config: &CreateConfig{
				Ports: []PortBinding{
					{HostPort: 8080, ContainerPort: 8080, Protocol: "tcp"},
					{HostPort: 9090, ContainerPort: 9090, Protocol: "udp"},
				},
			},
			expected: []string{
				"-p", "8080:8080/tcp",
				"-p", "9090:9090/udp",
			},
		},
		{
			name: "user only",
			config: &CreateConfig{
				User: "node",
			},
			expected: []string{
				"--user", "node",
			},
		},
		{
			name: "workspace folder only",
			config: &CreateConfig{
				WorkspaceFolder: "/workspaces/app",
			},
			expected: []string{
				"-w", "/workspaces/app",
			},
		},
		{
			name: "all resource bindings",
			config: &CreateConfig{
				Mounts:          []Mount{{Type: "bind", Source: "/src", Target: "/workspace"}},
				Ports:           []PortBinding{{HostPort: 3000, ContainerPort: 3000, Protocol: "tcp"}},
				User:            "node",
				WorkspaceFolder: "/workspace",
			},
			expected: []string{
				"--mount", "type=bind,source=/src,target=/workspace",
				"-p", "3000:3000/tcp",
				"--user", "node",
				"-w", "/workspace",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addResourceBindings([]string{}, tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAddImageAndCommand(t *testing.T) {
	tests := []struct {
		name     string
		config   *CreateConfig
		expected []string
	}{
		{
			name: "image only",
			config: &CreateConfig{
				Image: "ubuntu:22.04",
			},
			expected: []string{"ubuntu:22.04"},
		},
		{
			name: "image with run args",
			config: &CreateConfig{
				Image:   "ubuntu:22.04",
				RunArgs: []string{"--rm", "--network=host"},
			},
			expected: []string{"--rm", "--network=host", "ubuntu:22.04"},
		},
		{
			name: "image with override command",
			config: &CreateConfig{
				Image:           "ubuntu:22.04",
				OverrideCommand: true,
			},
			expected: []string{
				"--entrypoint", "/bin/sh",
				"ubuntu:22.04",
				"-c", "sleep infinity",
			},
		},
		{
			name: "image with run args and override command",
			config: &CreateConfig{
				Image:           "ubuntu:22.04",
				RunArgs:         []string{"--rm"},
				OverrideCommand: true,
			},
			expected: []string{
				"--rm",
				"--entrypoint", "/bin/sh",
				"ubuntu:22.04",
				"-c", "sleep infinity",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addImageAndCommand([]string{}, tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildExecArgs(t *testing.T) {
	tests := []struct {
		name        string
		containerID string
		cmd         []string
		opts        *ExecOptions
		expected    []string
	}{
		{
			name:        "minimal exec without options",
			containerID: "abc123",
			cmd:         []string{"ls", "-la"},
			opts:        nil,
			expected:    []string{"exec", "abc123", "ls", "-la"},
		},
		{
			name:        "exec with user",
			containerID: "abc123",
			cmd:         []string{"whoami"},
			opts: &ExecOptions{
				User: "node",
			},
			expected: []string{"exec", "--user", "node", "abc123", "whoami"},
		},
		{
			name:        "exec with working directory",
			containerID: "abc123",
			cmd:         []string{"pwd"},
			opts: &ExecOptions{
				WorkingDir: "/workspace",
			},
			expected: []string{"exec", "-w", "/workspace", "abc123", "pwd"},
		},
		{
			name:        "exec with tty",
			containerID: "abc123",
			cmd:         []string{"bash"},
			opts: &ExecOptions{
				Tty: true,
			},
			expected: []string{"exec", "-t", "abc123", "bash"},
		},
		{
			name:        "exec with attach stdin",
			containerID: "abc123",
			cmd:         []string{"bash"},
			opts: &ExecOptions{
				AttachStdin: true,
			},
			expected: []string{"exec", "-i", "abc123", "bash"},
		},
		{
			name:        "exec with environment variables",
			containerID: "abc123",
			cmd:         []string{"env"},
			opts: &ExecOptions{
				Env: []string{"DEBUG=true", "NODE_ENV=production"},
			},
			expected: []string{
				"exec",
				"-e", "DEBUG=true",
				"-e", "NODE_ENV=production",
				"abc123",
				"env",
			},
		},
		{
			name:        "exec with all options",
			containerID: "abc123",
			cmd:         []string{"bash", "-c", "echo hello"},
			opts: &ExecOptions{
				User:        "node",
				WorkingDir:  "/workspace",
				Tty:         true,
				AttachStdin: true,
				Env:         []string{"DEBUG=true"},
			},
			expected: []string{
				"exec",
				"--user", "node",
				"-w", "/workspace",
				"-t",
				"-i",
				"-e", "DEBUG=true",
				"abc123",
				"bash", "-c", "echo hello",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildExecArgs(tt.containerID, tt.cmd, tt.opts)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAddExecOptions(t *testing.T) {
	tests := []struct {
		name     string
		opts     *ExecOptions
		expected []string
	}{
		{
			name:     "no options",
			opts:     &ExecOptions{},
			expected: []string{},
		},
		{
			name: "user only",
			opts: &ExecOptions{
				User: "node",
			},
			expected: []string{"--user", "node"},
		},
		{
			name: "working directory only",
			opts: &ExecOptions{
				WorkingDir: "/workspace",
			},
			expected: []string{"-w", "/workspace"},
		},
		{
			name: "tty only",
			opts: &ExecOptions{
				Tty: true,
			},
			expected: []string{"-t"},
		},
		{
			name: "attach stdin only",
			opts: &ExecOptions{
				AttachStdin: true,
			},
			expected: []string{"-i"},
		},
		{
			name: "environment variables",
			opts: &ExecOptions{
				Env: []string{"DEBUG=true", "NODE_ENV=production"},
			},
			expected: []string{
				"-e", "DEBUG=true",
				"-e", "NODE_ENV=production",
			},
		},
		{
			name: "all options",
			opts: &ExecOptions{
				User:        "node",
				WorkingDir:  "/workspace",
				Tty:         true,
				AttachStdin: true,
				Env:         []string{"DEBUG=true"},
			},
			expected: []string{
				"--user", "node",
				"-w", "/workspace",
				"-t",
				"-i",
				"-e", "DEBUG=true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addExecOptions([]string{}, tt.opts)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildAttachCommand(t *testing.T) {
	tests := []struct {
		name            string
		opts            *AttachOptions
		expectedCmd     []string
		expectedExecOpt *ExecOptions
	}{
		{
			name:        "nil options - uses defaults",
			opts:        nil,
			expectedCmd: []string{"/bin/bash"},
			expectedExecOpt: &ExecOptions{
				Tty:          true,
				AttachStdin:  true,
				AttachStdout: true,
				AttachStderr: true,
			},
		},
		{
			name:        "empty options - uses defaults",
			opts:        &AttachOptions{},
			expectedCmd: []string{"/bin/bash"},
			expectedExecOpt: &ExecOptions{
				Tty:          true,
				AttachStdin:  true,
				AttachStdout: true,
				AttachStderr: true,
			},
		},
		{
			name: "custom shell",
			opts: &AttachOptions{
				Shell: "/bin/sh",
			},
			expectedCmd: []string{"/bin/sh"},
			expectedExecOpt: &ExecOptions{
				Tty:          true,
				AttachStdin:  true,
				AttachStdout: true,
				AttachStderr: true,
			},
		},
		{
			name: "shell with args",
			opts: &AttachOptions{
				Shell:     "/bin/bash",
				ShellArgs: []string{"-l", "-i"},
			},
			expectedCmd: []string{"/bin/bash", "-l", "-i"},
			expectedExecOpt: &ExecOptions{
				Tty:          true,
				AttachStdin:  true,
				AttachStdout: true,
				AttachStderr: true,
			},
		},
		{
			name: "custom user",
			opts: &AttachOptions{
				User: "node",
			},
			expectedCmd: []string{"/bin/bash"},
			expectedExecOpt: &ExecOptions{
				Tty:          true,
				AttachStdin:  true,
				AttachStdout: true,
				AttachStderr: true,
				User:         "node",
			},
		},
		{
			name: "all options",
			opts: &AttachOptions{
				Shell:     "/bin/zsh",
				ShellArgs: []string{"-c", "echo hello"},
				User:      "developer",
			},
			expectedCmd: []string{"/bin/zsh", "-c", "echo hello"},
			expectedExecOpt: &ExecOptions{
				Tty:          true,
				AttachStdin:  true,
				AttachStdout: true,
				AttachStderr: true,
				User:         "developer",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, execOpts := buildAttachCommand(tt.opts)

			// Verify command.
			assert.Equal(t, tt.expectedCmd, cmd, "command should match expected")

			// Verify exec options.
			assert.Equal(t, tt.expectedExecOpt, execOpts, "exec options should match expected")
		})
	}
}

func TestBuildBuildArgs(t *testing.T) {
	tests := []struct {
		name     string
		config   *BuildConfig
		expected []string
	}{
		{
			name: "minimal build - no args or tags",
			config: &BuildConfig{
				Dockerfile: "Dockerfile",
				Context:    ".",
			},
			expected: []string{"build", "-f", "Dockerfile", "."},
		},
		{
			name: "build with build args",
			config: &BuildConfig{
				Dockerfile: "Dockerfile",
				Context:    ".",
				Args: map[string]string{
					"NODE_VERSION": "18",
					"APP_ENV":      "production",
				},
			},
			expected: []string{
				"build",
				"--build-arg", "NODE_VERSION=18",
				"--build-arg", "APP_ENV=production",
				"-f", "Dockerfile", ".",
			},
		},
		{
			name: "build with tags",
			config: &BuildConfig{
				Dockerfile: "Dockerfile",
				Context:    ".",
				Tags:       []string{"myapp:latest", "myapp:v1.0"},
			},
			expected: []string{
				"build",
				"-t", "myapp:latest",
				"-t", "myapp:v1.0",
				"-f", "Dockerfile", ".",
			},
		},
		{
			name: "build with custom dockerfile and context",
			config: &BuildConfig{
				Dockerfile: "docker/Dockerfile.prod",
				Context:    "./app",
			},
			expected: []string{"build", "-f", "docker/Dockerfile.prod", "./app"},
		},
		{
			name: "build with all options",
			config: &BuildConfig{
				Dockerfile: "Dockerfile.dev",
				Context:    "/path/to/context",
				Args: map[string]string{
					"VERSION": "1.0",
				},
				Tags: []string{"myapp:dev", "myapp:latest"},
			},
			expected: []string{
				"build",
				"--build-arg", "VERSION=1.0",
				"-t", "myapp:dev",
				"-t", "myapp:latest",
				"-f", "Dockerfile.dev", "/path/to/context",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildBuildArgs(tt.config)

			// For configs with maps (args), we need to check presence rather than exact order.
			if len(tt.config.Args) > 0 {
				// Verify all expected args are present.
				for _, expectedArg := range tt.expected {
					assert.Contains(t, result, expectedArg)
				}
				// Verify length matches.
				assert.Equal(t, len(tt.expected), len(result))
			} else {
				// For configs without maps, order is deterministic.
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestBuildRemoveArgs(t *testing.T) {
	tests := []struct {
		name        string
		containerID string
		force       bool
		expected    []string
	}{
		{
			name:        "remove without force",
			containerID: "abc123",
			force:       false,
			expected:    []string{"rm", "abc123"},
		},
		{
			name:        "remove with force",
			containerID: "abc123",
			force:       true,
			expected:    []string{"rm", "-f", "abc123"},
		},
		{
			name:        "remove container with name",
			containerID: "my-container",
			force:       false,
			expected:    []string{"rm", "my-container"},
		},
		{
			name:        "force remove container with name",
			containerID: "my-container",
			force:       true,
			expected:    []string{"rm", "-f", "my-container"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildRemoveArgs(tt.containerID, tt.force)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildStopArgs(t *testing.T) {
	tests := []struct {
		name        string
		containerID string
		timeoutSecs int
		expected    []string
	}{
		{
			name:        "stop with default timeout",
			containerID: "abc123",
			timeoutSecs: 10,
			expected:    []string{"stop", "-t", "10", "abc123"},
		},
		{
			name:        "stop with zero timeout",
			containerID: "abc123",
			timeoutSecs: 0,
			expected:    []string{"stop", "-t", "0", "abc123"},
		},
		{
			name:        "stop with long timeout",
			containerID: "abc123",
			timeoutSecs: 300,
			expected:    []string{"stop", "-t", "300", "abc123"},
		},
		{
			name:        "stop container with name",
			containerID: "my-container",
			timeoutSecs: 15,
			expected:    []string{"stop", "-t", "15", "my-container"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildStopArgs(tt.containerID, tt.timeoutSecs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildLogsArgs(t *testing.T) {
	tests := []struct {
		name        string
		containerID string
		follow      bool
		tail        string
		expected    []string
	}{
		{
			name:        "logs without options",
			containerID: "abc123",
			follow:      false,
			tail:        "",
			expected:    []string{"logs", "abc123"},
		},
		{
			name:        "logs with follow",
			containerID: "abc123",
			follow:      true,
			tail:        "",
			expected:    []string{"logs", "--follow", "abc123"},
		},
		{
			name:        "logs with tail",
			containerID: "abc123",
			follow:      false,
			tail:        "100",
			expected:    []string{"logs", "--tail", "100", "abc123"},
		},
		{
			name:        "logs with tail=all (should be ignored)",
			containerID: "abc123",
			follow:      false,
			tail:        "all",
			expected:    []string{"logs", "abc123"},
		},
		{
			name:        "logs with follow and tail",
			containerID: "abc123",
			follow:      true,
			tail:        "50",
			expected:    []string{"logs", "--follow", "--tail", "50", "abc123"},
		},
		{
			name:        "logs for named container",
			containerID: "my-container",
			follow:      false,
			tail:        "10",
			expected:    []string{"logs", "--tail", "10", "my-container"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildLogsArgs(tt.containerID, tt.follow, tt.tail)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestType_String(t *testing.T) {
	tests := []struct {
		name     string
		typ      Type
		expected string
	}{
		{
			name:     "Docker type",
			typ:      TypeDocker,
			expected: "docker",
		},
		{
			name:     "Podman type",
			typ:      TypePodman,
			expected: "podman",
		},
		{
			name:     "Custom type",
			typ:      Type("custom"),
			expected: "custom",
		},
		{
			name:     "Empty type",
			typ:      Type(""),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.typ.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}
