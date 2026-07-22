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
	"github.com/cloudposse/atmos/pkg/vendoring/install"
	"github.com/cloudposse/atmos/pkg/vendoring/version"
)

// buildComponentPackagesOptions bundles buildComponentVendorPackages' inputs (Options Pattern,
// CLAUDE.md: crossed the >4-total-parameters threshold once RefreshLock/Lister joined the original
// AtmosConfig/VendorComponentSpec/Component/ComponentPath).
type buildComponentPackagesOptions struct {
	AtmosConfig         *schema.AtmosConfiguration
	VendorComponentSpec *schema.VendorComponentSpec
	Component           string
	ComponentPath       string
	// RefreshLock forces fresh semver-range version resolution -- see resolveEffectiveVersion.
	RefreshLock bool
	// Lister overrides the remote Git tag lister used to resolve a semver-range `version:`; nil in
	// every production call path -- see vendorSourceParams.lister's doc comment.
	Lister version.RemoteLister
}

// buildComponentVendorPackages resolves a single component's vendor spec (plus any mixins) into
// the package list executeVendorModel consumes, without executing the pull. Extracted from
// ExecuteComponentVendorInternal so ExecuteComponentVendorPullBatch can build package lists for
// multiple components and combine them into one executeVendorModel call.
func buildComponentVendorPackages(opts buildComponentPackagesOptions) ([]install.VendorPackage, error) {
	atmosConfig := opts.AtmosConfig
	vendorComponentSpec := opts.VendorComponentSpec
	component := opts.Component
	componentPath := opts.ComponentPath

	if vendorComponentSpec.Source.Uri == "" {
		return nil, fmt.Errorf("%w:'%s'", ErrUriMustSpecified, cfg.ComponentVendorConfigFileName)
	}
	if err := version.ValidateVersionRangeConstraints(vendorComponentSpec.Source.Version, vendorComponentSpec.Source.Constraints); err != nil {
		return nil, err
	}

	resolved, err := resolveComponentSourceURI(atmosConfig, vendorComponentSpec, component, opts)
	if err != nil {
		return nil, err
	}
	uri := resolved.URI

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
	componentPkg := install.NewComponentVendorPackage(&install.ComponentPackageParams{
		Name:              component,
		URI:               uri,
		ComponentPath:     componentPath,
		Version:           resolved.ResolvedVersion,
		RawVersion:        resolved.RawVersion,
		PkgType:           pType,
		SourceIsLocalFile: sourceIsLocalFile,
		Spec:              vendorComponentSpec,
	})

	packages := []install.VendorPackage{componentPkg}
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

// componentSourceResolution is resolveComponentSourceURI's result: the templated source URI (with
// a range-declared version: already resolved to a concrete version -- see resolveEffectiveVersion)
// plus its resolved/raw version, for building the eventual install.VendorPackage. RawVersion is
// non-empty only when the declared version was a range -- see install.VendorPackage.RawVersion.
type componentSourceResolution struct {
	URI             string
	ResolvedVersion string
	RawVersion      string
}

// resolveComponentSourceURI resolves a component.yaml source's declared `version:` (exact pin or
// semver range, via resolveEffectiveVersion) and templates the result into source.uri. Uses
// ProcessTmpl (the same helper vendor_utils.go uses for the top-level vendor.yaml source/target
// templating) so component.yaml source URIs get the same Sprig, gomplate, and Atmos template
// functions (the "atmos" namespace from internal/exec/template_funcs.go) rather than a narrower,
// hand-rolled function map.
func resolveComponentSourceURI(
	atmosConfig *schema.AtmosConfiguration,
	vendorComponentSpec *schema.VendorComponentSpec,
	component string,
	opts buildComponentPackagesOptions,
) (componentSourceResolution, error) {
	uri := vendorComponentSpec.Source.Uri

	resolvedVersion, rawVersion, err := resolveEffectiveVersion(&versionResolveInputs{
		AtmosConfig: atmosConfig,
		Name:        component,
		Source:      vendorComponentSpec.Source.Uri,
		RawVersion:  vendorComponentSpec.Source.Version,
		Constraints: vendorComponentSpec.Source.Constraints,
		RefreshLock: opts.RefreshLock,
		Lister:      opts.Lister,
	})
	if err != nil {
		return componentSourceResolution{}, fmt.Errorf("resolve version for component %q: %w", component, err)
	}

	if vendorComponentSpec.Source.Version == "" {
		return componentSourceResolution{URI: uri, ResolvedVersion: resolvedVersion, RawVersion: rawVersion}, nil
	}

	templateData := vendorComponentSpec.Source
	templateData.Version = resolvedVersion
	uri, err = ProcessTmpl(atmosConfig, fmt.Sprintf("source-uri-%s", resolvedVersion), vendorComponentSpec.Source.Uri, templateData, false)
	if err != nil {
		return componentSourceResolution{}, errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithContext("component", component).
			WithContext("uri", vendorComponentSpec.Source.Uri).
			Err()
	}
	return componentSourceResolution{URI: uri, ResolvedVersion: resolvedVersion, RawVersion: rawVersion}, nil
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

func processComponentMixins(atmosConfig *schema.AtmosConfiguration, vendorComponentSpec *schema.VendorComponentSpec, componentPath string) ([]install.VendorPackage, error) {
	var packages []install.VendorPackage
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
//
// Scope note: unlike a component's own source.version, a mixin's version is templated verbatim
// (mixin.Version below) and never passed through resolveEffectiveVersion -- a semver-range mixin
// version: is not supported, and is templated as a literal, unresolved string like any other
// non-range value (typically failing at fetch time with an invalid-ref error, not a silent
// mis-resolution).
func resolveMixinPackage(
	atmosConfig *schema.AtmosConfiguration,
	vendorComponentSpec *schema.VendorComponentSpec,
	mixin *schema.VendorComponentMixins,
	componentPath string,
) (pkg install.VendorPackage, alreadyMaterialized bool, err error) {
	// Parse 'uri' template.
	uri, err := parseMixinURI(atmosConfig, mixin)
	if err != nil {
		return install.VendorPackage{}, false, fmt.Errorf("mixin %q (uri %q): %w", mixin.Filename, mixin.Uri, err)
	}

	// Normalize `file://` URIs so they keep their absolute root (see resolveFileURIPath),
	// instead of leaving mixins on a broken relative path that only component sources
	// (via handleLocalFileScheme) previously normalized correctly.
	if resolved, ok := resolveFileURIPath(uri); ok {
		uri = resolved
	}

	pType := install.PkgTypeRemote
	// Check if `uri` uses the `oci://` scheme (to download the sources from an OCI-compatible registry).
	useOciScheme := false
	if strings.HasPrefix(uri, ociScheme) {
		useOciScheme = true
		pType = install.PkgTypeOci
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
		return install.VendorPackage{}, true, nil
	}

	return install.NewComponentVendorPackage(&install.ComponentPackageParams{
		Name:              "mixin " + uri,
		URI:               uri,
		ComponentPath:     componentPath,
		Version:           mixin.Version,
		PkgType:           pType,
		SourceIsLocalFile: false,
		Spec:              vendorComponentSpec,
		IsMixin:           true,
		MixinFilename:     mixin.Filename,
	}), false, nil
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
