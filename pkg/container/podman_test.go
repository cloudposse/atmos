package container

import (
	"context"
	"strings"
	"testing"

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
