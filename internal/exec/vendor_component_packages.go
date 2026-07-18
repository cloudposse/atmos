package exec

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/hairyhenderson/gomplate/v3"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// buildComponentVendorPackages resolves a single component's vendor spec (plus any mixins) into
// the package list executeVendorModel consumes, without executing the pull. Extracted from
// ExecuteComponentVendorInternal so ExecuteComponentVendorPullBatch can build package lists for
// multiple components and combine them into one executeVendorModel call.
func buildComponentVendorPackages(
	vendorComponentSpec *schema.VendorComponentSpec,
	component string,
	componentPath string,
) ([]pkgComponentVendor, error) {
	if vendorComponentSpec.Source.Uri == "" {
		return nil, fmt.Errorf("%w:'%s'", ErrUriMustSpecified, cfg.ComponentVendorConfigFileName)
	}
	uri := vendorComponentSpec.Source.Uri
	// Parse 'uri' template
	if vendorComponentSpec.Source.Version != "" {
		t, err := template.New(fmt.Sprintf("source-uri-%s", vendorComponentSpec.Source.Version)).Funcs(getSprigFuncMap()).Funcs(gomplate.CreateFuncs(context.Background(), nil)).Parse(vendorComponentSpec.Source.Uri)
		if err != nil {
			return nil, err
		}
		var tpl bytes.Buffer
		err = t.Execute(&tpl, vendorComponentSpec.Source)
		if err != nil {
			return nil, err
		}
		uri = tpl.String()
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
		mixinPkgs, err := processComponentMixins(vendorComponentSpec, componentPath)
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

	// Handle file:// scheme
	if parsedURL, err := url.Parse(uri); err == nil && parsedURL.Scheme != "" {
		if parsedURL.Scheme == "file" {
			trimmedPath := strings.TrimPrefix(filepath.ToSlash(parsedURL.Path), "/")
			uri = filepath.Clean(trimmedPath)
			useLocalFileSystem = true
		}
	}

	return uri, useLocalFileSystem, sourceIsLocalFile
}

func processComponentMixins(vendorComponentSpec *schema.VendorComponentSpec, componentPath string) ([]pkgComponentVendor, error) {
	var packages []pkgComponentVendor
	for _, mixin := range vendorComponentSpec.Mixins {
		if mixin.Uri == "" {
			return nil, ErrMissingMixinURI
		}

		if mixin.Filename == "" {
			return nil, ErrMissingMixinFilename
		}

		// Parse 'uri' template
		uri, err := parseMixinURI(&mixin)
		if err != nil {
			return nil, err
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
			if absPath, err := u.JoinPathAndValidate(componentPath, uri); err == nil {
				uri = absPath
			}
		}
		// Check if it's a local file .
		if absPath, err := u.JoinPathAndValidate(componentPath, uri); err == nil {
			if u.FileExists(absPath) {
				continue
			}
		}

		pkg := pkgComponentVendor{
			uri:                 uri,
			pkgType:             pType,
			name:                "mixin " + uri,
			sourceIsLocalFile:   false,
			IsMixins:            true,
			vendorComponentSpec: vendorComponentSpec,
			version:             mixin.Version,
			componentPath:       componentPath,
			mixinFilename:       mixin.Filename,
		}

		packages = append(packages, pkg)
	}
	return packages, nil
}

func parseMixinURI(mixin *schema.VendorComponentMixins) (string, error) {
	if mixin.Version == "" {
		return mixin.Uri, nil
	}

	tmpl, err := template.New("mixin-uri").Funcs(getSprigFuncMap()).Funcs(gomplate.CreateFuncs(context.Background(), nil)).Parse(mixin.Uri)
	if err != nil {
		return "", err
	}

	var tpl bytes.Buffer
	if err := tmpl.Execute(&tpl, mixin); err != nil {
		return "", err
	}

	return tpl.String(), nil
}
