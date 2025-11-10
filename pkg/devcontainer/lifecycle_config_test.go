package devcontainer

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestManager_ShowConfig(t *testing.T) {
	tests := []struct {
		name        string
		devName     string
		setupMocks  func(*MockConfigLoader)
		expectError bool
	}{
		{
			name:    "show config successfully",
			devName: "test",
			setupMocks: func(loader *MockConfigLoader) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{
						Name:  "test",
						Image: "ubuntu:22.04",
					}, &Settings{
						Runtime: "docker",
					}, nil)
			},
			expectError: false,
		},
		{
			name:    "show config with all fields",
			devName: "test",
			setupMocks: func(loader *MockConfigLoader) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{
						Name:  "test",
						Image: "ubuntu:22.04",
						Build: &Build{
							Dockerfile: "Dockerfile",
							Context:    ".",
							Args: map[string]string{
								"VERSION": "1.0",
							},
						},
						WorkspaceFolder: "/workspace",
						WorkspaceMount:  "source=/host,target=/workspace,type=bind",
						Mounts:          []string{"type=bind,source=/host,target=/container"},
						ForwardPorts:    []interface{}{8080, "9000:9000"},
						ContainerEnv: map[string]string{
							"FOO": "bar",
						},
						RunArgs:    []string{"--privileged"},
						RemoteUser: "vscode",
					}, &Settings{
						Runtime: "docker",
					}, nil)
			},
			expectError: false,
		},
		{
			name:    "config load fails",
			devName: "test",
			setupMocks: func(loader *MockConfigLoader) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(nil, nil, errors.New("config not found"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockLoader := NewMockConfigLoader(ctrl)
			if tt.setupMocks != nil {
				tt.setupMocks(mockLoader)
			}

			mgr := NewManager(
				WithConfigLoader(mockLoader),
			)

			// Capture stdout.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				_ = w.Close()
				os.Stdout = oldStdout
			}()

			err := mgr.ShowConfig(&schema.AtmosConfiguration{}, tt.devName)

			// Read captured output.
			_ = w.Close()
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				// Verify that some output was produced.
				assert.NotEmpty(t, buf.String())
			}
		})
	}
}

func TestPrintSettings(t *testing.T) {
	tests := []struct {
		name             string
		settings         *Settings
		expectedContains string
	}{
		{
			name: "prints runtime",
			settings: &Settings{
				Runtime: "docker",
			},
			expectedContains: "Runtime: docker",
		},
		{
			name:             "empty runtime - no output",
			settings:         &Settings{},
			expectedContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				_ = w.Close()
				os.Stdout = oldStdout
			}()

			printSettings(tt.settings)

			_ = w.Close()
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)

			output := buf.String()
			if tt.expectedContains != "" {
				assert.Contains(t, output, tt.expectedContains)
			}
		})
	}
}

func TestPrintBasicInfo(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		expectedContains []string
	}{
		{
			name: "prints name and image",
			config: &Config{
				Name:  "test",
				Image: "ubuntu:22.04",
			},
			expectedContains: []string{
				"Name: test",
				"Image: ubuntu:22.04",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				_ = w.Close()
				os.Stdout = oldStdout
			}()

			printBasicInfo(tt.config)

			_ = w.Close()
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)

			output := buf.String()
			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestPrintBuildInfo(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		expectedContains []string
		expectEmpty      bool
	}{
		{
			name: "prints build info",
			config: &Config{
				Build: &Build{
					Dockerfile: "Dockerfile",
					Context:    ".",
					Args: map[string]string{
						"VERSION": "1.0",
					},
				},
			},
			expectedContains: []string{
				"Build:",
				"Dockerfile: Dockerfile",
				"Context: .",
				"Args:",
				"VERSION: 1.0",
			},
		},
		{
			name: "no build config - no output",
			config: &Config{
				Build: nil,
			},
			expectEmpty: true,
		},
		{
			name: "build config without args",
			config: &Config{
				Build: &Build{
					Dockerfile: "Dockerfile",
					Context:    ".",
				},
			},
			expectedContains: []string{
				"Build:",
				"Dockerfile: Dockerfile",
				"Context: .",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				_ = w.Close()
				os.Stdout = oldStdout
			}()

			printBuildInfo(tt.config)

			_ = w.Close()
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)

			output := buf.String()
			if tt.expectEmpty {
				assert.Empty(t, output)
			} else {
				for _, expected := range tt.expectedContains {
					assert.Contains(t, output, expected)
				}
			}
		})
	}
}

func TestPrintWorkspaceInfo(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		expectedContains []string
		expectEmpty      bool
	}{
		{
			name: "prints workspace folder",
			config: &Config{
				WorkspaceFolder: "/workspace",
			},
			expectedContains: []string{
				"Workspace Folder: /workspace",
			},
		},
		{
			name: "prints workspace mount",
			config: &Config{
				WorkspaceMount: "source=/host,target=/workspace,type=bind",
			},
			expectedContains: []string{
				"Workspace Mount: source=/host,target=/workspace,type=bind",
			},
		},
		{
			name: "prints both",
			config: &Config{
				WorkspaceFolder: "/workspace",
				WorkspaceMount:  "source=/host,target=/workspace,type=bind",
			},
			expectedContains: []string{
				"Workspace Folder: /workspace",
				"Workspace Mount: source=/host,target=/workspace,type=bind",
			},
		},
		{
			name:        "no workspace config - no output",
			config:      &Config{},
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				_ = w.Close()
				os.Stdout = oldStdout
			}()

			printWorkspaceInfo(tt.config)

			_ = w.Close()
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)

			output := buf.String()
			if tt.expectEmpty {
				assert.Empty(t, output)
			} else {
				for _, expected := range tt.expectedContains {
					assert.Contains(t, output, expected)
				}
			}
		})
	}
}

func TestPrintMounts(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		expectedContains []string
		expectEmpty      bool
	}{
		{
			name: "prints mounts",
			config: &Config{
				Mounts: []string{
					"type=bind,source=/host,target=/container",
					"type=volume,source=data,target=/data",
				},
			},
			expectedContains: []string{
				"Mounts:",
				"type=bind,source=/host,target=/container",
				"type=volume,source=data,target=/data",
			},
		},
		{
			name: "no mounts - no output",
			config: &Config{
				Mounts: []string{},
			},
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				_ = w.Close()
				os.Stdout = oldStdout
			}()

			printMounts(tt.config)

			_ = w.Close()
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)

			output := buf.String()
			if tt.expectEmpty {
				assert.Empty(t, output)
			} else {
				for _, expected := range tt.expectedContains {
					assert.Contains(t, output, expected)
				}
			}
		})
	}
}

func TestPrintPorts(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		expectedContains []string
		expectEmpty      bool
	}{
		{
			name: "prints simple ports",
			config: &Config{
				ForwardPorts: []interface{}{8080, 9000},
			},
			expectedContains: []string{
				"Forward Ports:",
				"8080",
				"9000",
			},
		},
		{
			name: "prints port mappings",
			config: &Config{
				ForwardPorts: []interface{}{"3000:8080"},
			},
			expectedContains: []string{
				"Forward Ports:",
				"3000:8080",
			},
		},
		{
			name: "no ports - no output",
			config: &Config{
				ForwardPorts: []interface{}{},
			},
			expectEmpty: true,
		},
		{
			name: "invalid ports - shows error",
			config: &Config{
				ForwardPorts: []interface{}{"invalid"},
			},
			expectedContains: []string{
				"Error parsing ports",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				_ = w.Close()
				os.Stdout = oldStdout
			}()

			printPorts(tt.config)

			_ = w.Close()
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)

			output := buf.String()
			if tt.expectEmpty {
				assert.Empty(t, output)
			} else {
				for _, expected := range tt.expectedContains {
					assert.Contains(t, output, expected)
				}
			}
		})
	}
}

func TestPrintEnv(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		expectedContains []string
		expectEmpty      bool
	}{
		{
			name: "prints environment variables",
			config: &Config{
				ContainerEnv: map[string]string{
					"FOO": "bar",
					"BAZ": "qux",
				},
			},
			expectedContains: []string{
				"Environment Variables:",
				"FOO: bar",
				"BAZ: qux",
			},
		},
		{
			name: "no env vars - no output",
			config: &Config{
				ContainerEnv: map[string]string{},
			},
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				_ = w.Close()
				os.Stdout = oldStdout
			}()

			printEnv(tt.config)

			_ = w.Close()
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)

			output := buf.String()
			if tt.expectEmpty {
				assert.Empty(t, output)
			} else {
				for _, expected := range tt.expectedContains {
					assert.Contains(t, output, expected)
				}
			}
		})
	}
}

func TestPrintRunArgs(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		expectedContains []string
		expectEmpty      bool
	}{
		{
			name: "prints run arguments",
			config: &Config{
				RunArgs: []string{"--privileged", "--cap-add=SYS_PTRACE"},
			},
			expectedContains: []string{
				"Run Arguments:",
				"--privileged",
				"--cap-add=SYS_PTRACE",
			},
		},
		{
			name: "no run args - no output",
			config: &Config{
				RunArgs: []string{},
			},
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				_ = w.Close()
				os.Stdout = oldStdout
			}()

			printRunArgs(tt.config)

			_ = w.Close()
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)

			output := buf.String()
			if tt.expectEmpty {
				assert.Empty(t, output)
			} else {
				for _, expected := range tt.expectedContains {
					assert.Contains(t, output, expected)
				}
			}
		})
	}
}

func TestPrintRemoteUser(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		expectedContains string
		expectEmpty      bool
	}{
		{
			name: "prints remote user",
			config: &Config{
				RemoteUser: "vscode",
			},
			expectedContains: "Remote User: vscode",
		},
		{
			name:        "no remote user - no output",
			config:      &Config{},
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				_ = w.Close()
				os.Stdout = oldStdout
			}()

			printRemoteUser(tt.config)

			_ = w.Close()
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)

			output := buf.String()
			if tt.expectEmpty {
				assert.Empty(t, output)
			} else {
				assert.Contains(t, output, tt.expectedContains)
			}
		})
	}
}
