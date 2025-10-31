package container

import (
	"context"
	"testing"

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
