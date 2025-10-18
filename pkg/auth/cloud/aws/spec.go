package aws

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GetFilesBasePath extracts files.base_path from provider spec.
// Returns empty string if not configured.
func GetFilesBasePath(provider *schema.Provider) string {
	defer perf.Track(nil, "aws.GetFilesBasePath")()

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

// ValidateFilesBasePath validates spec.files.base_path if provided.
func ValidateFilesBasePath(provider *schema.Provider) error {
	basePath := GetFilesBasePath(provider)
	if basePath == "" {
		return nil // Optional field.
	}

	// Validate path is not empty after trimming.
	trimmed := strings.TrimSpace(basePath)
	if trimmed == "" {
		return fmt.Errorf("%w: spec.files.base_path cannot be empty or whitespace", errUtils.ErrInvalidProviderConfig)
	}

	// Validate path doesn't contain invalid characters.
	if strings.ContainsAny(basePath, "\x00\r\n") {
		return fmt.Errorf("%w: spec.files.base_path contains invalid characters", errUtils.ErrInvalidProviderConfig)
	}

	return nil
}
