package config

import (
	"net/url"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

func TestSchemaFileURLs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ path, want string }{
		{"/tmp/schema.json", "file:///tmp/schema.json"},
		{"C:/schemas/app.json", "file:///C:/schemas/app.json"},
		{"D:/schemas with spaces/app#%.yaml", "file:///D:/schemas%20with%20spaces/app%23%25.yaml"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			location := (&url.URL{Scheme: "file", Path: schemaURLPath(tc.path)}).String()
			assert.Equal(t, tc.want, location)
			resource, err := url.Parse(location)
			require.NoError(t, err)
			assert.Empty(t, resource.Host)
			assert.Equal(t, filepath.FromSlash(tc.path), schemaFilesystemPath(resource.Path, "windows"))
		})
	}
	assert.Equal(t, filepath.FromSlash("/C:/schema.json"), schemaFilesystemPath("/C:/schema.json", "linux"))
	assert.Equal(t, filepath.FromSlash("/tmp/schema.json"), schemaFilesystemPath("/tmp/schema.json", "windows"))
}

// Reject invalid resource URLs before invoking the host's file reader.
func TestConfigSchemaResourceURLs(t *testing.T) {
	t.Parallel()
	binding := &binding{host: Host{ReadFile: func(string) ([]byte, error) {
		t.Error("invalid resource URL must not trigger a file read")
		return nil, nil
	}}}
	for _, tc := range []struct{ location, want string }{
		{"file://remote/schema.json", "invalid local schema URL"},
		{"file:///schema.json?query=1", "invalid local schema URL"},
		{"file:///%zz", "invalid URL escape"},
		{"http://example.com/schema.json", "local files or HTTPS"},
	} {
		t.Run(tc.location, func(t *testing.T) {
			t.Parallel()
			reader, err := binding.loadConfigSchema(&starlark.Thread{}, tc.location)
			require.ErrorContains(t, err, tc.want)
			assert.Nil(t, reader)
		})
	}
}
