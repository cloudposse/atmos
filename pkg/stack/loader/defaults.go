package loader

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// SupportedExtensions returns a list of all supported file extensions.
// This is a static list of extensions that the loader system supports.
func SupportedExtensions() []string {
	defer perf.Track(nil, "loader.SupportedExtensions")()

	return []string{
		".yaml",
		".yml",
		".json",
		".hcl",
		".tf",
	}
}
