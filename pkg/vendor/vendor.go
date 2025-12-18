package vendor

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/samber/lo"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"

	"github.com/cloudposse/atmos/internal/exec"
)

// Pull vendors dependencies based on options.
func Pull(atmosConfig *schema.AtmosConfiguration, opts ...PullOption) error {
	defer perf.Track(atmosConfig, "vendor.Pull")()

	options := &PullOptions{
		ComponentType: "terraform", // default
	}
	for _, opt := range opts {
		opt(options)
	}

	// Route to appropriate handler based on options
	if options.Stack != "" {
		return VendorStack(atmosConfig, options.Stack, WithStackDryRun(options.DryRun))
	}

	// First, check if vendor.yaml exists - it takes precedence
	vendorConfig, vendorConfigExists, foundVendorConfigFile, err := ReadAndProcessVendorConfigFile(
		atmosConfig,
		cfg.AtmosVendorConfigFileName,
		true,
	)
	if err != nil {
		return err
	}

	// If vendor.yaml exists, use it (with optional component/tags filtering)
	if vendorConfigExists {
		return executeAtmosVendorInternal(&executeVendorOptions{
			vendorConfigFileName: foundVendorConfigFile,
			dryRun:               options.DryRun,
			atmosConfig:          atmosConfig,
			atmosVendorSpec:      vendorConfig.Spec,
			component:            options.Component,
			tags:                 options.Tags,
		})
	}

	// No vendor.yaml - if component specified, try component.yaml
	if options.Component != "" {
		return VendorComponent(atmosConfig, options.Component,
			WithComponentDryRun(options.DryRun),
			WithComponentComponentType(options.ComponentType),
		)
	}

	// No vendor.yaml and no component specified - error
	return fmt.Errorf("%w: %s", ErrVendorConfigNotExist, cfg.AtmosVendorConfigFileName)
}

// VendorComponent vendors a single component.
func VendorComponent(atmosConfig *schema.AtmosConfiguration, component string, opts ...ComponentOption) error {
	defer perf.Track(atmosConfig, "vendor.VendorComponent")()

	options := &ComponentOptions{
		ComponentType: "terraform",
	}
	for _, opt := range opts {
		opt(options)
	}

	config, path, err := ReadAndProcessComponentVendorConfigFile(
		atmosConfig,
		component,
		options.ComponentType,
	)
	if err != nil {
		return err
	}

	return ExecuteComponentVendorInternal(
		atmosConfig,
		&config.Spec,
		component,
		path,
		options.DryRun,
	)
}

// VendorStack vendors all components in a stack.
func VendorStack(atmosConfig *schema.AtmosConfiguration, stack string, opts ...StackOption) error {
	defer perf.Track(atmosConfig, "vendor.VendorStack")()

	options := &StackOptions{}
	for _, opt := range opts {
		opt(options)
	}

	return executeStackVendorInternal(atmosConfig, stack, options.DryRun)
}

// executeAtmosVendorInternal downloads the artifacts from the sources and writes them to the targets.
func executeAtmosVendorInternal(params *executeVendorOptions) error {
	defer perf.Track(nil, "vendor.executeAtmosVendorInternal")()

	var err error
	vendorConfigFilePath := filepath.Dir(params.vendorConfigFileName)

	logInitialMessage(params.vendorConfigFileName, params.tags)
	if len(params.atmosVendorSpec.Sources) == 0 && len(params.atmosVendorSpec.Imports) == 0 {
		return fmt.Errorf("%w '%s'", ErrMissingVendorConfigDefinition, params.vendorConfigFileName)
	}
	// Process imports and return all sources from all the imports and from `vendor.yaml`.
	sources, _, err := processVendorImports(
		params.atmosConfig,
		params.vendorConfigFileName,
		params.atmosVendorSpec.Imports,
		params.atmosVendorSpec.Sources,
		[]string{params.vendorConfigFileName},
	)
	if err != nil {
		return err
	}

	if len(sources) == 0 {
		return fmt.Errorf("%w %s", ErrEmptySources, params.vendorConfigFileName)
	}

	if err := validateTagsAndComponents(sources, params.vendorConfigFileName, params.component, params.tags); err != nil {
		return err
	}

	sourceParams := &vendorSourceParams{
		atmosConfig:          params.atmosConfig,
		sources:              sources,
		component:            params.component,
		tags:                 params.tags,
		vendorConfigFileName: params.vendorConfigFileName,
		vendorConfigFilePath: vendorConfigFilePath,
	}
	packages, err := processAtmosVendorSource(sourceParams)
	if err != nil {
		return err
	}
	if len(packages) > 0 {
		return executeVendorModel(packages, params.dryRun, params.atmosConfig)
	}
	return nil
}

// processAtmosVendorSource processes vendor sources and returns packages.
func processAtmosVendorSource(params *vendorSourceParams) ([]pkgAtmosVendor, error) {
	var packages []pkgAtmosVendor
	for indexSource := range params.sources {
		if shouldSkipSource(&params.sources[indexSource], params.component, params.tags) {
			continue
		}

		if err := validateSourceFields(&params.sources[indexSource], params.vendorConfigFileName); err != nil {
			return nil, err
		}

		tmplData := struct {
			Component string
			Version   string
		}{params.sources[indexSource].Component, params.sources[indexSource].Version}

		// Parse 'source' template
		uri, err := exec.ProcessTmpl(params.atmosConfig, fmt.Sprintf("source-%d", indexSource), params.sources[indexSource].Source, tmplData, false)
		if err != nil {
			return nil, err
		}

		// Normalize the URI to handle triple-slash pattern (///), which indicates cloning from
		// the root of the repository. This pattern broke in go-getter v1.7.9 due to CVE-2025-8959
		// security fixes.
		uri = normalizeVendorURI(uri)

		useOciScheme, useLocalFileSystem, sourceIsLocalFile, err := determineSourceType(&uri, params.vendorConfigFilePath)
		if err != nil {
			return nil, err
		}
		if !useLocalFileSystem {
			err = u.ValidateURI(uri)
			if err != nil {
				if strings.Contains(uri, "..") {
					return nil, fmt.Errorf("invalid URI for component %s: %w: Please ensure the source is a valid local path", params.sources[indexSource].Component, err)
				}
				return nil, fmt.Errorf("invalid URI for component %s: %w", params.sources[indexSource].Component, err)
			}
		}

		// Determine package type
		pType := determinePackageType(useOciScheme, useLocalFileSystem)

		// Process each target within the source
		pkgs, err := processTargets(&processTargetsParams{
			AtmosConfig:          params.atmosConfig,
			IndexSource:          indexSource,
			Source:               &params.sources[indexSource],
			TemplateData:         tmplData,
			VendorConfigFilePath: params.vendorConfigFilePath,
			URI:                  uri,
			PkgType:              pType,
			SourceIsLocalFile:    sourceIsLocalFile,
		})
		if err != nil {
			return nil, err
		}
		packages = append(packages, pkgs...)
	}

	return packages, nil
}

// processTargets processes targets for a source.
func processTargets(params *processTargetsParams) ([]pkgAtmosVendor, error) {
	var packages []pkgAtmosVendor
	for indexTarget, tgt := range params.Source.Targets {
		target, err := exec.ProcessTmpl(params.AtmosConfig, fmt.Sprintf("target-%d-%d", params.IndexSource, indexTarget), tgt, params.TemplateData, false)
		if err != nil {
			return nil, err
		}
		targetPath := filepath.Join(filepath.ToSlash(params.VendorConfigFilePath), filepath.ToSlash(target))
		pkgName := params.Source.Component
		if pkgName == "" {
			pkgName = params.URI
		}
		// Create package struct
		p := pkgAtmosVendor{
			uri:               params.URI,
			name:              pkgName,
			targetPath:        targetPath,
			sourceIsLocalFile: params.SourceIsLocalFile,
			pkgType:           params.PkgType,
			version:           params.Source.Version,
			atmosVendorSource: *params.Source,
		}
		packages = append(packages, p)
	}
	return packages, nil
}

// determineSourceType determines the source type from a URI.
func determineSourceType(uri *string, vendorConfigFilePath string) (bool, bool, bool, error) {
	// Determine if the URI is an OCI scheme, a local file, or remote
	useLocalFileSystem := false
	sourceIsLocalFile := false
	useOciScheme := strings.HasPrefix(*uri, "oci://")
	if useOciScheme {
		*uri = strings.TrimPrefix(*uri, "oci://")
		return useOciScheme, useLocalFileSystem, sourceIsLocalFile, nil
	}

	absPath, err := u.JoinPathAndValidate(filepath.ToSlash(vendorConfigFilePath), *uri)
	// if URI contain path traversal is path should be resolved
	if err != nil && strings.Contains(*uri, "..") && !strings.HasPrefix(*uri, "file://") {
		return useOciScheme, useLocalFileSystem, sourceIsLocalFile, fmt.Errorf("invalid source path '%s': %w", *uri, err)
	}
	if err == nil {
		uri = &absPath
		useLocalFileSystem = true
		sourceIsLocalFile = u.FileExists(*uri)
	}

	if parsedURL, parseErr := url.Parse(*uri); parseErr == nil {
		if parsedURL.Scheme != "" {
			if parsedURL.Scheme == "file" {
				trimmedPath := strings.TrimPrefix(filepath.ToSlash(parsedURL.Path), "/")
				*uri = filepath.Clean(trimmedPath)
				useLocalFileSystem = true
			}
		}
	}

	return useOciScheme, useLocalFileSystem, sourceIsLocalFile, nil
}

// ValidateFlags validates vendor command flags.
func ValidateFlags(component, stack string, tags []string, everything bool) error {
	if component != "" && stack != "" {
		return ErrValidateComponentStackFlag
	}

	if component != "" && len(tags) > 0 {
		return ErrValidateComponentFlag
	}

	if everything && (component != "" || stack != "" || len(tags) > 0) {
		return ErrValidateEverythingFlag
	}

	return nil
}

// ShouldSetEverythingDefault determines if --everything should default to true.
func ShouldSetEverythingDefault(component, stack string, tags []string) bool {
	return component == "" && stack == "" && len(tags) == 0
}

// Diff returns an error since vendor diff is not implemented.
func Diff() error {
	return ErrExecuteVendorDiffCmd
}

// HandleVendorConfigNotExist handles the case when the vendor config doesn't exist but --component is specified.
func HandleVendorConfigNotExist(atmosConfig *schema.AtmosConfiguration, component, componentType string, dryRun bool) error {
	if componentType == "" {
		componentType = "terraform"
	}

	config, path, err := ReadAndProcessComponentVendorConfigFile(
		atmosConfig,
		component,
		componentType,
	)
	if err != nil {
		return err
	}

	return ExecuteComponentVendorInternal(
		atmosConfig,
		&config.Spec,
		component,
		path,
		dryRun,
	)
}

// FilterByTags returns sources that match the given tags.
func FilterByTags(sources []schema.AtmosVendorSource, tags []string) []schema.AtmosVendorSource {
	if len(tags) == 0 {
		return sources
	}

	return lo.Filter(sources, func(s schema.AtmosVendorSource, _ int) bool {
		return len(lo.Intersect(tags, s.Tags)) > 0
	})
}
