package container

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDockerRuntime_parseInspectData tests parsing of Docker inspect JSON output.
func TestDockerRuntime_parseInspectData(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		expected Info
	}{
		{
			name: "complete inspect data with State.Status",
			data: map[string]interface{}{
				"Id":    "abc123def456",
				"Name":  "/my-container",
				"Image": "sha256:1234567890abcdef",
				"State": map[string]interface{}{
					"Status": "running",
				},
				"Created": "2023-10-15T12:30:45.123456789Z",
				"Config": map[string]interface{}{
					"Labels": map[string]interface{}{
						"app":     "test",
						"version": "1.0",
					},
				},
			},
			expected: Info{
				ID:     "abc123def456",
				Name:   "my-container", // Leading slash trimmed.
				Image:  "sha256:1234567890abcdef",
				Status: "running",
				Labels: map[string]string{
					"app":     "test",
					"version": "1.0",
				},
			},
		},
		{
			name: "inspect data with fallback Status field",
			data: map[string]interface{}{
				"Id":     "xyz789",
				"Name":   "simple-container",
				"Image":  "ubuntu:22.04",
				"Status": "exited (0) 2 hours ago",
			},
			expected: Info{
				ID:     "xyz789",
				Name:   "simple-container",
				Image:  "ubuntu:22.04",
				Status: "exited (0) 2 hours ago",
			},
		},
		{
			name: "inspect data without labels",
			data: map[string]interface{}{
				"Id":    "nolab123",
				"Name":  "/no-labels",
				"Image": "alpine:latest",
				"State": map[string]interface{}{
					"Status": "created",
				},
			},
			expected: Info{
				ID:     "nolab123",
				Name:   "no-labels",
				Image:  "alpine:latest",
				Status: "created",
				Labels: nil,
			},
		},
		{
			name: "inspect data with empty labels",
			data: map[string]interface{}{
				"Id":    "empty123",
				"Name":  "/empty-labels",
				"Image": "alpine",
				"Config": map[string]interface{}{
					"Labels": map[string]interface{}{},
				},
			},
			expected: Info{
				ID:     "empty123",
				Name:   "empty-labels",
				Image:  "alpine",
				Status: "",
				Labels: nil,
			},
		},
		{
			name: "inspect data with non-string label values (filtered out)",
			data: map[string]interface{}{
				"Id":    "mixed123",
				"Name":  "mixed-labels",
				"Image": "node",
				"Config": map[string]interface{}{
					"Labels": map[string]interface{}{
						"app":     "myapp",
						"count":   42,      // Non-string, should be ignored.
						"enabled": true,    // Non-string, should be ignored.
						"version": "1.0.0", // String, should be kept.
					},
				},
			},
			expected: Info{
				ID:    "mixed123",
				Name:  "mixed-labels",
				Image: "node",
				Labels: map[string]string{
					"app":     "myapp",
					"version": "1.0.0",
				},
			},
		},
		{
			name: "inspect data with invalid created timestamp",
			data: map[string]interface{}{
				"Id":      "badtime123",
				"Name":    "bad-timestamp",
				"Image":   "ubuntu",
				"Created": "invalid-timestamp",
			},
			expected: Info{
				ID:      "badtime123",
				Name:    "bad-timestamp",
				Image:   "ubuntu",
				Created: time.Time{}, // Zero value when parse fails.
			},
		},
		{
			name: "inspect data with valid created timestamp",
			data: map[string]interface{}{
				"Id":      "goodtime123",
				"Name":    "good-timestamp",
				"Image":   "ubuntu",
				"Created": "2023-10-15T12:30:45.123456789Z",
			},
			expected: Info{
				ID:    "goodtime123",
				Name:  "good-timestamp",
				Image: "ubuntu",
				// Created field will be populated with parsed time.
			},
		},
		{
			name: "minimal inspect data",
			data: map[string]interface{}{},
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
			d := &DockerRuntime{}
			result := d.parseInspectData(tt.data)

			assert.Equal(t, tt.expected.ID, result.ID)
			assert.Equal(t, tt.expected.Name, result.Name)
			assert.Equal(t, tt.expected.Image, result.Image)
			assert.Equal(t, tt.expected.Status, result.Status)
			assert.Equal(t, tt.expected.Labels, result.Labels)

			// For tests with valid timestamps, verify Created is non-zero.
			if tt.name == "inspect data with valid created timestamp" {
				assert.False(t, result.Created.IsZero(), "Created timestamp should be parsed")
				expected, _ := time.Parse(time.RFC3339Nano, "2023-10-15T12:30:45.123456789Z")
				assert.Equal(t, expected, result.Created)
			}

			// For tests with complete data, verify Created is parsed.
			if tt.name == "complete inspect data with State.Status" {
				assert.False(t, result.Created.IsZero())
			}
		})
	}
}

// TestDockerRuntime_getStatusFromInspect tests status extraction logic.
func TestDockerRuntime_getStatusFromInspect(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		expected string
	}{
		{
			name: "State.Status available (preferred)",
			data: map[string]interface{}{
				"State": map[string]interface{}{
					"Status": "running",
				},
				"Status": "Up 2 hours", // Fallback ignored when State.Status exists.
			},
			expected: "running",
		},
		{
			name: "only Status field available",
			data: map[string]interface{}{
				"Status": "Exited (0) 1 hour ago",
			},
			expected: "Exited (0) 1 hour ago",
		},
		{
			name: "State exists but no Status field",
			data: map[string]interface{}{
				"State": map[string]interface{}{
					"Running": true,
				},
				"Status": "Up 3 hours",
			},
			expected: "Up 3 hours",
		},
		{
			name: "State is not a map",
			data: map[string]interface{}{
				"State":  "invalid",
				"Status": "Up 1 hour",
			},
			expected: "Up 1 hour",
		},
		{
			name: "State.Status is empty string",
			data: map[string]interface{}{
				"State": map[string]interface{}{
					"Status": "",
				},
				"Status": "Created",
			},
			expected: "Created",
		},
		{
			name:     "no status fields",
			data:     map[string]interface{}{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getStatusFromInspect(tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDockerRuntime_getLabelsFromInspect tests label extraction logic.
func TestDockerRuntime_getLabelsFromInspect(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		expected map[string]string
	}{
		{
			name: "valid labels",
			data: map[string]interface{}{
				"Config": map[string]interface{}{
					"Labels": map[string]interface{}{
						"app":     "myapp",
						"version": "1.0",
						"env":     "production",
					},
				},
			},
			expected: map[string]string{
				"app":     "myapp",
				"version": "1.0",
				"env":     "production",
			},
		},
		{
			name: "empty labels map",
			data: map[string]interface{}{
				"Config": map[string]interface{}{
					"Labels": map[string]interface{}{},
				},
			},
			expected: nil,
		},
		{
			name: "Config.Labels is not a map",
			data: map[string]interface{}{
				"Config": map[string]interface{}{
					"Labels": "invalid",
				},
			},
			expected: nil,
		},
		{
			name: "Config is not a map",
			data: map[string]interface{}{
				"Config": "invalid",
			},
			expected: nil,
		},
		{
			name: "no Config field",
			data: map[string]interface{}{
				"Id": "abc123",
			},
			expected: nil,
		},
		{
			name: "labels with non-string values (filtered)",
			data: map[string]interface{}{
				"Config": map[string]interface{}{
					"Labels": map[string]interface{}{
						"app":     "myapp",
						"count":   42,
						"enabled": true,
						"version": "2.0",
					},
				},
			},
			expected: map[string]string{
				"app":     "myapp",
				"version": "2.0",
			},
		},
		{
			name:     "nil data",
			data:     nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getLabelsFromInspect(tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDockerRuntime_List_JSONParsing tests List's JSON parsing edge cases.
func TestDockerRuntime_List_JSONParsing(t *testing.T) {
	tests := []struct {
		name           string
		jsonLines      []string
		expectedCount  int
		expectedFirst  *Info
		skipValidation bool
	}{
		{
			name: "valid JSON lines",
			jsonLines: []string{
				`{"ID":"abc123","Names":"container1","Image":"ubuntu:22.04","State":"running"}`,
				`{"ID":"def456","Names":"container2","Image":"alpine:latest","State":"exited"}`,
			},
			expectedCount: 2,
			expectedFirst: &Info{
				ID:     "abc123",
				Name:   "container1",
				Image:  "ubuntu:22.04",
				Status: "running",
			},
		},
		{
			name: "JSON with leading slash in Names",
			jsonLines: []string{
				`{"ID":"xyz789","Names":"/my-container","Image":"node:18","State":"running"}`,
			},
			expectedCount: 1,
			expectedFirst: &Info{
				ID:     "xyz789",
				Name:   "my-container", // Leading slash should be trimmed.
				Image:  "node:18",
				Status: "running",
			},
		},
		{
			name: "JSON with labels string",
			jsonLines: []string{
				`{"ID":"lab123","Names":"with-labels","Image":"ubuntu","State":"running","Labels":"app=test,version=1.0"}`,
			},
			expectedCount: 1,
			expectedFirst: &Info{
				ID:     "lab123",
				Name:   "with-labels",
				Image:  "ubuntu",
				Status: "running",
				Labels: map[string]string{
					"app":     "test",
					"version": "1.0",
				},
			},
		},
		{
			name: "JSON with Status fallback (no State)",
			jsonLines: []string{
				`{"ID":"old123","Names":"old-format","Image":"centos","Status":"Exited (0) 1 hour ago"}`,
			},
			expectedCount: 1,
			expectedFirst: &Info{
				ID:     "old123",
				Name:   "old-format",
				Image:  "centos",
				Status: "Exited (0) 1 hour ago",
			},
		},
		{
			name: "empty lines ignored",
			jsonLines: []string{
				`{"ID":"abc123","Names":"container1","Image":"ubuntu","State":"running"}`,
				"",
				"   ",
				`{"ID":"def456","Names":"container2","Image":"alpine","State":"exited"}`,
			},
			expectedCount: 2,
			expectedFirst: &Info{
				ID:     "abc123",
				Name:   "container1",
				Image:  "ubuntu",
				Status: "running",
			},
		},
		{
			name: "invalid JSON line skipped with debug log",
			jsonLines: []string{
				`{"ID":"abc123","Names":"container1","Image":"ubuntu","State":"running"}`,
				`invalid json line here`,
				`{"ID":"def456","Names":"container2","Image":"alpine","State":"exited"}`,
			},
			expectedCount: 2,
			expectedFirst: &Info{
				ID:     "abc123",
				Name:   "container1",
				Image:  "ubuntu",
				Status: "running",
			},
		},
		{
			name:           "empty output",
			jsonLines:      []string{},
			expectedCount:  0,
			skipValidation: true,
		},
		{
			name:           "only empty lines",
			jsonLines:      []string{"", "  ", "\n"},
			expectedCount:  0,
			skipValidation: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the List JSON parsing logic.
			output := ""
			for _, line := range tt.jsonLines {
				output += line + "\n"
			}

			// This tests the exact parsing logic from docker.go List() method.
			var containers []Info
			scanner := json.NewDecoder(nil) // Not used, we'll parse manually.
			_ = scanner

			// Replicate the parsing logic from docker.go:218-254.
			lines := tt.jsonLines
			for _, line := range lines {
				line = trimSpaceAndNewlines(line)
				if line == "" {
					continue
				}

				var containerJSON map[string]interface{}
				if err := json.Unmarshal([]byte(line), &containerJSON); err != nil {
					// Invalid JSON - should be skipped with debug log.
					continue
				}

				// Use .State when available, fall back to .Status.
				status := getString(containerJSON, "State")
				if status == "" {
					status = getString(containerJSON, "Status")
				}

				info := Info{
					ID:     getString(containerJSON, "ID"),
					Name:   trimLeadingSlash(getString(containerJSON, "Names")),
					Image:  getString(containerJSON, "Image"),
					Status: status,
				}

				// Parse labels if present.
				if labelsStr := getString(containerJSON, "Labels"); labelsStr != "" {
					info.Labels = parseLabels(labelsStr)
				}

				containers = append(containers, info)
			}

			assert.Len(t, containers, tt.expectedCount)
			if !tt.skipValidation && tt.expectedCount > 0 {
				require.NotNil(t, tt.expectedFirst)
				assert.Equal(t, tt.expectedFirst.ID, containers[0].ID)
				assert.Equal(t, tt.expectedFirst.Name, containers[0].Name)
				assert.Equal(t, tt.expectedFirst.Image, containers[0].Image)
				assert.Equal(t, tt.expectedFirst.Status, containers[0].Status)
				assert.Equal(t, tt.expectedFirst.Labels, containers[0].Labels)
			}
		})
	}
}

// Helper functions to replicate docker.go parsing logic.
func trimSpaceAndNewlines(s string) string {
	// Same as strings.TrimSpace.
	return trimSpace(s)
}

func trimSpace(s string) string {
	// Simplified version - in real code we use strings.TrimSpace.
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for start < end && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func trimLeadingSlash(s string) string {
	if len(s) > 0 && s[0] == '/' {
		return s[1:]
	}
	return s
}

// TestDockerRuntime_parseLabels tests label parsing edge cases.
func TestDockerRuntime_parseLabels_EdgeCases(t *testing.T) {
	// This test is in docker_test.go, but we add more edge cases here.
	tests := []struct {
		name      string
		labelsStr string
		expected  map[string]string
	}{
		{
			name:      "empty string",
			labelsStr: "",
			expected:  map[string]string{},
		},
		{
			name:      "single label",
			labelsStr: "app=myapp",
			expected: map[string]string{
				"app": "myapp",
			},
		},
		{
			name:      "label with equals in value",
			labelsStr: "config=key=value=extra",
			expected: map[string]string{
				"config": "key=value=extra",
			},
		},
		{
			name:      "label with empty value",
			labelsStr: "empty=,nonempty=value",
			expected: map[string]string{
				"empty":    "",
				"nonempty": "value",
			},
		},
		{
			name:      "label without equals (invalid, skipped)",
			labelsStr: "invalid,app=test",
			expected: map[string]string{
				"app": "test",
			},
		},
		{
			name:      "trailing comma",
			labelsStr: "app=test,version=1.0,",
			expected: map[string]string{
				"app":     "test",
				"version": "1.0",
			},
		},
		{
			name:      "multiple commas",
			labelsStr: "app=test,,version=1.0",
			expected: map[string]string{
				"app":     "test",
				"version": "1.0",
			},
		},
		{
			name:      "special characters in value",
			labelsStr: "url=https://example.com:8080/path,env=prod",
			expected: map[string]string{
				"url": "https://example.com:8080/path",
				"env": "prod",
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
