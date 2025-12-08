package container

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPodmanRuntime_cleanPodmanOutput_Comprehensive tests additional edge cases.
func TestPodmanRuntime_cleanPodmanOutput_Comprehensive(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "no escapes",
			input:    []byte("simple text"),
			expected: "simple text",
		},
		{
			name:     "multiple escape types mixed",
			input:    []byte("line1\\nline2\\ttab\\rcarriage"),
			expected: "line1\nline2\ttab\rcarriage",
		},
		{
			name:     "consecutive escapes",
			input:    []byte("text\\n\\n\\ndouble newlines"),
			expected: "text\n\n\ndouble newlines",
		},
		{
			name:     "escape at start",
			input:    []byte("\\nstarts with newline"),
			expected: "starts with newline", // Leading newline is trimmed by TrimSpace.
		},
		{
			name:     "escape at end",
			input:    []byte("ends with newline\\n"),
			expected: "ends with newline", // Trailing newline is trimmed by TrimSpace.
		},
		{
			name:     "only escapes",
			input:    []byte("\\n\\t\\r"),
			expected: "", // All whitespace is trimmed.
		},
		{
			name:     "whitespace with escapes trimmed",
			input:    []byte("  \\ntext\\n  "),
			expected: "text", // Leading/trailing whitespace including newlines is trimmed.
		},
		{
			name:     "real podman create output",
			input:    []byte("Resolving\\nTrying to pull\\nd31a46dd77566a9b55c6062cdc711a38453cb7f859f086c984a3a1fe77892703"),
			expected: "Resolving\nTrying to pull\nd31a46dd77566a9b55c6062cdc711a38453cb7f859f086c984a3a1fe77892703",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanPodmanOutput(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestBuildPodmanListArgs tests the argument construction for podman ps command.
func TestBuildPodmanListArgs(t *testing.T) {
	tests := []struct {
		name    string
		filters map[string]string
		// validateArgs is a function that validates the returned args.
		// We can't use exact equality due to map iteration order for filters.
		validateArgs func(t *testing.T, args []string)
	}{
		{
			name:    "no filters",
			filters: nil,
			validateArgs: func(t *testing.T, args []string) {
				// Exact match for no filters case.
				assert.Equal(t, []string{"ps", "-a", "--format", "json"}, args)
			},
		},
		{
			name: "single filter",
			filters: map[string]string{
				"status": "running",
			},
			validateArgs: func(t *testing.T, args []string) {
				// First 4 args must be base args in order.
				assert.Equal(t, "ps", args[0])
				assert.Equal(t, "-a", args[1])
				assert.Equal(t, "--format", args[2])
				assert.Equal(t, "json", args[3])
				// Next args are filter pairs.
				assert.Len(t, args, 6) // 4 base + 2 filter args.
				assert.Equal(t, "--filter", args[4])
				assert.Equal(t, "status=running", args[5])
			},
		},
		{
			name: "multiple filters",
			filters: map[string]string{
				"status": "exited",
				"name":   "test",
			},
			validateArgs: func(t *testing.T, args []string) {
				// First 4 args must be base args in order.
				assert.Equal(t, "ps", args[0])
				assert.Equal(t, "-a", args[1])
				assert.Equal(t, "--format", args[2])
				assert.Equal(t, "json", args[3])
				// Length must be base + 2 filter pairs.
				assert.Len(t, args, 8) // 4 base + 4 filter args (2 filters * 2 args each).
				// Verify filter args are present (order may vary due to map iteration).
				filterArgs := args[4:]
				assert.Contains(t, filterArgs, "--filter")
				assert.Contains(t, filterArgs, "status=exited")
				assert.Contains(t, filterArgs, "name=test")
				// Count --filter occurrences.
				filterCount := 0
				for _, arg := range filterArgs {
					if arg == "--filter" {
						filterCount++
					}
				}
				assert.Equal(t, 2, filterCount)
			},
		},
		{
			name:    "empty filter map",
			filters: map[string]string{},
			validateArgs: func(t *testing.T, args []string) {
				// Exact match for empty filters case.
				assert.Equal(t, []string{"ps", "-a", "--format", "json"}, args)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call the actual implementation.
			args := buildPodmanListArgs(tt.filters)

			// Validate using test-specific validation function.
			tt.validateArgs(t, args)
		})
	}
}

// TestPodmanRuntime_parsePodmanContainers_EdgeCases tests parsing edge cases.
func TestPodmanRuntime_parsePodmanContainers_EdgeCases(t *testing.T) {
	tests := []struct {
		name              string
		podmanContainers  []map[string]interface{}
		expectedCount     int
		expectedFirstName string
	}{
		{
			name: "single container",
			podmanContainers: []map[string]interface{}{
				{
					"Id":    "abc123",
					"Names": []interface{}{"container1"},
					"Image": "ubuntu",
					"State": "running",
				},
			},
			expectedCount:     1,
			expectedFirstName: "container1",
		},
		{
			name: "container without Names field",
			podmanContainers: []map[string]interface{}{
				{
					"Id":    "xyz789",
					"Image": "alpine",
					"State": "running",
				},
			},
			expectedCount:     1,
			expectedFirstName: "", // No name extracted.
		},
		{
			name: "container with empty Names array",
			podmanContainers: []map[string]interface{}{
				{
					"Id":    "empty123",
					"Names": []interface{}{},
					"Image": "node",
					"State": "exited",
				},
			},
			expectedCount:     1,
			expectedFirstName: "",
		},
		{
			name: "container with Labels as map",
			podmanContainers: []map[string]interface{}{
				{
					"Id":    "lab123",
					"Names": []interface{}{"with-labels"},
					"Image": "postgres",
					"State": "running",
					"Labels": map[string]interface{}{
						"app":     "database",
						"version": "14",
					},
				},
			},
			expectedCount:     1,
			expectedFirstName: "with-labels",
		},
		{
			name: "container with non-map Labels (ignored)",
			podmanContainers: []map[string]interface{}{
				{
					"Id":     "badlab123",
					"Names":  []interface{}{"bad-labels"},
					"Image":  "redis",
					"State":  "running",
					"Labels": "not-a-map",
				},
			},
			expectedCount:     1,
			expectedFirstName: "bad-labels",
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

// TestPodmanRuntime_parsePodmanContainer_DetailedEdgeCases tests individual container parsing.
func TestPodmanRuntime_parsePodmanContainer_DetailedEdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		containerJSON map[string]interface{}
		expected      Info
	}{
		{
			name: "full container with all fields",
			containerJSON: map[string]interface{}{
				"Id":    "full123abc",
				"Names": []interface{}{"full-container"},
				"Image": "ubuntu:22.04",
				"State": "running",
				"Labels": map[string]interface{}{
					"app":     "web",
					"env":     "prod",
					"version": "2.0",
				},
			},
			expected: Info{
				ID:     "full123abc",
				Name:   "full-container",
				Image:  "ubuntu:22.04",
				Status: "running",
				Labels: map[string]string{
					"app":     "web",
					"env":     "prod",
					"version": "2.0",
				},
			},
		},
		{
			name: "Names is string instead of array (invalid)",
			containerJSON: map[string]interface{}{
				"Id":    "bad123",
				"Names": "not-an-array",
				"Image": "alpine",
				"State": "exited",
			},
			expected: Info{
				ID:     "bad123",
				Name:   "", // extractPodmanName returns empty for non-array.
				Image:  "alpine",
				Status: "exited",
			},
		},
		{
			name: "Names array contains non-string (first element)",
			containerJSON: map[string]interface{}{
				"Id":    "nonstr123",
				"Names": []interface{}{123, "actual-name"},
				"Image": "node",
				"State": "running",
			},
			expected: Info{
				ID:     "nonstr123",
				Name:   "", // First element is not string.
				Image:  "node",
				Status: "running",
			},
		},
		{
			name: "Labels with mixed types (non-strings filtered)",
			containerJSON: map[string]interface{}{
				"Id":    "mixed123",
				"Names": []interface{}{"mixed"},
				"Image": "postgres",
				"State": "running",
				"Labels": map[string]interface{}{
					"app":     "db",
					"port":    5432,     // Number, should be filtered.
					"enabled": true,     // Boolean, should be filtered.
					"version": "14.0.1", // String, should be kept.
				},
			},
			expected: Info{
				ID:     "mixed123",
				Name:   "mixed",
				Image:  "postgres",
				Status: "running",
				Labels: map[string]string{
					"app":     "db",
					"version": "14.0.1",
				},
			},
		},
		{
			name: "Labels is nil",
			containerJSON: map[string]interface{}{
				"Id":     "nolab123",
				"Names":  []interface{}{"no-labels"},
				"Image":  "nginx",
				"State":  "running",
				"Labels": nil,
			},
			expected: Info{
				ID:     "nolab123",
				Name:   "no-labels",
				Image:  "nginx",
				Status: "running",
				Labels: nil,
			},
		},
		{
			name:          "completely empty container JSON",
			containerJSON: map[string]interface{}{},
			expected: Info{
				ID:     "",
				Name:   "",
				Image:  "",
				Status: "",
				Labels: nil,
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

// TestFindContainerByIDOrName tests the container search logic used by Inspect.
func TestFindContainerByIDOrName(t *testing.T) {
	// This tests the actual production search logic extracted from Inspect.

	tests := []struct {
		name         string
		searchID     string
		containers   []Info
		expectFound  bool
		expectedInfo *Info
	}{
		{
			name:     "find by exact ID match",
			searchID: "abc123",
			containers: []Info{
				{ID: "xyz789", Name: "container1", Image: "ubuntu"},
				{ID: "abc123", Name: "container2", Image: "alpine"},
				{ID: "def456", Name: "container3", Image: "node"},
			},
			expectFound: true,
			expectedInfo: &Info{
				ID:    "abc123",
				Name:  "container2",
				Image: "alpine",
			},
		},
		{
			name:     "find by exact name match",
			searchID: "my-container",
			containers: []Info{
				{ID: "xyz789", Name: "other-container", Image: "ubuntu"},
				{ID: "abc123", Name: "my-container", Image: "alpine"},
			},
			expectFound: true,
			expectedInfo: &Info{
				ID:    "abc123",
				Name:  "my-container",
				Image: "alpine",
			},
		},
		{
			name:     "not found - no match",
			searchID: "nonexistent",
			containers: []Info{
				{ID: "xyz789", Name: "container1", Image: "ubuntu"},
				{ID: "abc123", Name: "container2", Image: "alpine"},
			},
			expectFound:  false,
			expectedInfo: nil,
		},
		{
			name:         "not found - empty container list",
			searchID:     "anything",
			containers:   []Info{},
			expectFound:  false,
			expectedInfo: nil,
		},
		{
			name:     "find first match when both ID and name match different containers",
			searchID: "shared",
			containers: []Info{
				{ID: "shared", Name: "first", Image: "ubuntu"},
				{ID: "other", Name: "shared", Image: "alpine"},
			},
			expectFound: true,
			expectedInfo: &Info{
				ID:    "shared",
				Name:  "first",
				Image: "ubuntu",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call the actual production search function.
			result, err := findContainerByIDOrName(tt.containers, tt.searchID)

			// Validate results.
			if tt.expectFound {
				require.NoError(t, err, "findContainerByIDOrName should not return error when container is found")
				require.NotNil(t, result, "findContainerByIDOrName should return container info")
				assert.Equal(t, tt.expectedInfo.ID, result.ID)
				assert.Equal(t, tt.expectedInfo.Name, result.Name)
				assert.Equal(t, tt.expectedInfo.Image, result.Image)
			} else {
				require.Error(t, err, "findContainerByIDOrName should return error when container not found")
				assert.Nil(t, result, "findContainerByIDOrName should return nil when container not found")
				assert.ErrorIs(t, err, errUtils.ErrContainerNotFound)
			}
		})
	}
}

// TestPodmanRuntime_List_JSONUnmarshalEdgeCases tests JSON unmarshaling.
func TestPodmanRuntime_List_JSONUnmarshalEdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		jsonOutput    string
		expectError   bool
		expectedCount int
	}{
		{
			name: "valid JSON array",
			jsonOutput: `[
				{"Id":"abc123","Names":["container1"],"Image":"ubuntu","State":"running"},
				{"Id":"def456","Names":["container2"],"Image":"alpine","State":"exited"}
			]`,
			expectError:   false,
			expectedCount: 2,
		},
		{
			name:          "empty JSON array",
			jsonOutput:    `[]`,
			expectError:   false,
			expectedCount: 0,
		},
		{
			name: "JSON array with one element",
			jsonOutput: `[
				{"Id":"single123","Names":["single"],"Image":"node","State":"running"}
			]`,
			expectError:   false,
			expectedCount: 1,
		},
		{
			name:          "invalid JSON",
			jsonOutput:    `{invalid json}`,
			expectError:   true,
			expectedCount: 0,
		},
		{
			name:          "empty string",
			jsonOutput:    ``,
			expectError:   true,
			expectedCount: 0,
		},
		{
			name: "JSON with extra whitespace",
			jsonOutput: `
			[
				{"Id":"ws123","Names":["whitespace"],"Image":"redis","State":"running"}
			]
			`,
			expectError:   false,
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var podmanContainers []map[string]interface{}
			err := json.Unmarshal([]byte(tt.jsonOutput), &podmanContainers)

			if tt.expectError {
				assert.Error(t, err, "should fail to unmarshal")
			} else {
				require.NoError(t, err, "should unmarshal successfully")
				assert.Len(t, podmanContainers, tt.expectedCount)
			}
		})
	}
}

// TestPodmanRuntime_Create_ContainerIDExtraction tests the Create method's container ID extraction.
func TestPodmanRuntime_Create_ContainerIDExtraction(t *testing.T) {
	// This is already tested in podman_test.go's TestExtractContainerIDFromPodmanCreateOutput,
	// but we add a few more edge cases here.

	tests := []struct {
		name        string
		output      string
		expectedID  string
		expectEmpty bool
	}{
		{
			name:        "ID on last line",
			output:      "some output\nabc123def456",
			expectedID:  "abc123def456",
			expectEmpty: false,
		},
		{
			name:        "ID with trailing newlines",
			output:      "line1\nabc123\n\n",
			expectedID:  "abc123",
			expectEmpty: false,
		},
		{
			name:        "single line ID",
			output:      "containerid123",
			expectedID:  "containerid123",
			expectEmpty: false,
		},
		{
			name:        "empty output",
			output:      "",
			expectedID:  "",
			expectEmpty: true,
		},
		{
			name:        "only whitespace",
			output:      "   \n\t  \n",
			expectedID:  "",
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate the extraction logic from podman.go Create (lines 72-84).
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
				assert.Empty(t, containerID)
			} else {
				assert.Equal(t, tt.expectedID, containerID)
			}
		})
	}
}

// TestPodmanRuntime_NewPodmanRuntime tests constructor.
func TestPodmanRuntime_NewPodmanRuntime_Type(t *testing.T) {
	runtime := NewPodmanRuntime()
	require.NotNil(t, runtime)
	assert.IsType(t, &PodmanRuntime{}, runtime)

	// Verify it implements Runtime interface.
	var _ Runtime = runtime
}

// TestPodmanRuntime_Info_Structure tests Info method structure validation.
func TestPodmanRuntime_Info_Structure(t *testing.T) {
	runtime := NewPodmanRuntime()
	require.NotNil(t, runtime)

	ctx := context.Background()
	info, err := runtime.Info(ctx)

	// If podman is available.
	if err == nil {
		require.NotNil(t, info)
		assert.Equal(t, string(TypePodman), info.Type)
		assert.True(t, info.Running)
		assert.NotEmpty(t, info.Version)
	} else {
		// If podman is not available, error should be wrapped properly.
		assert.Error(t, err)
		assert.Nil(t, info)
	}
}
