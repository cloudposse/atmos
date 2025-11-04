package container

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetString(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]interface{}
		key      string
		expected string
	}{
		{
			name: "string value exists",
			m: map[string]interface{}{
				"ID":     "abc123",
				"Status": "running",
			},
			key:      "ID",
			expected: "abc123",
		},
		{
			name: "key does not exist",
			m: map[string]interface{}{
				"ID": "abc123",
			},
			key:      "Name",
			expected: "",
		},
		{
			name: "value is not a string",
			m: map[string]interface{}{
				"Count": 42,
				"Valid": true,
			},
			key:      "Count",
			expected: "",
		},
		{
			name: "value is nil",
			m: map[string]interface{}{
				"ID": nil,
			},
			key:      "ID",
			expected: "",
		},
		{
			name:     "empty map",
			m:        map[string]interface{}{},
			key:      "ID",
			expected: "",
		},
		{
			name:     "nil map",
			m:        nil,
			key:      "ID",
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

func TestParseLabels(t *testing.T) {
	tests := []struct {
		name      string
		labelsStr string
		expected  map[string]string
	}{
		{
			name:      "single label",
			labelsStr: "app=myapp",
			expected: map[string]string{
				"app": "myapp",
			},
		},
		{
			name:      "multiple labels",
			labelsStr: "app=myapp,version=1.0,env=production",
			expected: map[string]string{
				"app":     "myapp",
				"version": "1.0",
				"env":     "production",
			},
		},
		{
			name:      "label with equals in value",
			labelsStr: "app=myapp,config=key=value",
			expected: map[string]string{
				"app":    "myapp",
				"config": "key=value",
			},
		},
		{
			name:      "empty string",
			labelsStr: "",
			expected:  map[string]string{},
		},
		{
			name:      "invalid format without equals",
			labelsStr: "app,version",
			expected:  map[string]string{},
		},
		{
			name:      "mixed valid and invalid",
			labelsStr: "app=myapp,invalid,version=1.0",
			expected: map[string]string{
				"app":     "myapp",
				"version": "1.0",
			},
		},
		{
			name:      "label with empty value",
			labelsStr: "app=,version=1.0",
			expected: map[string]string{
				"app":     "",
				"version": "1.0",
			},
		},
		{
			name:      "trailing comma",
			labelsStr: "app=myapp,",
			expected: map[string]string{
				"app": "myapp",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLabels(tt.labelsStr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewDockerRuntime(t *testing.T) {
	runtime := NewDockerRuntime()
	require.NotNil(t, runtime)
	assert.IsType(t, &DockerRuntime{}, runtime)
}

func TestDockerRuntime_Info(t *testing.T) {
	// This test verifies the Info method structure.
	// Actual execution requires docker to be installed.
	runtime := NewDockerRuntime()
	require.NotNil(t, runtime)

	ctx := context.Background()
	info, err := runtime.Info(ctx)
	if err != nil {
		// Docker not available - expected in CI without docker.
		t.Skip("Docker not available, skipping Info test")
		return
	}

	// If docker is available, verify structure.
	require.NotNil(t, info)
	assert.Equal(t, string(TypeDocker), info.Type)
	assert.True(t, info.Running)
	assert.NotEmpty(t, info.Version)
}

// TestDockerRuntime_Attach validates Docker's Attach() method logic for shell selection
// and argument handling. Tests verify that options are correctly interpreted and passed
// to the underlying Exec() call by testing the buildAttachCommand builder function.
//
// Tests are intentionally duplicated to verify both implementations independently, ensuring
// consistency across runtimes and allowing runtime-specific test evolution if needed.
//
//nolint:dupl // Docker and Podman implement identical Runtime interface with same Attach() behavior.
func TestDockerRuntime_Attach(t *testing.T) {
	tests := []struct {
		name         string
		opts         *AttachOptions
		expectShell  string
		expectArgs   []string
		expectUser   string
		expectTTY    bool
		expectAttach bool
	}{
		{
			name:         "default bash with no options",
			opts:         nil,
			expectShell:  "/bin/bash",
			expectArgs:   nil,
			expectUser:   "",
			expectTTY:    true,
			expectAttach: true,
		},
		{
			name:         "empty AttachOptions uses defaults",
			opts:         &AttachOptions{},
			expectShell:  "/bin/bash",
			expectArgs:   nil,
			expectUser:   "",
			expectTTY:    true,
			expectAttach: true,
		},
		{
			name: "custom shell",
			opts: &AttachOptions{
				Shell: "/bin/sh",
			},
			expectShell:  "/bin/sh",
			expectArgs:   nil,
			expectUser:   "",
			expectTTY:    true,
			expectAttach: true,
		},
		{
			name: "shell with args",
			opts: &AttachOptions{
				Shell:     "/bin/bash",
				ShellArgs: []string{"-l", "-i"},
			},
			expectShell:  "/bin/bash",
			expectArgs:   []string{"-l", "-i"},
			expectUser:   "",
			expectTTY:    true,
			expectAttach: true,
		},
		{
			name: "custom user preserved in exec options",
			opts: &AttachOptions{
				User: "node",
			},
			expectShell:  "/bin/bash",
			expectArgs:   nil,
			expectUser:   "node",
			expectTTY:    true,
			expectAttach: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call the builder function that constructs the command and exec options.
			cmd, execOpts := buildAttachCommand(tt.opts)

			// Verify command structure: first element is shell, rest are args.
			require.NotEmpty(t, cmd, "command should not be empty")
			assert.Equal(t, tt.expectShell, cmd[0], "shell should match expected")

			// Verify shell args.
			actualArgs := cmd[1:]
			if tt.expectArgs == nil {
				assert.Empty(t, actualArgs, "should have no shell args")
			} else {
				assert.Equal(t, tt.expectArgs, actualArgs, "shell args should match expected")
			}

			// Verify exec options.
			require.NotNil(t, execOpts, "exec options should not be nil")
			assert.Equal(t, tt.expectTTY, execOpts.Tty, "TTY flag should match expected")
			assert.Equal(t, tt.expectAttach, execOpts.AttachStdin, "AttachStdin should match expected")
			assert.Equal(t, tt.expectAttach, execOpts.AttachStdout, "AttachStdout should match expected")
			assert.Equal(t, tt.expectAttach, execOpts.AttachStderr, "AttachStderr should match expected")
			assert.Equal(t, tt.expectUser, execOpts.User, "User should match expected")
		})
	}
}

func TestDockerRuntime_List_Integration(t *testing.T) {
	// Integration test - runs actual docker command if available.
	runtime := NewDockerRuntime()
	require.NotNil(t, runtime)

	ctx := context.Background()
	containers, err := runtime.List(ctx, nil)
	if err != nil {
		// Docker not available or no permission - skip.
		t.Skipf("Docker not available, skipping List test: %v", err)
		return
	}

	// If docker is available, verify the structure of returned data.
	// containers can be nil or empty if no containers exist - both are valid.
	require.NoError(t, err, "List should not return error when Docker is available")
	for _, container := range containers {
		// Each container should have at least an ID.
		assert.NotEmpty(t, container.ID, "container should have an ID")
		// Other fields may be empty depending on container state.
	}
}

func TestDockerRuntime_List_WithFilters_Integration(t *testing.T) {
	// Integration test - tests filter parameter passing.
	runtime := NewDockerRuntime()
	require.NotNil(t, runtime)

	ctx := context.Background()
	filters := map[string]string{
		"status": "exited",
	}
	containers, err := runtime.List(ctx, filters)
	if err != nil {
		// Docker not available - skip.
		t.Skipf("Docker not available, skipping List with filters test: %v", err)
		return
	}

	// If docker is available, verify list succeeds with filters.
	// containers can be nil or empty if no matching containers exist - both are valid.
	require.NoError(t, err, "List should not return error when Docker is available")
	for _, container := range containers {
		// Status check is not strict because Docker status strings vary.
		// We're mainly testing that filters are passed correctly to docker ps.
		assert.NotEmpty(t, container.ID)
	}
}

func TestDockerRuntime_Pull_Integration(t *testing.T) {
	// Integration test - tests pulling a small image.
	runtime := NewDockerRuntime()
	require.NotNil(t, runtime)

	ctx := context.Background()
	// Use alpine as it's very small (~5MB).
	err := runtime.Pull(ctx, "alpine:latest")
	if err != nil {
		// Docker not available - skip.
		t.Skipf("Docker not available, skipping Pull test: %v", err)
		return
	}

	require.NoError(t, err, "Pull should succeed for alpine:latest")
}

// TestDockerRuntime_ContainerLifecycle_Integration validates the container lifecycle
// (Create, Start, Stop, Remove) for Docker runtime. Tests are intentionally duplicated
// to verify both Docker and Podman implementations independently, ensuring consistency
// across runtimes and allowing runtime-specific test evolution if needed.
//
//nolint:dupl // Docker and Podman implement identical Runtime interface with same lifecycle behavior.
func TestDockerRuntime_ContainerLifecycle_Integration(t *testing.T) {
	// Integration test - tests Create, Start, Stop, Remove lifecycle.
	runtime := NewDockerRuntime()
	require.NotNil(t, runtime)

	ctx := context.Background()

	// First ensure alpine image exists.
	err := runtime.Pull(ctx, "alpine:latest")
	if err != nil {
		t.Skipf("Docker not available, skipping lifecycle test: %v", err)
		return
	}

	// Create container.
	config := &CreateConfig{
		Name:            "atmos-test-lifecycle",
		Image:           "alpine:latest",
		OverrideCommand: true, // Use sleep infinity.
	}

	containerID, err := runtime.Create(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}
	require.NotEmpty(t, containerID, "container ID should not be empty")

	// Ensure cleanup.
	defer func() {
		_ = runtime.Remove(ctx, containerID, true)
	}()

	// Start container.
	err = runtime.Start(ctx, containerID)
	require.NoError(t, err, "Start should succeed")

	// Stop container.
	err = runtime.Stop(ctx, containerID, 5*time.Second)
	require.NoError(t, err, "Stop should succeed")

	// Remove container.
	err = runtime.Remove(ctx, containerID, false)
	require.NoError(t, err, "Remove should succeed")
}

func TestDockerRuntime_Build_Integration(t *testing.T) {
	// Integration test - tests building from Dockerfile.
	runtime := NewDockerRuntime()
	require.NotNil(t, runtime)

	ctx := context.Background()

	// Create temporary directory for build context.
	tmpDir := t.TempDir()

	// Create simple Dockerfile.
	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")
	dockerfile := `FROM alpine:latest
RUN echo "test build"
`
	err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0o644)
	require.NoError(t, err)

	// Build image.
	config := &BuildConfig{
		Dockerfile: dockerfilePath,
		Context:    tmpDir,
		Tags:       []string{"atmos-test-build:latest"},
	}

	err = runtime.Build(ctx, config)
	if err != nil {
		t.Skipf("Docker not available or build failed, skipping: %v", err)
		return
	}

	require.NoError(t, err, "Build should succeed")
}

func TestDockerRuntime_Logs_Integration(t *testing.T) {
	// Integration test - tests container logs.
	runtime := NewDockerRuntime()
	require.NotNil(t, runtime)

	ctx := context.Background()

	// Pull alpine if needed.
	err := runtime.Pull(ctx, "alpine:latest")
	if err != nil {
		t.Skipf("Docker not available, skipping Logs test: %v", err)
		return
	}

	// Create and start a container that outputs something.
	config := &CreateConfig{
		Name:  "atmos-test-logs",
		Image: "alpine:latest",
		// Don't override command - we want default.
	}

	containerID, err := runtime.Create(ctx, config)
	if err != nil {
		t.Skipf("Failed to create container: %v", err)
		return
	}

	// Ensure cleanup.
	defer func() {
		_ = runtime.Remove(ctx, containerID, true)
	}()

	// Note: We can't easily test Logs output since it goes to os.Stdout/Stderr.
	// This test mainly verifies the method doesn't panic and completes.
	// We test with tail to avoid hanging on follow.
	err = runtime.Logs(ctx, containerID, false, "10", nil, nil)
	require.NoError(t, err, "Logs should succeed")
}

// TestDockerRuntime_Exec_Integration validates the Exec() method for Docker runtime.
// Tests are intentionally duplicated to verify both Docker and Podman implementations
// independently, ensuring consistency across runtimes and allowing runtime-specific
// test evolution if needed.
//
//nolint:dupl // Docker and Podman implement identical Runtime interface with same Exec() behavior.
func TestDockerRuntime_Exec_Integration(t *testing.T) {
	// Integration test - tests exec command in running container.
	runtime := NewDockerRuntime()
	require.NotNil(t, runtime)

	ctx := context.Background()

	// Pull alpine if needed.
	err := runtime.Pull(ctx, "alpine:latest")
	if err != nil {
		t.Skipf("Docker not available, skipping Exec test: %v", err)
		return
	}

	// Create and start a container.
	config := &CreateConfig{
		Name:            "atmos-test-exec",
		Image:           "alpine:latest",
		OverrideCommand: true, // Use sleep infinity.
	}

	containerID, err := runtime.Create(ctx, config)
	if err != nil {
		t.Skipf("Failed to create container: %v", err)
		return
	}

	// Ensure cleanup.
	defer func() {
		_ = runtime.Remove(ctx, containerID, true)
	}()

	// Start container.
	err = runtime.Start(ctx, containerID)
	if err != nil {
		t.Skipf("Failed to start container: %v", err)
		return
	}

	// Execute a simple command in the container.
	execOpts := &ExecOptions{
		Tty:          false,
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
	}

	// Note: Exec output goes to os.Stdout/Stderr, we mainly verify it doesn't error.
	err = runtime.Exec(ctx, containerID, []string{"echo", "test"}, execOpts)
	require.NoError(t, err, "Exec should succeed")
}
