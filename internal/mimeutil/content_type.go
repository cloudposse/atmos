// Package mimeutil resolves consistent content types for published files.
package mimeutil

import (
	"mime"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ContentType prefers the filename extension, falls back to detected content,
// and sets UTF-8 for text formats used by Atmos artifacts and site deployments.
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
	if err != nil || !usesUTF8(mediaType) {
		return contentType
	}
	parameters["charset"] = "utf-8"
	return mime.FormatMediaType(mediaType, parameters)
}

func usesUTF8(mediaType string) bool {
	return strings.HasPrefix(mediaType, "text/") ||
		strings.HasSuffix(mediaType, "+json") ||
		strings.HasSuffix(mediaType, "+xml") ||
		strings.Contains(mediaType, "javascript") ||
		mediaType == "application/json" ||
		mediaType == "application/toml" ||
		mediaType == "application/xml" ||
		mediaType == "application/yaml"
}
