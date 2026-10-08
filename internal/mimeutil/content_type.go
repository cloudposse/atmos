// Package mimeutil resolves consistent content types for published files.
package mimeutil

import (
	"mime"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ContentType prefers the filename extension, falls back to detected content,
// and preserves a detected charset for text formats without assuming UTF-8.
func ContentType(path, detected string) string {
	defer perf.Track(nil, "mimeutil.ContentType")()

	extension := strings.ToLower(filepath.Ext(path))
	contentType := mime.TypeByExtension(extension)
	if contentType == "" {
		contentType = detected
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	mediaType, parameters, err := mime.ParseMediaType(contentType)
	if err != nil || !isText(mediaType) {
		return contentType
	}
	// Extension databases may assume UTF-8 regardless of the file's bytes.
	delete(parameters, "charset")
	if _, detectedParameters, err := mime.ParseMediaType(detected); err == nil {
		if charset := detectedParameters["charset"]; charset != "" {
			parameters["charset"] = charset
		}
	}
	return mime.FormatMediaType(mediaType, parameters)
}

func isText(mediaType string) bool {
	return strings.HasPrefix(mediaType, "text/") ||
		strings.HasSuffix(mediaType, "+json") ||
		strings.HasSuffix(mediaType, "+xml") ||
		strings.Contains(mediaType, "javascript") ||
		mediaType == "application/json" ||
		mediaType == "application/toml" ||
		mediaType == "application/xml" ||
		mediaType == "application/yaml"
}
