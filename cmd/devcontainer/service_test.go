package devcontainer

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Mock providers for testing.

type mockConfigProvider struct {
	atmosConfig   *schema.AtmosConfiguration
	loadError     error
	devcontainers []string
	listError     error
}

func (m *mockConfigProvider) LoadAtmosConfig() (*schema.AtmosConfiguration, error) {
	return m.atmosConfig, m.loadError
}

func (m *mockConfigProvider) ListDevcontainers(config *schema.AtmosConfiguration) ([]string, error) {
	return m.devcontainers, m.listError
}

func (m *mockConfigProvider) GetDevcontainerConfig(
	config *schema.AtmosConfiguration,
	name string,
) (*DevcontainerConfig, error) {
	return &DevcontainerConfig{Name: name}, nil
}

type mockRuntimeProvider struct {
	startError   error
	attachError  error
	stopError    error
	execError    error
	logsError    error
	removeError  error
	rebuildError error
	listResult   []ContainerInfo
	listError    error
}

func (m *mockRuntimeProvider) Start(ctx context.Context, atmosConfig *schema.AtmosConfiguration, name string, opts StartOptions) error {
	return m.startError
}

func (m *mockRuntimeProvider) Attach(ctx context.Context, atmosConfig *schema.AtmosConfiguration, name string, opts AttachOptions) error {
	return m.attachError
}

func (m *mockRuntimeProvider) Stop(ctx context.Context, atmosConfig *schema.AtmosConfiguration, name string, opts StopOptions) error {
	return m.stopError
}

func (m *mockRuntimeProvider) Exec(ctx context.Context, atmosConfig *schema.AtmosConfiguration, name string, cmd []string, opts ExecOptions) error {
	return m.execError
}

func (m *mockRuntimeProvider) Logs(ctx context.Context, atmosConfig *schema.AtmosConfiguration, name string, opts LogsOptions) (io.ReadCloser, error) {
	if m.logsError != nil {
		return nil, m.logsError
	}
	// Return a valid empty reader, not nil.
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *mockRuntimeProvider) Remove(ctx context.Context, atmosConfig *schema.AtmosConfiguration, name string, opts RemoveOptions) error {
	return m.removeError
}

func (m *mockRuntimeProvider) Rebuild(ctx context.Context, atmosConfig *schema.AtmosConfiguration, name string, opts RebuildOptions) error {
	return m.rebuildError
}

func (m *mockRuntimeProvider) ListRunning(ctx context.Context) ([]ContainerInfo, error) {
	return m.listResult, m.listError
}

type mockUIProvider struct {
	interactive   bool
	promptResult  string
	promptError   error
	confirmResult bool
}

func (m *mockUIProvider) IsInteractive() bool {
	return m.interactive
}

func (m *mockUIProvider) Prompt(message string, options []string) (string, error) {
	return m.promptResult, m.promptError
}

func (m *mockUIProvider) Confirm(message string) (bool, error) {
	return m.confirmResult, nil
}

func (m *mockUIProvider) Output() io.Writer {
	return io.Discard
}

func (m *mockUIProvider) Error() io.Writer {
	return io.Discard
}

// Tests.

func TestService_Initialize(t *testing.T) {
	tests := []struct {
		name          string
		config        *mockConfigProvider
		expectedError bool
		errorSentinel error
	}{
		{
			name: "successful initialization",
			config: &mockConfigProvider{
				atmosConfig: &schema.AtmosConfiguration{},
			},
			expectedError: false,
		},
		{
			name: "config load error",
			config: &mockConfigProvider{
				loadError: errors.New("config not found"),
			},
			expectedError: true,
			errorSentinel: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewTestableService(tt.config, nil, nil)

			err := service.Initialize()

			if tt.expectedError {
				require.Error(t, err)
				if tt.errorSentinel != nil {
					assert.ErrorIs(t, err, tt.errorSentinel)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, service.atmosConfig)
			}
		})
	}
}

func TestService_ResolveDevcontainerName(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		config        *mockConfigProvider
		ui            *mockUIProvider
		expectedName  string
		expectedError bool
		errorSentinel error
	}{
		{
			name: "name in args",
			args: []string{"geodesic"},
			config: &mockConfigProvider{
				atmosConfig: &schema.AtmosConfiguration{},
			},
			ui:           &mockUIProvider{interactive: true},
			expectedName: "geodesic",
		},
		{
			name: "non-interactive no args",
			args: []string{},
			config: &mockConfigProvider{
				atmosConfig: &schema.AtmosConfiguration{},
			},
			ui:            &mockUIProvider{interactive: false},
			expectedError: true,
			errorSentinel: errUtils.ErrDevcontainerNameEmpty,
		},
		{
			name: "interactive prompt success",
			args: []string{},
			config: &mockConfigProvider{
				atmosConfig:   &schema.AtmosConfiguration{},
				devcontainers: []string{"geodesic", "terraform"},
			},
			ui: &mockUIProvider{
				interactive:  true,
				promptResult: "geodesic",
			},
			expectedName: "geodesic",
		},
		{
			name: "prompt fails",
			args: []string{},
			config: &mockConfigProvider{
				atmosConfig:   &schema.AtmosConfiguration{},
				devcontainers: []string{"geodesic"},
			},
			ui: &mockUIProvider{
				interactive: true,
				promptError: errors.New("user cancelled"),
			},
			expectedError: true,
		},
		{
			name: "no devcontainers configured",
			args: []string{},
			config: &mockConfigProvider{
				atmosConfig:   &schema.AtmosConfiguration{},
				devcontainers: []string{},
			},
			ui: &mockUIProvider{
				interactive: true,
			},
			expectedError: true,
			errorSentinel: errUtils.ErrDevcontainerNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewTestableService(tt.config, nil, tt.ui)
			service.atmosConfig = tt.config.atmosConfig

			name, err := service.ResolveDevcontainerName(context.Background(), tt.args)

			if tt.expectedError {
				require.Error(t, err)
				if tt.errorSentinel != nil {
					assert.ErrorIs(t, err, tt.errorSentinel)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedName, name)
			}
		})
	}
}

func TestService_Start(t *testing.T) {
	tests := []struct {
		name          string
		devName       string
		opts          StartOptions
		runtime       *mockRuntimeProvider
		expectedError bool
	}{
		{
			name:    "start without attach",
			devName: "geodesic",
			opts:    StartOptions{Attach: false},
			runtime: &mockRuntimeProvider{},
		},
		{
			name:    "start with attach",
			devName: "geodesic",
			opts:    StartOptions{Attach: true},
			runtime: &mockRuntimeProvider{},
		},
		{
			name:    "start fails",
			devName: "geodesic",
			opts:    StartOptions{},
			runtime: &mockRuntimeProvider{
				startError: errors.New("docker error"),
			},
			expectedError: true,
		},
		{
			name:    "attach after start fails",
			devName: "geodesic",
			opts:    StartOptions{Attach: true},
			runtime: &mockRuntimeProvider{
				attachError: errors.New("attach failed"),
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &mockConfigProvider{
				atmosConfig: &schema.AtmosConfiguration{},
			}
			service := NewTestableService(config, tt.runtime, nil)
			service.atmosConfig = config.atmosConfig

			err := service.Start(context.Background(), tt.devName, tt.opts)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestService_Stop(t *testing.T) {
	tests := []struct {
		name          string
		devName       string
		timeout       int
		runtime       *mockRuntimeProvider
		expectedError bool
	}{
		{
			name:    "stop success",
			devName: "geodesic",
			timeout: 10,
			runtime: &mockRuntimeProvider{},
		},
		{
			name:    "stop fails",
			devName: "geodesic",
			timeout: 10,
			runtime: &mockRuntimeProvider{
				stopError: errors.New("container not found"),
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewTestableService(nil, tt.runtime, nil)
			service.InitializeWithConfig(&schema.AtmosConfiguration{})

			err := service.Stop(context.Background(), tt.devName, StopOptions{Timeout: tt.timeout})

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestService_Attach(t *testing.T) {
	tests := []struct {
		name          string
		devName       string
		opts          AttachOptions
		runtime       *mockRuntimeProvider
		expectedError bool
	}{
		{
			name:    "attach success",
			devName: "geodesic",
			opts:    AttachOptions{Instance: "default"},
			runtime: &mockRuntimeProvider{},
		},
		{
			name:    "attach fails",
			devName: "geodesic",
			opts:    AttachOptions{},
			runtime: &mockRuntimeProvider{
				attachError: errors.New("container not running"),
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewTestableService(nil, tt.runtime, nil)
			service.InitializeWithConfig(&schema.AtmosConfiguration{})

			err := service.Attach(context.Background(), tt.devName, tt.opts)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestService_List(t *testing.T) {
	tests := []struct {
		name          string
		runtime       *mockRuntimeProvider
		expectedCount int
		expectedError bool
	}{
		{
			name: "list success",
			runtime: &mockRuntimeProvider{
				listResult: []ContainerInfo{
					{Name: "geodesic", Status: "running"},
					{Name: "terraform", Status: "running"},
				},
			},
			expectedCount: 2,
		},
		{
			name: "list empty",
			runtime: &mockRuntimeProvider{
				listResult: []ContainerInfo{},
			},
			expectedCount: 0,
		},
		{
			name: "list fails",
			runtime: &mockRuntimeProvider{
				listError: errors.New("docker daemon not running"),
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewTestableService(nil, tt.runtime, nil)

			containers, err := service.List(context.Background())

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, containers, tt.expectedCount)
			}
		})
	}
}

func TestService_Exec(t *testing.T) {
	tests := []struct {
		name          string
		devName       string
		cmd           []string
		opts          ExecOptions
		runtime       *mockRuntimeProvider
		expectedError bool
	}{
		{
			name:    "exec success",
			devName: "geodesic",
			cmd:     []string{"echo", "hello"},
			opts:    ExecOptions{},
			runtime: &mockRuntimeProvider{},
		},
		{
			name:    "exec fails",
			devName: "geodesic",
			cmd:     []string{"invalid"},
			opts:    ExecOptions{},
			runtime: &mockRuntimeProvider{
				execError: errors.New("command not found"),
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewTestableService(nil, tt.runtime, nil)
			service.InitializeWithConfig(&schema.AtmosConfiguration{})

			err := service.Exec(context.Background(), tt.devName, tt.cmd, tt.opts)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestService_Remove(t *testing.T) {
	tests := []struct {
		name          string
		devName       string
		opts          RemoveOptions
		runtime       *mockRuntimeProvider
		expectedError bool
	}{
		{
			name:    "remove success",
			devName: "geodesic",
			opts:    RemoveOptions{Force: false},
			runtime: &mockRuntimeProvider{},
		},
		{
			name:    "remove with force",
			devName: "geodesic",
			opts:    RemoveOptions{Force: true},
			runtime: &mockRuntimeProvider{},
		},
		{
			name:    "remove fails",
			devName: "geodesic",
			opts:    RemoveOptions{Force: false},
			runtime: &mockRuntimeProvider{
				removeError: errors.New("container in use"),
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewTestableService(nil, tt.runtime, nil)
			service.InitializeWithConfig(&schema.AtmosConfiguration{})

			err := service.Remove(context.Background(), tt.devName, tt.opts)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestService_Rebuild(t *testing.T) {
	tests := []struct {
		name          string
		devName       string
		opts          RebuildOptions
		runtime       *mockRuntimeProvider
		expectedError bool
	}{
		{
			name:    "rebuild success",
			devName: "geodesic",
			opts:    RebuildOptions{},
			runtime: &mockRuntimeProvider{},
		},
		{
			name:    "rebuild with no-pull",
			devName: "geodesic",
			opts:    RebuildOptions{NoPull: true},
			runtime: &mockRuntimeProvider{},
		},
		{
			name:    "rebuild fails",
			devName: "geodesic",
			opts:    RebuildOptions{},
			runtime: &mockRuntimeProvider{
				rebuildError: errors.New("build failed"),
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewTestableService(nil, tt.runtime, nil)
			service.InitializeWithConfig(&schema.AtmosConfiguration{})

			err := service.Rebuild(context.Background(), tt.devName, tt.opts)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestService_Logs(t *testing.T) {
	tests := []struct {
		name          string
		devName       string
		opts          LogsOptions
		runtime       *mockRuntimeProvider
		expectedError bool
	}{
		{
			name:    "logs success with nil reader",
			devName: "geodesic",
			opts:    LogsOptions{Follow: false, Tail: "100"},
			runtime: &mockRuntimeProvider{},
		},
		{
			name:    "logs with follow",
			devName: "geodesic",
			opts:    LogsOptions{Follow: true, Tail: "all"},
			runtime: &mockRuntimeProvider{},
		},
		{
			name:    "logs fails",
			devName: "geodesic",
			opts:    LogsOptions{},
			runtime: &mockRuntimeProvider{
				logsError: errors.New("container not found"),
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewTestableService(nil, tt.runtime, nil)
			// Initialize the service with a mock config.
			service.InitializeWithConfig(&schema.AtmosConfiguration{})

			err := service.Logs(context.Background(), tt.devName, tt.opts)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestService_NotInitialized(t *testing.T) {
	// Test that service methods fail gracefully when not initialized.
	t.Run("Attach fails when not initialized", func(t *testing.T) {
		service := NewTestableService(nil, &mockRuntimeProvider{}, nil)
		// Don't call Initialize or InitializeWithConfig.

		err := service.Attach(context.Background(), "test", AttachOptions{})

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAtmosConfigIsNil)
	})

	t.Run("Start fails when not initialized", func(t *testing.T) {
		service := NewTestableService(&mockConfigProvider{}, &mockRuntimeProvider{}, nil)

		err := service.Start(context.Background(), "test", StartOptions{})

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAtmosConfigIsNil)
	})

	t.Run("Stop fails when not initialized", func(t *testing.T) {
		service := NewTestableService(nil, &mockRuntimeProvider{}, nil)

		err := service.Stop(context.Background(), "test", StopOptions{})

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAtmosConfigIsNil)
	})

	t.Run("Exec fails when not initialized", func(t *testing.T) {
		service := NewTestableService(nil, &mockRuntimeProvider{}, nil)

		err := service.Exec(context.Background(), "test", []string{"echo"}, ExecOptions{})

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAtmosConfigIsNil)
	})

	t.Run("Logs fails when not initialized", func(t *testing.T) {
		service := NewTestableService(nil, &mockRuntimeProvider{}, nil)

		err := service.Logs(context.Background(), "test", LogsOptions{})

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAtmosConfigIsNil)
	})

	t.Run("Remove fails when not initialized", func(t *testing.T) {
		service := NewTestableService(nil, &mockRuntimeProvider{}, nil)

		err := service.Remove(context.Background(), "test", RemoveOptions{})

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAtmosConfigIsNil)
	})

	t.Run("Rebuild fails when not initialized", func(t *testing.T) {
		service := NewTestableService(nil, &mockRuntimeProvider{}, nil)

		err := service.Rebuild(context.Background(), "test", RebuildOptions{})

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAtmosConfigIsNil)
	})

	t.Run("List works without initialization", func(t *testing.T) {
		// List doesn't require atmosConfig as it queries runtime directly.
		service := NewTestableService(nil, &mockRuntimeProvider{listResult: []ContainerInfo{}}, nil)

		containers, err := service.List(context.Background())

		require.NoError(t, err)
		assert.Empty(t, containers)
	})
}

func TestService_LogsWithCustomOutput(t *testing.T) {
	// Test that logs can be streamed to a custom writer.
	t.Run("logs stream to custom writer", func(t *testing.T) {
		runtime := &mockRuntimeProvider{}

		// Create a buffer to capture output.
		var buf strings.Builder
		service := NewTestableServiceWithLogOutput(nil, runtime, nil, &buf)
		service.InitializeWithConfig(&schema.AtmosConfiguration{})

		// The mock returns an empty reader, so we just verify
		// the method completes without error.
		err := service.Logs(context.Background(), "geodesic", LogsOptions{})

		require.NoError(t, err)
		// The empty reader produces no output, but the method should complete.
	})
}
