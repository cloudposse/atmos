package devcontainer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestGetAtmosXDGEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected map[string]string
	}{
		{
			name: "with workspace folder specified",
			config: &Config{
				WorkspaceFolder: "/workspaces/my-project",
			},
			expected: map[string]string{
				"XDG_CONFIG_HOME": "/workspaces/my-project/.atmos",
				"XDG_DATA_HOME":   "/workspaces/my-project/.atmos",
				"XDG_CACHE_HOME":  "/workspaces/my-project/.atmos",
				"ATMOS_BASE_PATH": "/workspaces/my-project",
			},
		},
		{
			name:   "with empty workspace folder defaults to /workspace",
			config: &Config{},
			expected: map[string]string{
				"XDG_CONFIG_HOME": "/workspace/.atmos",
				"XDG_DATA_HOME":   "/workspace/.atmos",
				"XDG_CACHE_HOME":  "/workspace/.atmos",
				"ATMOS_BASE_PATH": "/workspace",
			},
		},
		{
			name: "with custom workspace path",
			config: &Config{
				WorkspaceFolder: "/custom/path",
			},
			expected: map[string]string{
				"XDG_CONFIG_HOME": "/custom/path/.atmos",
				"XDG_DATA_HOME":   "/custom/path/.atmos",
				"XDG_CACHE_HOME":  "/custom/path/.atmos",
				"ATMOS_BASE_PATH": "/custom/path",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getAtmosXDGEnvironment(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTranslatePath(t *testing.T) {
	tests := []struct {
		name               string
		hostFilePath       string
		hostWorkspace      string
		containerWorkspace string
		userHome           string
		expected           string
	}{
		{
			name:               "path under host workspace translates to container workspace",
			hostFilePath:       "/Users/john/projects/my-app/config.yaml",
			hostWorkspace:      "/Users/john/projects/my-app",
			containerWorkspace: "/workspaces/my-app",
			userHome:           "/Users/john",
			expected:           "/workspaces/my-app/config.yaml",
		},
		{
			name:               "path under user home translates to container workspace",
			hostFilePath:       "/Users/john/.aws/config",
			hostWorkspace:      "/Users/john/projects/my-app",
			containerWorkspace: "/workspaces/my-app",
			userHome:           "/Users/john",
			expected:           "/workspaces/my-app/.aws/config",
		},
		{
			name:               "absolute path outside workspace returns unchanged",
			hostFilePath:       "/etc/ssl/certs/ca-bundle.crt",
			hostWorkspace:      "/Users/john/projects/my-app",
			containerWorkspace: "/workspaces/my-app",
			userHome:           "/Users/john",
			expected:           "/etc/ssl/certs/ca-bundle.crt",
		},
		{
			name:               "empty user home uses workspace prefix only",
			hostFilePath:       "/Users/john/projects/my-app/file.txt",
			hostWorkspace:      "/Users/john/projects/my-app",
			containerWorkspace: "/workspaces/my-app",
			userHome:           "",
			expected:           "/workspaces/my-app/file.txt",
		},
		{
			name:               "nested path under workspace",
			hostFilePath:       "/Users/john/projects/my-app/src/main.go",
			hostWorkspace:      "/Users/john/projects/my-app",
			containerWorkspace: "/workspaces/my-app",
			userHome:           "/Users/john",
			expected:           "/workspaces/my-app/src/main.go",
		},
		{
			name:               "Windows path with backslash separator under workspace",
			hostFilePath:       `C:\Users\john\project\config.yaml`,
			hostWorkspace:      `C:\Users\john\project`,
			containerWorkspace: "/workspace",
			userHome:           `C:\Users\john`,
			expected:           "/workspace/config.yaml",
		},
		{
			name:               "Windows path with backslash separator under user home",
			hostFilePath:       `C:\Users\john\.aws\config`,
			hostWorkspace:      `C:\Users\john\project`,
			containerWorkspace: "/workspace",
			userHome:           `C:\Users\john`,
			expected:           "/workspace/.aws/config",
		},
		{
			name:               "nested Windows path with multiple backslashes",
			hostFilePath:       `C:\Users\john\project\stacks\dev\vpc.yaml`,
			hostWorkspace:      `C:\Users\john\project`,
			containerWorkspace: "/workspace",
			userHome:           `C:\Users\john`,
			expected:           "/workspace/stacks/dev/vpc.yaml",
		},
		{
			name:               "path with trailing slash after workspace trim",
			hostFilePath:       "/Users/john/project//config.yaml",
			hostWorkspace:      "/Users/john/project",
			containerWorkspace: "/workspace",
			userHome:           "/Users/john",
			expected:           "/workspace/config.yaml",
		},
		{
			name:               "exact workspace path match (no subdirectory)",
			hostFilePath:       "/Users/john/project",
			hostWorkspace:      "/Users/john/project",
			containerWorkspace: "/workspace",
			userHome:           "/Users/john",
			expected:           "/workspace",
		},
		{
			name:               "exact user home path match (no subdirectory)",
			hostFilePath:       "/Users/john",
			hostWorkspace:      "/Users/john/project",
			containerWorkspace: "/workspace",
			userHome:           "/Users/john",
			expected:           "/workspace",
		},
		{
			name:               "container workspace is not /workspace",
			hostFilePath:       "/Users/john/project/config.yaml",
			hostWorkspace:      "/Users/john/project",
			containerWorkspace: "/app",
			userHome:           "/Users/john",
			expected:           "/app/config.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := translatePath(tt.hostFilePath, tt.hostWorkspace, tt.containerWorkspace, tt.userHome)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseMountPaths(t *testing.T) {
	tests := []struct {
		name              string
		workspaceMount    string
		workspaceFolder   string
		expectedHost      string
		expectedContainer string
	}{
		{
			name:              "parses source and uses workspace folder",
			workspaceMount:    "type=bind,source=/Users/john/projects/my-app,target=/workspaces/my-app",
			workspaceFolder:   "/workspaces/my-app",
			expectedHost:      "/Users/john/projects/my-app",
			expectedContainer: "/workspaces/my-app",
		},
		{
			name:              "empty workspace mount returns current directory",
			workspaceMount:    "",
			workspaceFolder:   "/workspaces/my-app",
			expectedHost:      "", // Will be current directory in actual code
			expectedContainer: "/workspaces/my-app",
		},
		{
			name:              "empty workspace folder defaults to /workspace",
			workspaceMount:    "type=bind,source=/home/user/project,target=/workspace",
			workspaceFolder:   "",
			expectedHost:      "/home/user/project",
			expectedContainer: "/workspace",
		},
		{
			name:              "mount string without source key",
			workspaceMount:    "type=bind,target=/workspaces/my-app",
			workspaceFolder:   "/workspaces/my-app",
			expectedHost:      "", // Will be current directory
			expectedContainer: "/workspaces/my-app",
		},
		{
			name:              "mount string with additional options",
			workspaceMount:    "type=bind,source=/host/path,target=/container/path,readonly",
			workspaceFolder:   "/container/path",
			expectedHost:      "/host/path",
			expectedContainer: "/container/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostPath, containerPath := parseMountPaths(tt.workspaceMount, tt.workspaceFolder)

			// For empty expected host, we expect current directory (non-empty string).
			if tt.expectedHost == "" {
				assert.NotEmpty(t, hostPath, "host path should default to current directory")
			} else {
				assert.Equal(t, tt.expectedHost, hostPath)
			}

			assert.Equal(t, tt.expectedContainer, containerPath)
		})
	}
}

func TestAddCredentialMounts(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		paths       []types.Path
		wantErr     bool
		errContains string
		validate    func(t *testing.T, config *Config)
	}{
		{
			name: "adds mount for existing required path",
			config: &Config{
				WorkspaceMount:  "type=bind,source=/host/project,target=/workspace",
				WorkspaceFolder: "/workspace",
			},
			paths: []types.Path{
				{
					Location: "testdata", // Use testdata directory that exists
					Purpose:  "test directory",
					Required: false,
				},
			},
			wantErr: false,
			validate: func(t *testing.T, config *Config) {
				require.NotNil(t, config.Mounts)
				require.Len(t, config.Mounts, 1)
				assert.Contains(t, config.Mounts[0], "type=bind")
				assert.Contains(t, config.Mounts[0], "testdata")
				assert.Contains(t, config.Mounts[0], "readonly")
			},
		},
		{
			name: "skips optional path that doesn't exist",
			config: &Config{
				WorkspaceMount:  "type=bind,source=/host/project,target=/workspace",
				WorkspaceFolder: "/workspace",
			},
			paths: []types.Path{
				{
					Location: "/nonexistent/optional/path",
					Purpose:  "optional credentials",
					Required: false,
				},
			},
			wantErr: false,
			validate: func(t *testing.T, config *Config) {
				// Mount should not be added for nonexistent optional path.
				assert.Empty(t, config.Mounts)
			},
		},
		{
			name: "errors on required path that doesn't exist",
			config: &Config{
				WorkspaceMount:  "type=bind,source=/host/project,target=/workspace",
				WorkspaceFolder: "/workspace",
			},
			paths: []types.Path{
				{
					Location: "/nonexistent/required/path",
					Purpose:  "required credentials",
					Required: true,
				},
			},
			wantErr:     true,
			errContains: "required credential path",
		},
		{
			name: "respects read_only metadata false",
			config: &Config{
				WorkspaceMount:  "type=bind,source=/host/project,target=/workspace",
				WorkspaceFolder: "/workspace",
			},
			paths: []types.Path{
				{
					Location: "testdata",
					Purpose:  "writable directory",
					Required: false,
					Metadata: map[string]string{
						"read_only": "false",
					},
				},
			},
			wantErr: false,
			validate: func(t *testing.T, config *Config) {
				require.NotNil(t, config.Mounts)
				require.Len(t, config.Mounts, 1)
				assert.Contains(t, config.Mounts[0], "type=bind")
				assert.NotContains(t, config.Mounts[0], "readonly", "mount should not be readonly when metadata specifies false")
			},
		},
		{
			name: "handles empty paths slice",
			config: &Config{
				WorkspaceMount:  "type=bind,source=/host/project,target=/workspace",
				WorkspaceFolder: "/workspace",
			},
			paths:   []types.Path{},
			wantErr: false,
			validate: func(t *testing.T, config *Config) {
				assert.Empty(t, config.Mounts)
			},
		},
		{
			name: "handles nil mounts initially",
			config: &Config{
				WorkspaceMount:  "type=bind,source=/host/project,target=/workspace",
				WorkspaceFolder: "/workspace",
				Mounts:          nil,
			},
			paths: []types.Path{
				{
					Location: "testdata",
					Purpose:  "test",
					Required: false,
				},
			},
			wantErr: false,
			validate: func(t *testing.T, config *Config) {
				require.NotNil(t, config.Mounts)
				require.Len(t, config.Mounts, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := addCredentialMounts(tt.config, tt.paths)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, tt.config)
				}
			}
		})
	}
}
