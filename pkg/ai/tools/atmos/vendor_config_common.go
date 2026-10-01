package atmos

import (
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
)

// defaultVendorManifest is the default vendor manifest filename, matching
// cmd/vendor.DefaultVendorManifest.
const defaultVendorManifest = "vendor.yaml"

// resolveVendorConfigFile picks the vendor manifest to operate on: the
// caller-supplied file override, otherwise ./vendor.yaml in the current
// working directory. This mirrors cmd/vendor.resolveVendorFileWithOverride so
// AI tool behavior matches the `atmos vendor config` CLI commands.
func resolveVendorConfigFile(fileParam string) (string, error) {
	if fileParam != "" {
		return fileParam, nil
	}
	if _, err := os.Stat(defaultVendorManifest); err == nil {
		return defaultVendorManifest, nil
	}
	return "", errUtils.ErrAIVendorFileNotFound
}
