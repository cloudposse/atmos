package aws

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// GetFilesBasePath extracts files.base_path from provider spec.
// Returns empty string if not configured.
func GetFilesBasePath(provider *schema.Provider) string {
	if provider == nil || provider.Spec == nil {
		return ""
	}

	files, ok := provider.Spec["files"].(map[string]interface{})
	if !ok {
		return ""
	}

	basePath, ok := files["base_path"].(string)
	if !ok {
		return ""
	}

	return basePath
}
