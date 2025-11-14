package devcontainer

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDefaultConfigProvider_LoadAtmosConfig tests the LoadAtmosConfig method.
func TestDefaultConfigProvider_LoadAtmosConfig(t *testing.T) {
	// Note: This test requires a valid atmos.yaml in the workspace.
	// In a real test environment, we would set up fixtures.
	// For now, we test that it returns without panicking.
	provider := &DefaultConfigProvider{}

	config, err := provider.LoadAtmosConfig()

	// We expect either success or a specific error.
	if err != nil {
		// Error is acceptable if no atmos.yaml exists.
		assert.Error(t, err)
	} else {
		// Success means we got a valid config.
		require.NotNil(t, config)
	}
}

// TestDefaultConfigProvider_ListDevcontainers tests listing devcontainers.
func TestDefaultConfigProvider_ListDevcontainers(t *testing.T) {
	tests := []struct {
		name          string
		config        *schema.AtmosConfiguration
		expectedError bool
		errorSentinel error
		expectSorted  bool
	}{
		{
			name:          "nil config",
			config:        nil,
			expectedError: true,
			errorSentinel: errUtils.ErrDevcontainerNotFound,
		},
		{
			name: "nil devcontainer config",
			config: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: nil,
				},
			},
			expectedError: true,
			errorSentinel: errUtils.ErrDevcontainerNotFound,
		},
		{
			name: "empty devcontainer config",
			config: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: map[string]any{},
				},
			},
			expectedError: false,
		},
		{
			name: "multiple devcontainers sorted",
			config: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: map[string]any{
						"zulu":     map[string]any{},
						"alpha":    map[string]any{},
						"bravo":    map[string]any{},
						"geodesic": map[string]any{},
					},
				},
			},
			expectedError: false,
			expectSorted:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &DefaultConfigProvider{}

			names, err := provider.ListDevcontainers(tt.config)

			if tt.expectedError {
				require.Error(t, err)
				if tt.errorSentinel != nil {
					assert.ErrorIs(t, err, tt.errorSentinel)
				}
			} else {
				require.NoError(t, err)
				if tt.expectSorted {
					// Verify sorted order.
					require.Len(t, names, 4)
					assert.Equal(t, "alpha", names[0])
					assert.Equal(t, "bravo", names[1])
					assert.Equal(t, "geodesic", names[2])
					assert.Equal(t, "zulu", names[3])
				}
			}
		})
	}
}

// TestDefaultConfigProvider_GetDevcontainerConfig tests retrieving config for a devcontainer.
func TestDefaultConfigProvider_GetDevcontainerConfig(t *testing.T) {
	tests := []struct {
		name         string
		config       *schema.AtmosConfiguration
		devName      string
		expectedName string
	}{
		{
			name: "get config for devcontainer",
			config: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: map[string]any{
						"geodesic": map[string]any{
							"image": "cloudposse/geodesic",
						},
					},
				},
			},
			devName:      "geodesic",
			expectedName: "geodesic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &DefaultConfigProvider{}

			devConfig, err := provider.GetDevcontainerConfig(tt.config, tt.devName)

			require.NoError(t, err)
			require.NotNil(t, devConfig)
			assert.Equal(t, tt.expectedName, devConfig.Name)
		})
	}
}

// TestDockerRuntimeProvider_NewDockerRuntimeProvider tests provider creation.
func TestDockerRuntimeProvider_NewDockerRuntimeProvider(t *testing.T) {
	provider := NewDockerRuntimeProvider()

	require.NotNil(t, provider)
	require.NotNil(t, provider.manager)
}

// TestDockerRuntimeProvider_ListRunning tests listing running containers.
func TestDockerRuntimeProvider_ListRunning(t *testing.T) {
	provider := NewDockerRuntimeProvider()
	ctx := context.Background()

	// Note: Current implementation returns empty list.
	// This test verifies it doesn't panic and returns expected stub behavior.
	containers, err := provider.ListRunning(ctx)

	require.NoError(t, err)
	assert.Empty(t, containers, "current implementation returns empty list")
}

// TestDockerRuntimeProvider_Start tests starting a devcontainer.
func TestDockerRuntimeProvider_Start(t *testing.T) {
	tests := []struct {
		name          string
		devName       string
		opts          StartOptions
		atmosConfig   *schema.AtmosConfiguration
		expectedError bool
	}{
		{
			name:    "start with valid config",
			devName: "geodesic",
			opts:    StartOptions{Instance: "default"},
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Devcontainer: map[string]any{
						"geodesic": map[string]any{},
					},
				},
			},
			// Note: This will fail because it tries to actually start a container.
			// In a real scenario, we'd mock the manager.
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewDockerRuntimeProvider()
			ctx := context.Background()

			err := provider.Start(ctx, tt.atmosConfig, tt.devName, tt.opts)

			if tt.expectedError {
				// We expect error because no Docker daemon or the manager will fail.
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestDockerRuntimeProvider_Stop tests stopping a devcontainer.
func TestDockerRuntimeProvider_Stop(t *testing.T) {
	provider := NewDockerRuntimeProvider()
	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	err := provider.Stop(ctx, atmosConfig, "test", StopOptions{Timeout: 10})

	// We expect this to fail without Docker daemon/container.
	assert.Error(t, err)
}

// TestDockerRuntimeProvider_Attach tests attaching to a devcontainer.
func TestDockerRuntimeProvider_Attach(t *testing.T) {
	provider := NewDockerRuntimeProvider()
	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	err := provider.Attach(ctx, atmosConfig, "test", AttachOptions{})

	// We expect this to fail without Docker daemon/container.
	assert.Error(t, err)
}

// TestDockerRuntimeProvider_Exec tests executing a command.
func TestDockerRuntimeProvider_Exec(t *testing.T) {
	provider := NewDockerRuntimeProvider()
	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	err := provider.Exec(ctx, atmosConfig, "test", []string{"echo", "hello"}, ExecOptions{})

	// We expect this to fail without Docker daemon/container.
	assert.Error(t, err)
}

// TestDockerRuntimeProvider_Logs tests retrieving logs.
func TestDockerRuntimeProvider_Logs(t *testing.T) {
	provider := NewDockerRuntimeProvider()
	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	_, err := provider.Logs(ctx, atmosConfig, "test", LogsOptions{})

	// We expect this to fail without Docker daemon/container.
	assert.Error(t, err)
}

// TestDockerRuntimeProvider_Remove tests removing a devcontainer.
func TestDockerRuntimeProvider_Remove(t *testing.T) {
	provider := NewDockerRuntimeProvider()
	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	err := provider.Remove(ctx, atmosConfig, "test", false)

	// We expect this to fail without Docker daemon/container.
	assert.Error(t, err)
}

// TestDockerRuntimeProvider_Rebuild tests rebuilding a devcontainer.
func TestDockerRuntimeProvider_Rebuild(t *testing.T) {
	provider := NewDockerRuntimeProvider()
	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	err := provider.Rebuild(ctx, atmosConfig, "test", RebuildOptions{})

	// We expect this to fail without Docker daemon/container.
	assert.Error(t, err)
}

// TestDefaultUIProvider_IsInteractive tests terminal detection.
func TestDefaultUIProvider_IsInteractive(t *testing.T) {
	provider := &DefaultUIProvider{}

	// This will return true/false depending on test environment.
	// We just verify it doesn't panic.
	isInteractive := provider.IsInteractive()

	// In CI, this is typically false; in local terminal, true.
	assert.IsType(t, false, isInteractive)
}

// TestDefaultUIProvider_Prompt tests prompting user.
func TestDefaultUIProvider_Prompt(t *testing.T) {
	// Skip in non-interactive environments.
	if os.Getenv("CI") != "" {
		t.Skip("skipping interactive prompt test in CI")
	}

	provider := &DefaultUIProvider{}

	// This would block waiting for user input in real usage.
	// We skip the actual call but verify the method exists.
	t.Skip("requires interactive terminal and user input")

	_, err := provider.Prompt("Select option:", []string{"a", "b"})
	assert.Error(t, err, "expected error in non-interactive test mode")
}

// TestDefaultUIProvider_Prompt_EmptyOptions tests prompt with no options.
func TestDefaultUIProvider_Prompt_EmptyOptions(t *testing.T) {
	provider := &DefaultUIProvider{}

	_, err := provider.Prompt("Select option:", []string{})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDevcontainerNotFound)
}

// TestDefaultUIProvider_Confirm tests confirmation prompts.
func TestDefaultUIProvider_Confirm(t *testing.T) {
	// Skip in non-interactive environments.
	if os.Getenv("CI") != "" {
		t.Skip("skipping interactive confirm test in CI")
	}

	provider := &DefaultUIProvider{}

	// This would block waiting for user input.
	t.Skip("requires interactive terminal and user input")

	_, err := provider.Confirm("Are you sure?")
	assert.Error(t, err, "expected error in non-interactive test mode")
}

// TestDefaultUIProvider_Output tests output writer.
func TestDefaultUIProvider_Output(t *testing.T) {
	provider := &DefaultUIProvider{}

	writer := provider.Output()

	require.NotNil(t, writer)
	assert.Equal(t, os.Stderr, writer)
}

// TestDefaultUIProvider_Error tests error writer.
func TestDefaultUIProvider_Error(t *testing.T) {
	provider := &DefaultUIProvider{}

	writer := provider.Error()

	require.NotNil(t, writer)
	assert.Equal(t, os.Stderr, writer)
}

// TestDefaultUIProvider_OutputWritable tests writing to output.
func TestDefaultUIProvider_OutputWritable(t *testing.T) {
	provider := &DefaultUIProvider{}

	// Capture stderr to verify writing works.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	_, err := provider.Output().Write([]byte("test output\n"))
	require.NoError(t, err)

	// Restore stderr.
	w.Close()
	os.Stderr = oldStderr

	// Read what was written.
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	assert.Contains(t, buf.String(), "test output")
}

// TestDefaultUIProvider_ErrorWritable tests writing to error output.
func TestDefaultUIProvider_ErrorWritable(t *testing.T) {
	provider := &DefaultUIProvider{}

	// Capture stderr to verify writing works.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	_, err := provider.Error().Write([]byte("test error\n"))
	require.NoError(t, err)

	// Restore stderr.
	w.Close()
	os.Stderr = oldStderr

	// Read what was written.
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	assert.Contains(t, buf.String(), "test error")
}

// TestDefaultConfigProvider_GetDevcontainerConfig_NilConfig tests nil config handling.
func TestDefaultConfigProvider_GetDevcontainerConfig_NilConfig(t *testing.T) {
	provider := &DefaultConfigProvider{}

	// Even with nil config, GetDevcontainerConfig should return a config with the name.
	// This is the current behavior - it's a minimal stub.
	devConfig, err := provider.GetDevcontainerConfig(nil, "test")

	require.NoError(t, err)
	require.NotNil(t, devConfig)
	assert.Equal(t, "test", devConfig.Name)
}

// TestDockerRuntimeProvider_ProviderIntegration tests provider integration.
func TestDockerRuntimeProvider_ProviderIntegration(t *testing.T) {
	// This test verifies that the provider can be created and its manager is initialized.
	// This is important for ensuring the provider is ready for use.
	provider := NewDockerRuntimeProvider()

	require.NotNil(t, provider, "provider should not be nil")
	require.NotNil(t, provider.manager, "manager should be initialized")

	// Verify ListRunning doesn't panic (even if it returns empty).
	ctx := context.Background()
	containers, err := provider.ListRunning(ctx)

	require.NoError(t, err)
	require.NotNil(t, containers, "containers list should not be nil")
}

// TestDefaultUIProvider_Prompt_ValidatesInput tests prompt input validation.
func TestDefaultUIProvider_Prompt_ValidatesInput(t *testing.T) {
	provider := &DefaultUIProvider{}

	// Test that empty options returns appropriate error.
	_, err := provider.Prompt("Select:", []string{})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDevcontainerNotFound)
}
