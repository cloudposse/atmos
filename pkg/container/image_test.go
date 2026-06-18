package container

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePushDigest(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name:     "docker style digest",
			output:   "latest: digest: sha256:abc123 size: 1234",
			expected: "sha256:abc123",
		},
		{
			name:     "podman multiline digest",
			output:   "Copying blob\nWriting manifest\ndigest: sha256:ABCDEF123456\n",
			expected: "sha256:ABCDEF123456",
		},
		{
			name:     "missing digest",
			output:   "pushed without digest line",
			expected: "",
		},
		{
			name:     "invalid digest prefix",
			output:   "digest: md5:abc123",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parsePushDigest(tt.output))
		})
	}
}

func TestParseImageInspectOutput(t *testing.T) {
	info, err := parseImageInspectOutput([]byte(`{
		"Id": "sha256:image-id",
		"RepoTags": ["app:latest", "registry.example.com/app:latest"],
		"RepoDigests": ["app@sha256:abc123"],
		"Extra": ["ignored"]
	}`))

	require.NoError(t, err)
	assert.Equal(t, "sha256:image-id", info.ID)
	assert.Equal(t, []string{"app:latest", "registry.example.com/app:latest"}, info.RepoTags)
	assert.Equal(t, []string{"app@sha256:abc123"}, info.RepoDigests)
}

func TestParseImageInspectOutput_InvalidJSON(t *testing.T) {
	info, err := parseImageInspectOutput([]byte(`not-json`))

	require.Error(t, err)
	assert.Nil(t, info)
}

func TestGetStringSlice(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]any
		key      string
		expected []string
	}{
		{
			name:     "strings only",
			data:     map[string]any{"items": []any{"one", "two"}},
			key:      "items",
			expected: []string{"one", "two"},
		},
		{
			name:     "filters non-string values",
			data:     map[string]any{"items": []any{"one", 2, true, "two"}},
			key:      "items",
			expected: []string{"one", "two"},
		},
		{
			name:     "missing key",
			data:     map[string]any{},
			key:      "items",
			expected: nil,
		},
		{
			name:     "wrong type",
			data:     map[string]any{"items": "one"},
			key:      "items",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, getStringSlice(tt.data, tt.key))
		})
	}
}
