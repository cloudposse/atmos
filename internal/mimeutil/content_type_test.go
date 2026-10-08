package mimeutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContentType(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ filename, detected, want string }{
		{"index.HTML", "text/plain", "text/html; charset=utf-8"},
		{"app.js", "text/plain", "text/javascript; charset=utf-8"},
		{"data.json", "", "application/json; charset=utf-8"},
		{"image.png", "text/plain", "image/png"},
		{"unknown.atmosunknown", "", "application/octet-stream"},
		{"unknown.atmosunknown", "invalid mime type", "invalid mime type"},
		{"unknown.atmosunknown", "text/plain; charset=iso-8859-1", "text/plain; charset=utf-8"},
		{"unknown.atmosunknown", "application/manifest+json", "application/manifest+json; charset=utf-8"},
		{"unknown.atmosunknown", "image/svg+xml", "image/svg+xml; charset=utf-8"},
		{"unknown.atmosunknown", "application/javascript", "application/javascript; charset=utf-8"},
		{"unknown.atmosunknown", "application/toml", "application/toml; charset=utf-8"},
		{"unknown.atmosunknown", "application/xml", "application/xml; charset=utf-8"},
		{"unknown.atmosunknown", "application/yaml", "application/yaml; charset=utf-8"},
	} {
		t.Run(tc.filename+"/"+tc.detected, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, ContentType(tc.filename, tc.detected))
		})
	}
}
