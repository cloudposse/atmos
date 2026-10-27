package exec

import (
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/output"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const DefaultFileMode = output.DefaultFileMode

func printOrWriteToFile(config *schema.AtmosConfiguration, format, file string, data any) error {
	return output.WriteDocument(output.DocumentRequest{AtmosConfig: config, Format: format, File: file, Data: data})
}

// SanitizeFileName sanitizes the filename in a URI.
//
// Deprecated: Use filesystem.SanitizeFileName.
func SanitizeFileName(uri string) string {
	defer perf.Track(nil, "exec.SanitizeFileName")()

	return filesystem.SanitizeFileName(uri)
}
