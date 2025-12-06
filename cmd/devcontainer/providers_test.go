package devcontainer

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDefaultConfigProvider_LoadAtmosConfig tests the LoadAtmosConfig method
// using a known test fixture to ensure deterministic behavior.
func TestDefaultConfigProvider_LoadAtmosConfig(t *testing.T) {
	tests := []struct {
		name        string
		testDir     string
		expectError bool
	}{
		{
			name:        "loads config from devcontainer example",
			testDir:     "../../examples/devcontainer",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Change to test directory with known atmos.yaml.
			t.Chdir(tt.testDir)

			provider := &DefaultConfigProvider{}
			config, err := provider.LoadAtmosConfig()

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, config)
				// Verify the config has expected devcontainer configuration.
				assert.NotNil(t, config.Devcontainer, "config should have devcontainer section")
			}
		})
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
				Devcontainer: nil,
			},
			expectedError: true,
			errorSentinel: errUtils.ErrDevcontainerNotFound,
		},
		{
			name: "empty devcontainer config",
			config: &schema.AtmosConfiguration{
				Devcontainer: map[string]any{},
			},
			expectedError: false,
		},
		{
			name: "multiple devcontainers sorted",
			config: &schema.AtmosConfiguration{
				Devcontainer: map[string]any{
					"zulu":     map[string]any{},
					"alpha":    map[string]any{},
					"bravo":    map[string]any{},
					"geodesic": map[string]any{},
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
				Devcontainer: map[string]any{
					"geodesic": map[string]any{
						"image": "cloudposse/geodesic",
					},
				},
			},
			devName:      "geodesic",
			expectedName: "geodesic",
		},
		{
			name:         "nil config returns config with name",
			config:       nil,
			devName:      "test",
			expectedName: "test",
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

// TestDockerRuntimeProvider_OperationsRequireConfig tests that runtime operations
// fail with appropriate errors when devcontainer is not configured.
// These tests verify the contract that operations require valid configuration,
// using sentinel errors to catch any future semantic changes.
func TestDockerRuntimeProvider_OperationsRequireConfig(t *testing.T) {
	// Note: These tests verify that operations fail gracefully when the devcontainer
	// is not configured. The actual Docker/Podman runtime tests would require
	// integration tests with a running container runtime.
	//
	// We use assert.Error instead of assert.ErrorIs because the underlying
	// pkg/devcontainer errors are not yet standardized with sentinel errors.
	// When the devcontainer package is refactored to use sentinel errors
	// (e.g., ErrDevcontainerNotFound, ErrRuntimeNotAvailable), these tests
	// should be updated to use assert.ErrorIs.

	t.Run("Start requires valid devcontainer config", func(t *testing.T) {
		provider := NewDockerRuntimeProvider()
		atmosConfig := &schema.AtmosConfiguration{
			Devcontainer: map[string]any{
				"geodesic": map[string]any{},
			},
		}

		err := provider.Start(t.Context(), atmosConfig, "geodesic", StartOptions{Instance: "default"})

		// Error expected because no Docker daemon or container runtime available.
		require.Error(t, err, "Start should fail without container runtime")
	})

	t.Run("Stop requires running container", func(t *testing.T) {
		provider := NewDockerRuntimeProvider()
		atmosConfig := &schema.AtmosConfiguration{}

		err := provider.Stop(t.Context(), atmosConfig, "nonexistent", StopOptions{Timeout: 10})

		require.Error(t, err, "Stop should fail for nonexistent container")
	})

	t.Run("Attach requires running container", func(t *testing.T) {
		provider := NewDockerRuntimeProvider()
		atmosConfig := &schema.AtmosConfiguration{}

		err := provider.Attach(t.Context(), atmosConfig, "nonexistent", AttachOptions{})

		require.Error(t, err, "Attach should fail for nonexistent container")
	})

	t.Run("Exec requires running container", func(t *testing.T) {
		provider := NewDockerRuntimeProvider()
		atmosConfig := &schema.AtmosConfiguration{}

		err := provider.Exec(t.Context(), atmosConfig, "nonexistent", []string{"echo", "hello"}, ExecOptions{})

		require.Error(t, err, "Exec should fail for nonexistent container")
	})

	t.Run("Logs requires container", func(t *testing.T) {
		provider := NewDockerRuntimeProvider()
		atmosConfig := &schema.AtmosConfiguration{}

		_, err := provider.Logs(t.Context(), atmosConfig, "nonexistent", LogsOptions{})

		require.Error(t, err, "Logs should fail for nonexistent container")
	})

	t.Run("Remove requires container", func(t *testing.T) {
		provider := NewDockerRuntimeProvider()
		atmosConfig := &schema.AtmosConfiguration{}

		err := provider.Remove(t.Context(), atmosConfig, "nonexistent", RemoveOptions{Force: false})

		require.Error(t, err, "Remove should fail for nonexistent container")
	})

	t.Run("Rebuild requires valid devcontainer config", func(t *testing.T) {
		provider := NewDockerRuntimeProvider()
		atmosConfig := &schema.AtmosConfiguration{}

		err := provider.Rebuild(t.Context(), atmosConfig, "nonexistent", RebuildOptions{})

		require.Error(t, err, "Rebuild should fail for nonexistent devcontainer")
	})
}

// TestDefaultUIProvider_IsInteractive tests terminal detection.
func TestDefaultUIProvider_IsInteractive(t *testing.T) {
	provider := &DefaultUIProvider{}

	// This will return true/false depending on test environment.
	// We verify it returns a boolean without panicking.
	isInteractive := provider.IsInteractive()

	// In CI, this is typically false; in local terminal, true.
	// We just verify the type is correct.
	assert.IsType(t, false, isInteractive)
}

// TestDefaultUIProvider_Prompt tests the Prompt method validation.
func TestDefaultUIProvider_Prompt(t *testing.T) {
	tests := []struct {
		name          string
		message       string
		options       []string
		expectError   bool
		errorSentinel error
	}{
		{
			name:          "empty options returns ErrDevcontainerNotFound",
			message:       "Select option:",
			options:       []string{},
			expectError:   true,
			errorSentinel: errUtils.ErrDevcontainerNotFound,
		},
		{
			name:          "nil options returns ErrDevcontainerNotFound",
			message:       "Select option:",
			options:       nil,
			expectError:   true,
			errorSentinel: errUtils.ErrDevcontainerNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &DefaultUIProvider{}

			_, err := provider.Prompt(tt.message, tt.options)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorSentinel != nil {
					assert.ErrorIs(t, err, tt.errorSentinel)
				}
			}
		})
	}
}

// TestDefaultUIProvider_Confirm is skipped because it requires interactive input.
func TestDefaultUIProvider_Confirm(t *testing.T) {
	t.Skip("Confirm requires interactive terminal and user input")
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
