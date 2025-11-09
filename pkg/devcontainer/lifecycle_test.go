package devcontainer

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestPrintSettings(t *testing.T) {
	tests := []struct {
		name             string
		settings         *Settings
		expectedContains []string
	}{
		{
			name: "with runtime setting",
			settings: &Settings{
				Runtime: "docker",
			},
			expectedContains: []string{"Runtime: docker"},
		},
		{
			name:             "with empty runtime",
			settings:         &Settings{},
			expectedContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				printSettings(tt.settings)
			})

			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestPrintBasicInfo(t *testing.T) {
	config := &Config{
		Name:  "test-container",
		Image: "ubuntu:22.04",
	}

	output := captureStdout(func() {
		printBasicInfo(config)
	})

	assert.Contains(t, output, "Name: test-container")
	assert.Contains(t, output, "Image: ubuntu:22.04")
}

func TestPrintBuildInfo(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		expectedContains []string
	}{
		{
			name: "with build config and args",
			config: &Config{
				Build: &Build{
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
			config: &Config{
				Build: &Build{
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
			config:           &Config{},
			expectedContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				printBuildInfo(tt.config)
			})

			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestPrintWorkspaceInfo(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		expectedContains []string
	}{
		{
			name: "with both folder and mount",
			config: &Config{
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
			config: &Config{
				WorkspaceFolder: "/app",
			},
			expectedContains: []string{
				"Workspace Folder: /app",
			},
		},
		{
			name:             "without workspace info",
			config:           &Config{},
			expectedContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				printWorkspaceInfo(tt.config)
			})

			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestPrintMounts(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		expectedContains []string
	}{
		{
			name: "with mounts",
			config: &Config{
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
			config:           &Config{},
			expectedContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				printMounts(tt.config)
			})

			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestPrintPorts(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		expectedContains []string
	}{
		{
			name: "with forward ports",
			config: &Config{
				ForwardPorts: []interface{}{8080, "3000:3001"},
			},
			expectedContains: []string{
				"Forward Ports:",
			},
		},
		{
			name:             "without ports",
			config:           &Config{},
			expectedContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				printPorts(tt.config)
			})

			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestPrintEnv(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		expectedContains []string
	}{
		{
			name: "with environment variables",
			config: &Config{
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
			config:           &Config{},
			expectedContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				printEnv(tt.config)
			})

			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestPrintRunArgs(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		expectedContains []string
	}{
		{
			name: "with run arguments",
			config: &Config{
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
			config:           &Config{},
			expectedContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				printRunArgs(tt.config)
			})

			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestPrintRemoteUser(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		expectedContains []string
	}{
		{
			name: "with remote user",
			config: &Config{
				RemoteUser: "node",
			},
			expectedContains: []string{
				"Remote User: node",
			},
		},
		{
			name:             "without remote user",
			config:           &Config{},
			expectedContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				printRemoteUser(tt.config)
			})

			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}
