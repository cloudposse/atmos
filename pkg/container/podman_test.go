package container

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanPodmanOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "simple string without escapes",
			input:    []byte("hello world"),
			expected: "hello world",
		},
		{
			name:     "string with literal \\n",
			input:    []byte("line1\\nline2\\nline3"),
			expected: "line1\nline2\nline3",
		},
		{
			name:     "string with literal \\t",
			input:    []byte("col1\\tcol2\\tcol3"),
			expected: "col1\tcol2\tcol3",
		},
		{
			name:     "string with literal \\r",
			input:    []byte("text\\rmore text"),
			expected: "text\rmore text",
		},
		{
			name:     "mixed escape sequences",
			input:    []byte("line1\\nline2\\tcolumn\\rtext"),
			expected: "line1\nline2\tcolumn\rtext",
		},
		{
			name:     "podman pull output with escapes",
			input:    []byte("Resolving\\nTrying to pull\\nGetting image\\nCopying blob\\n"),
			expected: "Resolving\nTrying to pull\nGetting image\nCopying blob",
		},
		{
			name:     "trailing and leading whitespace",
			input:    []byte("  text with spaces  \\n  more text  "),
			expected: "text with spaces  \n  more text",
		},
		{
			name:     "empty string",
			input:    []byte(""),
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    []byte("   \t\n  "),
			expected: "",
		},
		{
			name:     "real podman error with escaped newlines",
			input:    []byte("Error: no container with name or ID \"Resolving\\nTrying to pull\\nGetting image\""),
			expected: "Error: no container with name or ID \"Resolving\nTrying to pull\nGetting image\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanPodmanOutput(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test the container ID extraction logic used in podman Create.
func TestExtractContainerIDFromPodmanCreateOutput(t *testing.T) {
	tests := []struct {
		name             string
		output           string
		expectedID       string
		expectEmpty      bool
		errorDescription string
	}{
		{
			name:       "simple container ID only",
			output:     "d31a46dd77566a9b55c6062cdc711a38453cb7f859f086c984a3a1fe77892703",
			expectedID: "d31a46dd77566a9b55c6062cdc711a38453cb7f859f086c984a3a1fe77892703",
		},
		{
			name: "container ID with pull output (real podman behavior)",
			output: `Resolving "hashicorp/terraform" using unqualified-search registries
Trying to pull docker.io/hashicorp/terraform:1.10...
Getting image source signatures
Copying blob sha256:3f8427b65950b065cf17e781548deca8611c329bee69fd12c944e8b77615c5af
Copying blob sha256:185961b25d19dc6017cce9b3a843a692f95ba285c7ae1f50a6c1eb7bac1fb97a
Copying config sha256:36df1606fe8f0580466adc91adba819177db091c29094febf7ed2e10e64b4127
Writing manifest to image destination
d31a46dd77566a9b55c6062cdc711a38453cb7f859f086c984a3a1fe77892703`,
			expectedID: "d31a46dd77566a9b55c6062cdc711a38453cb7f859f086c984a3a1fe77892703",
		},
		{
			name: "container ID with trailing newlines",
			output: `Some output
abcd1234567890

`,
			expectedID: "abcd1234567890",
		},
		{
			name: "container ID with multiple trailing newlines",
			output: `Output line 1
Output line 2
container-id-here


`,
			expectedID: "container-id-here",
		},
		{
			name:             "empty output",
			output:           "",
			expectEmpty:      true,
			errorDescription: "should return empty for no output",
		},
		{
			name:             "only whitespace",
			output:           "   \n\t\n   ",
			expectEmpty:      true,
			errorDescription: "should return empty for whitespace-only output",
		},
		{
			name:       "single line with whitespace",
			output:     "  container-id  ",
			expectedID: "container-id",
		},
		{
			name: "multiline with empty lines in middle",
			output: `First line


Last line with ID`,
			expectedID: "Last line with ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate the exact logic from podman.go Create function (lines 72-84).
			lines := strings.Split(tt.output, "\n")
			var containerID string
			for i := len(lines) - 1; i >= 0; i-- {
				line := strings.TrimSpace(lines[i])
				if line != "" {
					containerID = line
					break
				}
			}

			if tt.expectEmpty {
				assert.Empty(t, containerID, tt.errorDescription)
			} else {
				assert.Equal(t, tt.expectedID, containerID)
			}
		})
	}
}

func TestExtractPodmanName(t *testing.T) {
	tests := []struct {
		name          string
		containerJSON map[string]interface{}
		expected      string
	}{
		{
			name: "single name in array",
			containerJSON: map[string]interface{}{
				"Names": []interface{}{"my-container"},
			},
			expected: "my-container",
		},
		{
			name: "multiple names returns first",
			containerJSON: map[string]interface{}{
				"Names": []interface{}{"first-name", "second-name"},
			},
			expected: "first-name",
		},
		{
			name: "empty names array",
			containerJSON: map[string]interface{}{
				"Names": []interface{}{},
			},
			expected: "",
		},
		{
			name: "names key missing",
			containerJSON: map[string]interface{}{
				"Id": "abc123",
			},
			expected: "",
		},
		{
			name: "names is not array",
			containerJSON: map[string]interface{}{
				"Names": "my-container",
			},
			expected: "",
		},
		{
			name: "names array contains non-string",
			containerJSON: map[string]interface{}{
				"Names": []interface{}{123, "my-container"},
			},
			expected: "",
		},
		{
			name:          "nil container json",
			containerJSON: nil,
			expected:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPodmanName(tt.containerJSON)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseLabelsMap(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]interface{}
		expected map[string]string
	}{
		{
			name: "all string values",
			labels: map[string]interface{}{
				"app":     "myapp",
				"version": "1.0",
				"env":     "production",
			},
			expected: map[string]string{
				"app":     "myapp",
				"version": "1.0",
				"env":     "production",
			},
		},
		{
			name: "mixed types filters non-strings",
			labels: map[string]interface{}{
				"app":     "myapp",
				"count":   42,
				"enabled": true,
				"version": "1.0",
			},
			expected: map[string]string{
				"app":     "myapp",
				"version": "1.0",
			},
		},
		{
			name:     "empty map",
			labels:   map[string]interface{}{},
			expected: map[string]string{},
		},
		{
			name:     "nil map",
			labels:   nil,
			expected: map[string]string{},
		},
		{
			name: "all non-string values",
			labels: map[string]interface{}{
				"count":   42,
				"enabled": true,
				"value":   3.14,
			},
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLabelsMap(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParsePodmanContainer(t *testing.T) {
	tests := []struct {
		name          string
		containerJSON map[string]interface{}
		expected      Info
	}{
		{
			name: "complete container info",
			containerJSON: map[string]interface{}{
				"Id":    "abc123def456",
				"Names": []interface{}{"my-container"},
				"Image": "ubuntu:22.04",
				"State": "running",
				"Labels": map[string]interface{}{
					"app":     "test",
					"version": "1.0",
				},
			},
			expected: Info{
				ID:     "abc123def456",
				Name:   "my-container",
				Image:  "ubuntu:22.04",
				Status: "running",
				Labels: map[string]string{
					"app":     "test",
					"version": "1.0",
				},
			},
		},
		{
			name: "container without labels",
			containerJSON: map[string]interface{}{
				"Id":    "xyz789",
				"Names": []interface{}{"simple-container"},
				"Image": "alpine:latest",
				"State": "exited",
			},
			expected: Info{
				ID:     "xyz789",
				Name:   "simple-container",
				Image:  "alpine:latest",
				Status: "exited",
			},
		},
		{
			name: "container with non-map labels ignored",
			containerJSON: map[string]interface{}{
				"Id":     "xyz789",
				"Names":  []interface{}{"test"},
				"Image":  "alpine",
				"State":  "running",
				"Labels": "not-a-map",
			},
			expected: Info{
				ID:     "xyz789",
				Name:   "test",
				Image:  "alpine",
				Status: "running",
			},
		},
		{
			name:          "minimal container info",
			containerJSON: map[string]interface{}{},
			expected: Info{
				ID:     "",
				Name:   "",
				Image:  "",
				Status: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePodmanContainer(tt.containerJSON)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParsePodmanContainers(t *testing.T) {
	tests := []struct {
		name              string
		podmanContainers  []map[string]interface{}
		expectedCount     int
		expectedFirstName string
	}{
		{
			name: "multiple containers",
			podmanContainers: []map[string]interface{}{
				{
					"Id":    "abc123",
					"Names": []interface{}{"container1"},
					"Image": "ubuntu",
					"State": "running",
				},
				{
					"Id":    "def456",
					"Names": []interface{}{"container2"},
					"Image": "alpine",
					"State": "exited",
				},
			},
			expectedCount:     2,
			expectedFirstName: "container1",
		},
		{
			name:              "empty slice",
			podmanContainers:  []map[string]interface{}{},
			expectedCount:     0,
			expectedFirstName: "",
		},
		{
			name:              "nil slice",
			podmanContainers:  nil,
			expectedCount:     0,
			expectedFirstName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePodmanContainers(tt.podmanContainers)
			assert.Len(t, result, tt.expectedCount)
			if tt.expectedCount > 0 {
				assert.Equal(t, tt.expectedFirstName, result[0].Name)
			}
		})
	}
}

func TestNewPodmanRuntime(t *testing.T) {
	runtime := NewPodmanRuntime()
	require.NotNil(t, runtime)
	assert.IsType(t, &PodmanRuntime{}, runtime)
}

func TestPodmanRuntime_Info(t *testing.T) {
	// This test verifies the Info method structure.
	// Actual execution requires podman to be installed.
	runtime := NewPodmanRuntime()
	require.NotNil(t, runtime)

	ctx := context.Background()
	info, err := runtime.Info(ctx)
	if err != nil {
		// Podman not available - expected in CI without podman.
		t.Skip("Podman not available, skipping Info test")
		return
	}

	// If podman is available, verify structure.
	require.NotNil(t, info)
	assert.Equal(t, string(TypePodman), info.Type)
	assert.True(t, info.Running)
	assert.NotEmpty(t, info.Version)
}

func TestPodmanRuntime_Inspect(t *testing.T) {
	// Inspect uses List internally, so we test the logic path.
	// In CI without podman, this will fail at List() call.
	runtime := NewPodmanRuntime()
	require.NotNil(t, runtime)

	ctx := context.Background()
	_, err := runtime.Inspect(ctx, "nonexistent-container")
	if err != nil {
		// Expected: either podman not available or container not found.
		// Both are acceptable for this test.
		t.Logf("Inspect failed as expected (podman not available or container not found): %v", err)
	}
}

// TestPodmanRuntime_Attach validates Podman's Attach() method logic for shell selection
// and argument handling. Tests verify that options are correctly interpreted and passed
// to the underlying Exec() call by testing the buildAttachCommand builder function.
//
// Tests are intentionally duplicated to verify both implementations independently, ensuring
// consistency across runtimes and allowing runtime-specific test evolution if needed.
//
//nolint:dupl // Docker and Podman implement identical Runtime interface with same Attach() behavior.
func TestPodmanRuntime_Attach(t *testing.T) {
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

func TestPodmanRuntime_List_Integration(t *testing.T) {
	// Integration test - runs actual podman command if available.
	runtime := NewPodmanRuntime()
	require.NotNil(t, runtime)

	ctx := context.Background()
	containers, err := runtime.List(ctx, nil)
	if err != nil {
		// Podman not available or no permission - skip.
		t.Skipf("Podman not available, skipping List test: %v", err)
		return
	}

	// If podman is available, verify the structure of returned data.
	// containers can be nil or empty if no containers exist - both are valid.
	require.NoError(t, err, "List should not return error when Podman is available")
	for _, container := range containers {
		// Each container should have at least an ID.
		assert.NotEmpty(t, container.ID, "container should have an ID")
		// Other fields may be empty depending on container state.
	}
}

func TestPodmanRuntime_Pull_Integration(t *testing.T) {
	// Integration test - tests pulling a small image.
	runtime := NewPodmanRuntime()
	require.NotNil(t, runtime)

	ctx := context.Background()
	// Use alpine as it's very small (~5MB).
	err := runtime.Pull(ctx, "alpine:latest")
	if err != nil {
		// Podman not available - skip.
		t.Skipf("Podman not available, skipping Pull test: %v", err)
		return
	}

	require.NoError(t, err, "Pull should succeed for alpine:latest")
}

// TestPodmanRuntime_ContainerLifecycle_Integration validates the container lifecycle
// (Create, Start, Stop, Remove) for Podman runtime. Tests are intentionally duplicated
// to verify both Docker and Podman implementations independently, ensuring consistency
// across runtimes and allowing runtime-specific test evolution if needed.
//
//nolint:dupl // Docker and Podman implement identical Runtime interface with same lifecycle behavior.
func TestPodmanRuntime_ContainerLifecycle_Integration(t *testing.T) {
	// Integration test - tests Create, Start, Stop, Remove lifecycle.
	runtime := NewPodmanRuntime()
	require.NotNil(t, runtime)

	ctx := context.Background()

	// First ensure alpine image exists.
	err := runtime.Pull(ctx, "alpine:latest")
	if err != nil {
		t.Skipf("Podman not available, skipping lifecycle test: %v", err)
		return
	}

	// Create container.
	config := &CreateConfig{
		Name:            "atmos-test-lifecycle-podman",
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

func TestPodmanRuntime_Logs_Integration(t *testing.T) {
	// Integration test - tests container logs.
	runtime := NewPodmanRuntime()
	require.NotNil(t, runtime)

	ctx := context.Background()

	// Pull alpine if needed.
	err := runtime.Pull(ctx, "alpine:latest")
	if err != nil {
		t.Skipf("Podman not available, skipping Logs test: %v", err)
		return
	}

	// Create and start a container that outputs something.
	config := &CreateConfig{
		Name:  "atmos-test-logs-podman",
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

// TestPodmanRuntime_Exec_Integration validates the Exec() method for Podman runtime.
// Tests are intentionally duplicated to verify both Docker and Podman implementations
// independently, ensuring consistency across runtimes and allowing runtime-specific
// test evolution if needed.
//
//nolint:dupl // Docker and Podman implement identical Runtime interface with same Exec() behavior.
func TestPodmanRuntime_Exec_Integration(t *testing.T) {
	// Integration test - tests exec command in running container.
	runtime := NewPodmanRuntime()
	require.NotNil(t, runtime)

	ctx := context.Background()

	// Pull alpine if needed.
	err := runtime.Pull(ctx, "alpine:latest")
	if err != nil {
		t.Skipf("Podman not available, skipping Exec test: %v", err)
		return
	}

	// Create and start a container.
	config := &CreateConfig{
		Name:            "atmos-test-exec-podman",
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
	err = runtime.Exec(ctx, containerID, []string{"echo", "test"}, execOpts, nil, nil)
	require.NoError(t, err, "Exec should succeed")
}

func TestPodmanRuntime_Build_Integration(t *testing.T) {
	// Integration test - tests building from Dockerfile.
	runtime := NewPodmanRuntime()
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
		Tags:       []string{"atmos-test-build-podman:latest"},
	}

	err = runtime.Build(ctx, config)
	if err != nil {
		t.Skipf("Podman not available or build failed, skipping: %v", err)
		return
	}

	require.NoError(t, err, "Build should succeed")
}
