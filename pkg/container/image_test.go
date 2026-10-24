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

func TestParseImageInspectOutput_RichMetadata(t *testing.T) {
	info, err := parseImageInspectOutput([]byte(`{
		"Id": "sha256:image-id",
		"RepoTags": ["app:latest"],
		"RepoDigests": ["app@sha256:abc123"],
		"Size": 8912896,
		"Created": "2026-06-18T23:06:08Z",
		"Architecture": "arm64",
		"Os": "linux",
		"Author": "Cloud Posse",
		"Config": {
			"Env": ["PATH=/bin", "APP_ENV=test"],
			"Cmd": ["./app"],
			"Entrypoint": ["/entrypoint.sh"],
			"ExposedPorts": {"8080/tcp": {}, "9090/tcp": {}},
			"StopSignal": "SIGTERM",
			"Labels": {"org.opencontainers.image.title": "App", "extra": "x"}
		},
		"GraphDriver": {"Name": "overlay2"},
		"RootFS": {"Type": "layers", "Layers": ["sha256:l1", "sha256:l2", "sha256:l3"]}
	}`))

	require.NoError(t, err)
	assert.Equal(t, int64(8912896), info.Size)
	assert.Equal(t, "2026-06-18T23:06:08Z", info.Created)
	assert.Equal(t, "arm64", info.Architecture)
	assert.Equal(t, "linux", info.Os)
	assert.Equal(t, "Cloud Posse", info.Author)
	assert.Equal(t, "App", info.Labels["org.opencontainers.image.title"])
	assert.Equal(t, []string{"PATH=/bin", "APP_ENV=test"}, info.Env)
	assert.Equal(t, []string{"./app"}, info.Cmd)
	assert.Equal(t, []string{"/entrypoint.sh"}, info.Entrypoint)
	assert.Equal(t, []string{"8080/tcp", "9090/tcp"}, info.ExposedPorts)
	assert.Equal(t, "SIGTERM", info.StopSignal)
	assert.Equal(t, "overlay2", info.StorageDriver)
	assert.Equal(t, []string{"sha256:l1", "sha256:l2", "sha256:l3"}, info.LayerDigests)
	assert.Equal(t, 3, info.Layers)
	assert.Contains(t, info.RawInspectJSON, "[")
	assert.Contains(t, info.RawInspectJSON, `"Id": "sha256:image-id"`)
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
