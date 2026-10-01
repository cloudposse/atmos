package atmos

import (
	"fmt"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// resolveTerraformComponentFilePath resolves file_path against the
// Terraform components base path and enforces the same sandbox guard used
// by read_component_file.go/write_component_file.go, scoped to terraform
// only since the HCL editing tools have no component_type param -- hcledit
// only understands HCL, and Terraform is the only component type that uses
// it (helmfile is YAML, packer templates may be JSON).
func resolveTerraformComponentFilePath(atmosConfig *schema.AtmosConfiguration, filePath string) (string, error) {
	basePath := atmosConfig.Components.Terraform.BasePath

	absoluteBasePath := basePath
	if !filepath.IsAbs(basePath) {
		absoluteBasePath = filepath.Join(atmosConfig.BasePath, basePath)
	}

	fullPath := filepath.Join(absoluteBasePath, filePath)
	cleanPath := filepath.Clean(fullPath)
	cleanBase := filepath.Clean(absoluteBasePath)

	if !strings.HasPrefix(cleanPath, cleanBase+string(filepath.Separator)) && cleanPath != cleanBase {
		return "", errUtils.ErrAIFileAccessDeniedComponents
	}

	return cleanPath, nil
}

// extractRequiredStringParam extracts a required string parameter, or
// ErrAIToolParameterRequired if missing/empty.
func extractRequiredStringParam(params map[string]interface{}, name string) (string, error) {
	value, ok := params[name].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("%w: %s", errUtils.ErrAIToolParameterRequired, name)
	}
	return value, nil
}
