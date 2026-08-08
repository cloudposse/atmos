package exec

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Path resolution for vendoring. Three questions get answered here, and they have different answers
// on purpose:
//
//   - Where is the vendor manifest?          resolveVendorConfigFilePath
//   - Where does a relative `source` point?  resolveVendorSourceBasePath (the declaring manifest)
//   - Where does a relative `target` land?   resolveVendorTargetBasePath (one root per project)
//
// Sources are anchored per manifest so an imported manifest stays self-contained; targets are
// anchored to a single root so splitting a manifest into imports never moves vendored artifacts.

// Helper function to resolve the vendor config file path.
func resolveVendorConfigFilePath(atmosConfig *schema.AtmosConfiguration, vendorConfigFile string, checkGlobalConfig bool) string {
	if checkGlobalConfig && atmosConfig.Vendor.BasePath != "" {
		if !filepath.IsAbs(atmosConfig.Vendor.BasePath) {
			return filepath.Join(atmosConfig.BasePath, atmosConfig.Vendor.BasePath)
		}
		return atmosConfig.Vendor.BasePath
	}

	// Search for the vendor config file
	foundVendorConfigFile, fileExists := u.SearchConfigFile(vendorConfigFile)
	if !fileExists {
		pathToVendorConfig := filepath.Join(atmosConfig.BasePath, vendorConfigFile)
		foundVendorConfigFile, fileExists = u.SearchConfigFile(pathToVendorConfig)
		if !fileExists {
			return "" // File does not exist, but this is not an error
		}
	}
	return foundVendorConfigFile
}

// resolveVendorTargetBasePath returns the directory that relative `targets` paths in the vendor
// manifest are resolved against.
//
// When the manifest location is declared in `atmos.yaml` via the `vendor.base_path` setting, the
// manifest is part of the Atmos configuration and can live anywhere (e.g. next to `atmos.yaml` in a
// dedicated config directory outside the repo root). In that case relative targets are resolved
// against the Atmos `base_path` — the same root that `components.terraform.base_path` and
// `stacks.base_path` are relative to — so vendored artifacts land where the rest of the
// configuration expects them, not next to the manifest.
//
// Otherwise the manifest was discovered next to the working directory, and relative targets stay
// relative to the manifest itself.
func resolveVendorTargetBasePath(atmosConfig *schema.AtmosConfiguration, vendorConfigFileName string) string {
	vendorConfigFilePath := filepath.Dir(vendorConfigFileName)

	if atmosConfig == nil || atmosConfig.Vendor.BasePath == "" || atmosConfig.BasePath == "" {
		return vendorConfigFilePath
	}

	if atmosConfig.BasePath != vendorConfigFilePath {
		log.Debug("Resolving relative vendor targets against the Atmos base path",
			"vendor_config_file", vendorConfigFileName,
			"vendor_base_path", atmosConfig.Vendor.BasePath,
			"base_path", atmosConfig.BasePath,
		)
	}

	return atmosConfig.BasePath
}

// resolveVendorSourceBasePath returns the directory a relative local `source` is resolved against:
// the directory of the manifest that declared it, following the Atmos convention that a relative
// path in a configuration file is anchored to the file declaring it
// (see docs/prd/base-path-resolution-semantics.md).
//
// Sources pulled in through `spec.imports` are merged into one flat list, so without the per-source
// provenance recorded in BasePath they would all be anchored to the root manifest instead of the
// imported manifest that declares them. Falls back to the root manifest's directory for sources
// that carry no provenance.
func resolveVendorSourceBasePath(source *schema.AtmosVendorSource, vendorConfigFilePath string) string {
	if source != nil && source.BasePath != "" {
		return source.BasePath
	}
	return vendorConfigFilePath
}

// resolveVendorTargetPath resolves a single templated target path against the vendor target base
// path. Absolute targets are honored as-is — joining them to the base path would nest them under it.
func resolveVendorTargetPath(targetBasePath, target string) string {
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	return filepath.Join(targetBasePath, target)
}

// determineSourceType classifies a vendor source URI as OCI, local, or remote. sourceBasePath is the
// directory a relative local source is anchored to — the manifest that declared the source, see
// resolveVendorSourceBasePath.
func determineSourceType(uri *string, sourceBasePath string) (bool, bool, bool, error) {
	// Determine if the URI is an OCI scheme, a local file, or remote
	useLocalFileSystem := false
	sourceIsLocalFile := false
	useOciScheme := strings.HasPrefix(*uri, "oci://")
	if useOciScheme {
		*uri = strings.TrimPrefix(*uri, "oci://")
		return useOciScheme, useLocalFileSystem, sourceIsLocalFile, nil
	}

	absPath, err := u.JoinPathAndValidate(sourceBasePath, *uri)
	// if URI contain path traversal is path should be resolved
	if err != nil && strings.Contains(*uri, "..") && !strings.HasPrefix(*uri, "file://") {
		return useOciScheme, useLocalFileSystem, sourceIsLocalFile, fmt.Errorf("invalid source path '%s': %w", *uri, err)
	}
	if err == nil {
		// Write the anchored path back to the caller: the package is downloaded from this URI, so
		// leaving it relative would resolve it against the process working directory instead of the
		// manifest that declared it. Return here rather than falling through to url.Parse — the URI
		// is now a filesystem path, and parsing it as a URL would read a Windows drive letter
		// ("C:\...") as a scheme.
		*uri = absPath
		useLocalFileSystem = true
		sourceIsLocalFile = u.FileExists(*uri)
		return useOciScheme, useLocalFileSystem, sourceIsLocalFile, nil
	}

	parsedURL, err := url.Parse(*uri)
	if err != nil {
		return useOciScheme, useLocalFileSystem, sourceIsLocalFile, err
	}
	if parsedURL.Scheme != "" {
		if parsedURL.Scheme == "file" {
			trimmedPath := strings.TrimPrefix(filepath.ToSlash(parsedURL.Path), "/")
			*uri = filepath.Clean(trimmedPath)
			useLocalFileSystem = true
		}
	}

	return useOciScheme, useLocalFileSystem, sourceIsLocalFile, nil
}
