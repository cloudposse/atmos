package exec

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/devcontainer"
)

func TestGetShellArgs(t *testing.T) {
	tests := []struct {
		name         string
		userEnvProbe string
		expected     []string
	}{
		{
			name:         "loginShell returns -l flag",
			userEnvProbe: "loginShell",
			expected:     []string{"-l"},
		},
		{
			name:         "loginInteractiveShell returns -l flag",
			userEnvProbe: "loginInteractiveShell",
			expected:     []string{"-l"},
		},
		{
			name:         "empty string returns nil",
			userEnvProbe: "",
			expected:     nil,
		},
		{
			name:         "interactiveShell returns nil",
			userEnvProbe: "interactiveShell",
			expected:     nil,
		},
		{
			name:         "none returns nil",
			userEnvProbe: "none",
			expected:     nil,
		},
		{
			name:         "unknown value returns nil",
			userEnvProbe: "unknownValue",
			expected:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getShellArgs(tt.userEnvProbe)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// captureStdout captures stdout during function execution.
func captureStdout(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestPrintDevcontainerSettings(t *testing.T) {
	tests := []struct {
		name             string
		settings         *devcontainer.Settings
		expectedContains []string
	}{
		{
			name: "with runtime setting",
			settings: &devcontainer.Settings{
				Runtime: "docker",
			},
			expectedContains: []string{"Runtime: docker"},
		},
		{
			name:             "with empty runtime",
			settings:         &devcontainer.Settings{},
			expectedContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				printDevcontainerSettings(tt.settings)
			})

			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestPrintDevcontainerBasicInfo(t *testing.T) {
	config := &devcontainer.Config{
		Name:  "test-container",
		Image: "ubuntu:22.04",
	}

	output := captureStdout(func() {
		printDevcontainerBasicInfo(config)
	})

	assert.Contains(t, output, "Name: test-container")
	assert.Contains(t, output, "Image: ubuntu:22.04")
}

func TestPrintDevcontainerBuildInfo(t *testing.T) {
	tests := []struct {
		name             string
		config           *devcontainer.Config
		expectedContains []string
	}{
		{
			name: "with build config and args",
			config: &devcontainer.Config{
				Build: &devcontainer.Build{
					Dockerfile: "Dockerfile",
					Context:    ".",
					Args: map[string]string{
						"VERSION": "1.0",
						"ENV":     "prod",
					},
				},
			},
			expectedContains: []string{
				"Build:",
				"Dockerfile: Dockerfile",
				"Context: .",
				"Args:",
				"VERSION: 1.0",
				"ENV: prod",
			},
		},
		{
			name: "with build config without args",
			config: &devcontainer.Config{
				Build: &devcontainer.Build{
					Dockerfile: "Dockerfile.dev",
					Context:    "/app",
				},
			},
			expectedContains: []string{
				"Build:",
				"Dockerfile: Dockerfile.dev",
				"Context: /app",
			},
		},
		{
			name:             "without build config",
			config:           &devcontainer.Config{},
			expectedContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				printDevcontainerBuildInfo(tt.config)
			})

			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestPrintDevcontainerWorkspaceInfo(t *testing.T) {
	tests := []struct {
		name             string
		config           *devcontainer.Config
		expectedContains []string
	}{
		{
			name: "with both folder and mount",
			config: &devcontainer.Config{
				WorkspaceFolder: "/workspace",
				WorkspaceMount:  "type=bind,source=/host,target=/workspace",
			},
			expectedContains: []string{
				"Workspace Folder: /workspace",
				"Workspace Mount: type=bind,source=/host,target=/workspace",
			},
		},
		{
			name: "with folder only",
			config: &devcontainer.Config{
				WorkspaceFolder: "/app",
			},
			expectedContains: []string{
				"Workspace Folder: /app",
			},
		},
		{
			name:             "without workspace info",
			config:           &devcontainer.Config{},
			expectedContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				printDevcontainerWorkspaceInfo(tt.config)
			})

			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestPrintDevcontainerMounts(t *testing.T) {
	tests := []struct {
		name             string
		config           *devcontainer.Config
		expectedContains []string
	}{
		{
			name: "with mounts",
			config: &devcontainer.Config{
				Mounts: []string{
					"type=bind,source=/host,target=/container",
					"type=volume,source=my-vol,target=/data",
				},
			},
			expectedContains: []string{
				"Mounts:",
				"type=bind,source=/host,target=/container",
				"type=volume,source=my-vol,target=/data",
			},
		},
		{
			name:             "without mounts",
			config:           &devcontainer.Config{},
			expectedContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				printDevcontainerMounts(tt.config)
			})

			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestPrintDevcontainerPorts(t *testing.T) {
	tests := []struct {
		name             string
		config           *devcontainer.Config
		expectedContains []string
	}{
		{
			name: "with forward ports",
			config: &devcontainer.Config{
				ForwardPorts: []interface{}{8080, "3000:3001"},
			},
			expectedContains: []string{
				"Forward Ports:",
			},
		},
		{
			name:             "without ports",
			config:           &devcontainer.Config{},
			expectedContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				printDevcontainerPorts(tt.config)
			})

			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestPrintDevcontainerEnv(t *testing.T) {
	tests := []struct {
		name             string
		config           *devcontainer.Config
		expectedContains []string
	}{
		{
			name: "with environment variables",
			config: &devcontainer.Config{
				ContainerEnv: map[string]string{
					"NODE_ENV": "production",
					"DEBUG":    "true",
				},
			},
			expectedContains: []string{
				"Environment Variables:",
				"NODE_ENV: production",
				"DEBUG: true",
			},
		},
		{
			name:             "without environment variables",
			config:           &devcontainer.Config{},
			expectedContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				printDevcontainerEnv(tt.config)
			})

			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestPrintDevcontainerRunArgs(t *testing.T) {
	tests := []struct {
		name             string
		config           *devcontainer.Config
		expectedContains []string
	}{
		{
			name: "with run arguments",
			config: &devcontainer.Config{
				RunArgs: []string{"--rm", "--network=host"},
			},
			expectedContains: []string{
				"Run Arguments:",
				"--rm",
				"--network=host",
			},
		},
		{
			name:             "without run arguments",
			config:           &devcontainer.Config{},
			expectedContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				printDevcontainerRunArgs(tt.config)
			})

			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestPrintDevcontainerRemoteUser(t *testing.T) {
	tests := []struct {
		name             string
		config           *devcontainer.Config
		expectedContains []string
	}{
		{
			name: "with remote user",
			config: &devcontainer.Config{
				RemoteUser: "node",
			},
			expectedContains: []string{
				"Remote User: node",
			},
		},
		{
			name:             "without remote user",
			config:           &devcontainer.Config{},
			expectedContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				printDevcontainerRemoteUser(tt.config)
			})

			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}
