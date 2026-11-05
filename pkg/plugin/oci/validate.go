package oci

import (
	"errors"
	"io/fs"
	"maps"
	"slices"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
)

// ValidatePackage applies the packaging guards that need no built binary and no
// code execution: the path guard on every declared executable and schema file, and
// the native-format check on each platform executable that already exists. A
// platform whose binary has not been built yet is skipped; its path is still checked.
func ValidatePackage(directory string, definition *manifest.Config) error {
	defer perf.Track(nil, "pluginoci.ValidatePackage")()

	if err := checkDeclaredPaths(declaredExecutables(definition)...); err != nil {
		return err
	}
	if _, err := packageFiles(directory); err != nil {
		return err
	}
	for _, platform := range slices.Sorted(maps.Keys(definition.Spec.Platforms)) {
		parsed, err := parsePlatform(platform)
		if err != nil {
			return err
		}
		file, _, err := openPackageFile(directory, definition.Spec.Platforms[platform].Executable)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		err = checkExecutableFormat(file, parsed)
		_ = file.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
