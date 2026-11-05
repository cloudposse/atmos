package mimeutil

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContentType(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ filename, detected, want string }{
		{"index.HTML", "text/plain; charset=utf-8", "text/html; charset=utf-8"},
		{"app.js", "text/plain; charset=iso-8859-1", "text/javascript; charset=iso-8859-1"},
		{"data.json", "application/json", "application/json"},
		{"image.png", "text/plain; charset=utf-8", "image/png"},
		{"unknown.atmosunknown", "", "application/octet-stream"},
		{"unknown.atmosunknown", "invalid mime type", "invalid mime type"},
		{"unknown.atmosunknown", "text/plain; charset=iso-8859-1", "text/plain; charset=iso-8859-1"},
		{"unknown.atmosunknown", "application/manifest+json", "application/manifest+json"},
		{"unknown.atmosunknown", "image/svg+xml", "image/svg+xml"},
		{"unknown.atmosunknown", "application/javascript", "application/javascript"},
		{"unknown.atmosunknown", "application/toml", "application/toml"},
		{"unknown.atmosunknown", "application/xml; charset=utf-16le", "application/xml; charset=utf-16le"},
		{"unknown.atmosunknown", "application/yaml", "application/yaml"},
		{"notes.txt", "application/octet-stream", "text/plain"},
		{"notes.txt", "invalid mime type", "text/plain"},
	} {
		t.Run(tc.filename+"/"+tc.detected, func(t *testing.T) {
			t.Parallel()
			// Windows registers the application/javascript alias for .js files.
			got := strings.Replace(ContentType(tc.filename, tc.detected), "application/javascript;", "text/javascript;", 1)
			assert.Equal(t, tc.want, got)
		})
	}
}
