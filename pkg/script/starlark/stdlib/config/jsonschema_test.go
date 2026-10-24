package config

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/url"
	"path/filepath"
	"testing"
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
