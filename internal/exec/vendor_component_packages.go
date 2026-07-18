package exec

import (
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// buildComponentVendorPackages resolves a single component's vendor spec (plus any mixins) into
// the package list executeVendorModel consumes, without executing the pull. Extracted from
// ExecuteComponentVendorInternal so ExecuteComponentVendorPullBatch can build package lists for
// multiple components and combine them into one executeVendorModel call.
func buildComponentVendorPackages(
	atmosConfig *schema.AtmosConfiguration,
	vendorComponentSpec *schema.VendorComponentSpec,
	component string,
	componentPath string,
) ([]pkgComponentVendor, error) {
	if vendorComponentSpec.Source.Uri == "" {
		return nil, fmt.Errorf("%w:'%s'", ErrUriMustSpecified, cfg.ComponentVendorConfigFileName)
	}
	uri := vendorComponentSpec.Source.Uri
	// Parse 'uri' template. Uses ProcessTmpl (the same helper vendor_utils.go uses for the
	// top-level vendor.yaml source/target templating) so component.yaml source URIs get the
	// same Sprig, gomplate, and Atmos template functions (the "atmos" namespace from
	// internal/exec/template_funcs.go) rather than a narrower, hand-rolled function map.
	if vendorComponentSpec.Source.Version != "" {
		var err error
		uri, err = ProcessTmpl(atmosConfig, fmt.Sprintf("source-uri-%s", vendorComponentSpec.Source.Version), vendorComponentSpec.Source.Uri, vendorComponentSpec.Source, false)
		if err != nil {
			return nil, errUtils.Build(errUtils.ErrTemplateEvaluation).
				WithCause(err).
				WithContext("component", component).
				WithContext("uri", vendorComponentSpec.Source.Uri).
				Err()
		}
	}
	var useOciScheme, useLocalFileSystem, sourceIsLocalFile bool

	// Check if `uri` uses the `oci://` scheme (to download the sources from an OCI-compatible registry).
	if strings.HasPrefix(uri, ociScheme) {
		useOciScheme = true
		uri = strings.TrimPrefix(uri, ociScheme)
	}

	if !useOciScheme {
		uri, useLocalFileSystem, sourceIsLocalFile = handleLocalFileScheme(componentPath, uri)
	}
	pType := determinePackageType(useOciScheme, useLocalFileSystem)
	componentPkg := pkgComponentVendor{
		uri:                 uri,
		name:                component,
		componentPath:       componentPath,
		sourceIsLocalFile:   sourceIsLocalFile,
		pkgType:             pType,
		version:             vendorComponentSpec.Source.Version,
		vendorComponentSpec: vendorComponentSpec,
		IsComponent:         true,
	}

	var packages []pkgComponentVendor
	packages = append(packages, componentPkg)
	// Process mixins
	if len(vendorComponentSpec.Mixins) > 0 {
		mixinPkgs, err := processComponentMixins(atmosConfig, vendorComponentSpec, componentPath)
		if err != nil {
			return nil, err
		}
		packages = append(packages, mixinPkgs...)
	}
	return packages, nil
}

// handleLocalFileScheme processes the URI for local file system paths.
// Check if `uri` is a file path.
// If it's a file path, check if it's an absolute path.
// If it's not absolute path, join it with the base path (component dir) and convert to absolute path.
func handleLocalFileScheme(componentPath, uri string) (string, bool, bool) {
	var useLocalFileSystem, sourceIsLocalFile bool

	// Handle absolute path resolution
	if absPath, err := u.JoinPathAndValidate(componentPath, uri); err == nil {
		uri = absPath
		useLocalFileSystem = true
		sourceIsLocalFile = u.FileExists(uri)
	}

	// Handle file:// scheme. Shared with processComponentMixins (via resolveFileURIPath) so
	// component sources and mixins normalize `file://` URIs identically.
	if resolved, ok := resolveFileURIPath(uri); ok {
		uri = resolved
		useLocalFileSystem = true
		sourceIsLocalFile = u.FileExists(uri)
	}

	return uri, useLocalFileSystem, sourceIsLocalFile
}

// resolveFileURIPath converts a "file://" URI into a local filesystem path, preserving the
// URI's root instead of trimming it down to a relative path. `file:///tmp/source` resolves to
// the absolute path "/tmp/source" (not the relative "tmp/source"); on Windows, a drive-letter
// path like `file:///C:/foo` resolves to `C:\foo`, and a UNC path like `file://host/share/foo`
// resolves to `\\host\share\foo`. Returns ok=false when uri does not use the "file" scheme.
func resolveFileURIPath(uri string) (path string, ok bool) {
	parsedURL, err := url.Parse(uri)
	if err != nil || parsedURL.Scheme != "file" {
		return "", false
	}

	p := filepath.FromSlash(parsedURL.Path)
	if runtime.GOOS == "windows" {
		if parsedURL.Host != "" && !strings.EqualFold(parsedURL.Host, "localhost") {
			// UNC path: file://host/share/foo -> \\host\share\foo.
			return filepath.Clean(`\\` + parsedURL.Host + p), true
		}
		// Strip the spurious leading separator before a drive letter: /C:/foo -> C:\foo.
		if len(p) >= 3 && p[0] == filepath.Separator && p[2] == ':' {
			return filepath.Clean(p[1:]), true
		}
	}
	return filepath.Clean(p), true
}

func processComponentMixins(atmosConfig *schema.AtmosConfiguration, vendorComponentSpec *schema.VendorComponentSpec, componentPath string) ([]pkgComponentVendor, error) {
	var packages []pkgComponentVendor
	for _, mixin := range vendorComponentSpec.Mixins {
		if mixin.Uri == "" {
			return nil, ErrMissingMixinURI
		}

		if mixin.Filename == "" {
			return nil, ErrMissingMixinFilename
		}

		pkg, alreadyMaterialized, err := resolveMixinPackage(atmosConfig, vendorComponentSpec, &mixin, componentPath)
		if err != nil {
			return nil, err
		}
		if alreadyMaterialized {
			continue
		}

		packages = append(packages, pkg)
	}
	return packages, nil
}

// resolveMixinPackage resolves a single mixin's URI template and local-file/OCI/remote
// classification into a package. The alreadyMaterialized flag is true when the mixin's target
// already exists locally, in which case pkg is the zero value and there is nothing to fetch.
func resolveMixinPackage(
	atmosConfig *schema.AtmosConfiguration,
	vendorComponentSpec *schema.VendorComponentSpec,
	mixin *schema.VendorComponentMixins,
	componentPath string,
) (pkg pkgComponentVendor, alreadyMaterialized bool, err error) {
	// Parse 'uri' template.
	uri, err := parseMixinURI(atmosConfig, mixin)
	if err != nil {
		return pkgComponentVendor{}, false, fmt.Errorf("mixin %q (uri %q): %w", mixin.Filename, mixin.Uri, err)
	}

	// Normalize `file://` URIs so they keep their absolute root (see resolveFileURIPath),
	// instead of leaving mixins on a broken relative path that only component sources
	// (via handleLocalFileScheme) previously normalized correctly.
	if resolved, ok := resolveFileURIPath(uri); ok {
		uri = resolved
	}

	pType := pkgTypeRemote
	// Check if `uri` uses the `oci://` scheme (to download the sources from an OCI-compatible registry).
	useOciScheme := false
	if strings.HasPrefix(uri, ociScheme) {
		useOciScheme = true
		pType = pkgTypeOci
		uri = strings.TrimPrefix(uri, ociScheme)
	}

	// Check if `uri` is a file path.
	// If it's a file path, check if it's an absolute path.
	// If it's not absolute path, join it with the base path (component dir) and convert to absolute path.
	if !useOciScheme {
		if absPath, joinErr := u.JoinPathAndValidate(componentPath, uri); joinErr == nil {
			uri = absPath
		}
	}
	// Check if it's a local file already present at the destination; if so, there's nothing to fetch.
	if absPath, joinErr := u.JoinPathAndValidate(componentPath, uri); joinErr == nil && u.FileExists(absPath) {
		return pkgComponentVendor{}, true, nil
	}

	return pkgComponentVendor{
		uri:                 uri,
		pkgType:             pType,
		name:                "mixin " + uri,
		sourceIsLocalFile:   false,
		IsMixins:            true,
		vendorComponentSpec: vendorComponentSpec,
		version:             mixin.Version,
		componentPath:       componentPath,
		mixinFilename:       mixin.Filename,
	}, false, nil
}

func parseMixinURI(atmosConfig *schema.AtmosConfiguration, mixin *schema.VendorComponentMixins) (string, error) {
	if mixin.Version == "" {
		return mixin.Uri, nil
	}

	uri, err := ProcessTmpl(atmosConfig, "mixin-uri", mixin.Uri, mixin, false)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithContext("filename", mixin.Filename).
			WithContext("uri", mixin.Uri).
			Err()
	}

	return uri, nil
}
