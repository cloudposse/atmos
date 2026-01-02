package exec

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/samber/lo"

	errUtils "github.com/cloudposse/atmos/errors"
	tuiTerm "github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/perf"
	p "github.com/cloudposse/atmos/pkg/provenance"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

type DescribeComponentParams struct {
	Component            string
	Stack                string
	ProcessTemplates     bool
	ProcessYamlFunctions bool
	Skip                 []string
	Query                string
	Format               string
	File                 string
	Provenance           bool
	AuthManager          auth.AuthManager // Optional: Auth manager for credential management (from --identity flag).
}

type DescribeComponentExec struct {
	pageCreator              pager.PageCreator
	printOrWriteToFile       func(atmosConfig *schema.AtmosConfiguration, format string, file string, data any) error
	IsTTYSupportForStdout    func() bool
	initCliConfig            func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error)
	executeDescribeComponent func(params *ExecuteDescribeComponentParams) (map[string]any, error)
	evaluateYqExpression     func(atmosConfig *schema.AtmosConfiguration, data any, yq string) (any, error)
}

func NewDescribeComponentExec() *DescribeComponentExec {
	defer perf.Track(nil, "exec.NewDescribeComponentExec")()

	return &DescribeComponentExec{
		printOrWriteToFile:       printOrWriteToFile,
		IsTTYSupportForStdout:    tuiTerm.IsTTYSupportForStdout,
		pageCreator:              pager.New(),
		initCliConfig:            cfg.InitCliConfig,
		executeDescribeComponent: ExecuteDescribeComponent,
		evaluateYqExpression:     u.EvaluateYqExpression,
	}
}

func (d *DescribeComponentExec) ExecuteDescribeComponentCmd(describeComponentParams DescribeComponentParams) error {
	defer perf.Track(nil, "exec.DescribeComponentExec.ExecuteDescribeComponentCmd")()

	component := describeComponentParams.Component
	stack := describeComponentParams.Stack
	processTemplates := describeComponentParams.ProcessTemplates
	processYamlFunctions := describeComponentParams.ProcessYamlFunctions
	skip := describeComponentParams.Skip
	query := describeComponentParams.Query
	format := describeComponentParams.Format
	file := describeComponentParams.File
	provenance := describeComponentParams.Provenance

	var err error
	var atmosConfig schema.AtmosConfiguration

	atmosConfig, err = d.initCliConfig(schema.ConfigAndStacksInfo{
		ComponentFromArg: component,
		Stack:            stack,
	}, true)
	if err != nil {
		return err
	}

	// Enable provenance tracking if requested.
	if provenance {
		atmosConfig.TrackProvenance = true
	}

	var componentSection map[string]any
	var mergeContext *m.MergeContext
	var stackFile string

	if provenance {
		// Use the context-aware version to get the merge context
		result, err := ExecuteDescribeComponentWithContext(DescribeComponentContextParams{
			AtmosConfig:          &atmosConfig, // Pass atmosConfig with TrackProvenance = true
			Component:            component,
			Stack:                stack,
			ProcessTemplates:     processTemplates,
			ProcessYamlFunctions: processYamlFunctions,
			Skip:                 skip,
			AuthManager:          describeComponentParams.AuthManager,
		})
		if err != nil {
			return err
		}
		componentSection = result.ComponentSection
		mergeContext = result.MergeContext
		stackFile = result.StackFile

		// Filter out computed fields when provenance is enabled
		componentSection = FilterComputedFields(componentSection)
	} else {
		// Use the standard version
		componentSection, err = d.executeDescribeComponent(&ExecuteDescribeComponentParams{
			Component:            component,
			Stack:                stack,
			ProcessTemplates:     processTemplates,
			ProcessYamlFunctions: processYamlFunctions,
			Skip:                 skip,
			AuthManager:          describeComponentParams.AuthManager,
		})
		if err != nil {
			return err
		}
	}

	var res any

	if query != "" {
		res, err = d.evaluateYqExpression(&atmosConfig, componentSection, query)
		if err != nil {
			return err
		}
	} else {
		res = componentSection
	}

	// If provenance is enabled and we have a merge context, render with inline provenance
	if provenance && mergeContext != nil && mergeContext.IsProvenanceEnabled() {
		resMap, ok := res.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: provenance rendering requires a map, got %T", errUtils.ErrInvalidComponent, res)
		}
		return d.renderProvenance(resMap, mergeContext, &atmosConfig, stackFile, file)
	}

	if atmosConfig.Settings.Terminal.IsPagerEnabled() {
		err = d.viewConfig(&atmosConfig, component, format, res)
		switch err.(type) {
		case DescribeConfigFormatError:
			return err
		case nil:
			return nil
		default:
			log.Debug("Failed to use pager")
		}
	}

	err = d.printOrWriteToFile(&atmosConfig, format, file, res)
	if err != nil {
		return err
	}

	return nil
}

func (d *DescribeComponentExec) viewConfig(atmosConfig *schema.AtmosConfiguration, displayName string, format string, data any) error {
	if !d.IsTTYSupportForStdout() {
		return ErrTTYNotSupported
	}
	var content string
	var err error
	switch format {
	case "yaml":
		content, err = u.GetHighlightedYAML(atmosConfig, data)
		if err != nil {
			return err
		}
	case "json":
		content, err = u.GetHighlightedJSON(atmosConfig, data)
		if err != nil {
			return err
		}
	default:
		return DescribeConfigFormatError{
			format,
		}
	}
	if err := d.pageCreator.Run(displayName, content); err != nil {
		return err
	}
	return nil
}

// DescribeComponentResult contains the result of describing a component.
type DescribeComponentResult struct {
	ComponentSection map[string]any
	MergeContext     *m.MergeContext
	StackFile        string // The stack manifest file being described
}

// ExecuteDescribeComponentParams contains parameters for ExecuteDescribeComponent.
type ExecuteDescribeComponentParams struct {
	Component            string
	Stack                string
	ProcessTemplates     bool
	ProcessYamlFunctions bool
	Skip                 []string
	AuthManager          auth.AuthManager
}

// ExecuteDescribeComponent describes component config.
func ExecuteDescribeComponent(params *ExecuteDescribeComponentParams) (map[string]any, error) {
	defer perf.Track(nil, "exec.ExecuteDescribeComponent")()

	result, err := ExecuteDescribeComponentWithContext(DescribeComponentContextParams{
		AtmosConfig:          nil,
		Component:            params.Component,
		Stack:                params.Stack,
		ProcessTemplates:     params.ProcessTemplates,
		ProcessYamlFunctions: params.ProcessYamlFunctions,
		Skip:                 params.Skip,
		AuthManager:          params.AuthManager,
	})
	if err != nil {
		return nil, err
	}
	return result.ComponentSection, nil
}

// writeOutputToFile writes output to a file if specified.
func writeOutputToFile(file string, output string) error {
	const filePermissions = 0o600
	if file != "" {
		return os.WriteFile(file, []byte(output), filePermissions)
	}
	return nil
}

// renderProvenance renders component configuration with provenance tracking.
func (d *DescribeComponentExec) renderProvenance(
	res map[string]any,
	mergeContext *m.MergeContext,
	atmosConfig *schema.AtmosConfiguration,
	stackFile string,
	file string,
) error {
	output := p.RenderInlineProvenanceWithStackFile(res, mergeContext, atmosConfig, stackFile)

	// Write to file if specified, otherwise print to stdout
	if file != "" {
		return writeOutputToFile(file, output)
	}

	// Print to stdout (pipeable)
	fmt.Print(output)
	return nil
}

// extractImportsList converts imports from any type to []string.
func extractImportsList(componentSection map[string]any) []string {
	// Try []any first (most common after YAML unmarshaling), then []string
	if importsAny, ok := componentSection["imports"].([]any); ok && len(importsAny) > 0 {
		imports := make([]string, 0, len(importsAny))
		for _, imp := range importsAny {
			if impStr, ok := imp.(string); ok {
				imports = append(imports, impStr)
			}
			// Skip non-string imports
		}
		return imports
	}
	if importsStr, ok := componentSection["imports"].([]string); ok && len(importsStr) > 0 {
		return importsStr
	}
	return nil
}

// tryGetProvenanceByKey tries to get provenance by exact key.
func tryGetProvenanceByKey(mergeContext *m.MergeContext, key string) *m.ProvenanceEntry {
	entries := mergeContext.GetProvenance(key)
	if len(entries) > 0 {
		return &entries[0]
	}
	return nil
}

// enhanceWithYAMLPosition enhances an entry with YAML parsing line/column info.
func enhanceWithYAMLPosition(entry *m.ProvenanceEntry, mergeContext *m.MergeContext, importPath string) {
	yamlKey := fmt.Sprintf("__import__:%s", importPath)
	if yamlEntry := tryGetProvenanceByKey(mergeContext, yamlKey); yamlEntry != nil {
		// Use line/column from YAML parsing, but keep depth from metadata.
		entry.Line = yamlEntry.Line
		entry.Column = yamlEntry.Column
		// Keep everything else from metadata (file, depth, type)
	}
}

// searchAllPaths searches all provenance paths for an import.
func searchAllPaths(mergeContext *m.MergeContext, importPath string) *m.ProvenanceEntry {
	allPaths := mergeContext.GetProvenancePaths()
	for _, path := range allPaths {
		// Check if this is a meta or yaml key for this import
		if strings.HasPrefix(path, "__import_meta__:"+importPath) ||
			strings.HasPrefix(path, "__import__:"+importPath) {
			if entry := tryGetProvenanceByKey(mergeContext, path); entry != nil {
				return entry
			}
		}
	}
	return nil
}

// findImportProvenanceEntry looks up provenance for an import path.
// It tries __import_meta__ first, then __import__, then searches all paths.
func findImportProvenanceEntry(mergeContext *m.MergeContext, importPath string) *m.ProvenanceEntry {
	// Try __import_meta__ first (has accurate depth info but placeholder line numbers).
	metaKey := fmt.Sprintf("__import_meta__:%s", importPath)
	if entry := tryGetProvenanceByKey(mergeContext, metaKey); entry != nil {
		// Found metadata from recursive import processing.
		// Try to enhance with more accurate line number from YAML parsing.
		enhanceWithYAMLPosition(entry, mergeContext, importPath)
		return entry
	}

	// Fall back to YAML parsing data if metadata isn't available.
	yamlKey := fmt.Sprintf("__import__:%s", importPath)
	if entry := tryGetProvenanceByKey(mergeContext, yamlKey); entry != nil {
		return entry
	}

	// No direct provenance found - search all paths.
	return searchAllPaths(mergeContext, importPath)
}

// recordImportsKeyProvenance records provenance for the "imports" key itself.
func recordImportsKeyProvenance(mergeContext *m.MergeContext, firstImport string) {
	const importsKey = "imports"
	firstMetaKey := fmt.Sprintf("__import_meta__:%s", firstImport)

	if entry := tryGetProvenanceByKey(mergeContext, firstMetaKey); entry != nil {
		mergeContext.RecordProvenance(importsKey, *entry)
		return
	}

	// Fall back to YAML parsing data.
	firstYAMLKey := fmt.Sprintf("__import__:%s", firstImport)
	if entry := tryGetProvenanceByKey(mergeContext, firstYAMLKey); entry != nil {
		mergeContext.RecordProvenance(importsKey, *entry)
	}
}

// recordImportsProvenance records provenance for the imports array.
func recordImportsProvenance(mergeContext *m.MergeContext, imports []string) {
	for i, importPath := range imports {
		entry := findImportProvenanceEntry(mergeContext, importPath)
		if entry == nil {
			// This import wasn't tracked - skip it.
			// This likely means it came from a deeply nested import chain.
			continue
		}

		// Record provenance for this array element in the final output.
		arrayPath := fmt.Sprintf("imports[%d]", i)
		mergeContext.RecordProvenance(arrayPath, *entry)
	}

	// Also record provenance for the "imports" key itself (using first import's metadata if available).
	if len(imports) > 0 {
		recordImportsKeyProvenance(mergeContext, imports[0])
	}
}

// DescribeComponentContextParams contains parameters for describing a component with context.
type DescribeComponentContextParams struct {
	AtmosConfig          *schema.AtmosConfiguration
	Component            string
	Stack                string
	ProcessTemplates     bool
	ProcessYamlFunctions bool
	Skip                 []string
	AuthManager          auth.AuthManager // Optional: Auth manager for credential management
}

// componentTypeProcessParams contains parameters for tryProcessWithComponentType.
type componentTypeProcessParams struct {
	atmosConfig          *schema.AtmosConfiguration
	configAndStacksInfo  schema.ConfigAndStacksInfo
	componentType        string
	processTemplates     bool
	processYamlFunctions bool
	skip                 []string
	authManager          auth.AuthManager
}

// tryProcessWithComponentType attempts to process stacks with a specific component type.
func tryProcessWithComponentType(params *componentTypeProcessParams) (schema.ConfigAndStacksInfo, error) {
	params.configAndStacksInfo.ComponentType = params.componentType
	result, err := ProcessStacks(params.atmosConfig, params.configAndStacksInfo, true, params.processTemplates, params.processYamlFunctions, params.skip, params.authManager)
	result.ComponentSection[cfg.ComponentTypeSectionName] = params.componentType
	return result, err
}

// detectComponentType tries to detect component type (Terraform, Helmfile, or Packer).
func detectComponentType(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	params DescribeComponentContextParams,
) (schema.ConfigAndStacksInfo, error) {
	baseParams := componentTypeProcessParams{
		atmosConfig:          atmosConfig,
		configAndStacksInfo:  *configAndStacksInfo,
		processTemplates:     params.ProcessTemplates,
		processYamlFunctions: params.ProcessYamlFunctions,
		skip:                 params.Skip,
		authManager:          params.AuthManager,
	}

	// Try Terraform.
	baseParams.componentType = cfg.TerraformComponentType
	result, err := tryProcessWithComponentType(&baseParams)
	if err != nil {
		// If this is NOT a "component not found" type error, don't try other component types.
		// For example, if the component has invalid HCL syntax, we should report that error
		// rather than trying Helmfile/Packer and ultimately returning "component not found".
		// This fixes https://github.com/cloudposse/atmos/issues/1864
		if !errors.Is(err, errUtils.ErrInvalidComponent) {
			return result, err
		}

		// Try Helmfile.
		baseParams.configAndStacksInfo = result
		baseParams.componentType = cfg.HelmfileComponentType
		result, err = tryProcessWithComponentType(&baseParams)
		if err != nil {
			// Same check for Helmfile errors.
			if !errors.Is(err, errUtils.ErrInvalidComponent) {
				return result, err
			}

			// Try Packer.
			baseParams.configAndStacksInfo = result
			baseParams.componentType = cfg.PackerComponentType
			result, err = tryProcessWithComponentType(&baseParams)
			if err != nil {
				result.ComponentSection[cfg.ComponentTypeSectionName] = ""
				return result, err
			}
		}
	}
	return result, nil
}

// ExecuteDescribeComponentWithContext describes component config and returns the merge context.
func ExecuteDescribeComponentWithContext(params DescribeComponentContextParams) (*DescribeComponentResult, error) {
	defer perf.Track(params.AtmosConfig, "exec.ExecuteDescribeComponentWithContext")()

	var configAndStacksInfo schema.ConfigAndStacksInfo
	configAndStacksInfo.ComponentFromArg = params.Component
	configAndStacksInfo.Stack = params.Stack
	configAndStacksInfo.CliArgs = []string{"describe", "component"}
	configAndStacksInfo.ComponentSection = make(map[string]any)

	var err error
	atmosConfig := params.AtmosConfig
	// Use provided atmosConfig or initialize a new one
	if atmosConfig == nil {
		var config schema.AtmosConfiguration
		config, err = cfg.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			return nil, err
		}
		atmosConfig = &config
	}

	// Clear any previous merge contexts before processing
	ClearMergeContexts()

	// Populate AuthContext from AuthManager if provided.
	// This enables YAML template functions (!terraform.state, !terraform.output)
	// to access authenticated credentials for S3 backends and other remote state.
	if params.AuthManager != nil {
		// Get the stack info from the auth manager which should contain
		// the populated AuthContext from the authentication process.
		managerStackInfo := params.AuthManager.GetStackInfo()
		if managerStackInfo != nil && managerStackInfo.AuthContext != nil {
			// Copy the AuthContext from the manager's stack info
			configAndStacksInfo.AuthContext = managerStackInfo.AuthContext
			log.Debug("Populated AuthContext from AuthManager for template functions")
		}
	}

	// Detect component type (Terraform, Helmfile, or Packer)
	configAndStacksInfo, err = detectComponentType(atmosConfig, &configAndStacksInfo, params)
	if err != nil {
		return nil, err
	}

	// Get the merge context for the specific stack file that was stored during processing
	var mergeContext *m.MergeContext
	if configAndStacksInfo.StackFile != "" {
		mergeContext = GetMergeContextForStack(configAndStacksInfo.StackFile)
	}
	// Fall back to the old method if the stack file is not set
	if mergeContext == nil {
		mergeContext = GetLastMergeContext()
	}

	// Record provenance for imports array if tracking is enabled.
	if atmosConfig.TrackProvenance && mergeContext != nil && mergeContext.IsProvenanceEnabled() {
		imports := extractImportsList(configAndStacksInfo.ComponentSection)
		if len(imports) > 0 {
			recordImportsProvenance(mergeContext, imports)
		}
	}

	// Apply filtering based on include_empty setting
	includeEmpty := GetIncludeEmptySetting(atmosConfig)
	filteredComponentSection := FilterEmptySections(configAndStacksInfo.ComponentSection, includeEmpty)

	return &DescribeComponentResult{
		ComponentSection: filteredComponentSection,
		MergeContext:     mergeContext,
		StackFile:        configAndStacksInfo.StackFile,
	}, nil
}

// FilterComputedFields removes Atmos-added fields that don't come from stack files.
// Only keeps fields that are defined in stack YAML files.
func FilterComputedFields(componentSection map[string]any) map[string]any {
	if componentSection == nil {
		return map[string]any{}
	}

	// Fields to keep (from stack files)
	fieldsToKeep := map[string]bool{
		"vars":         true,
		"settings":     true,
		"env":          true,
		"backend":      true,
		"metadata":     true,
		"overrides":    true,
		"providers":    true,
		"imports":      true,
		"dependencies": true,
	}

	filtered := make(map[string]any)
	for k, v := range componentSection {
		if fieldsToKeep[k] {
			filtered[k] = v
		}
	}

	return filtered
}

// FilterAbstractComponents This function removes abstract components and returns the list of components.
func FilterAbstractComponents(componentsMap map[string]any) []string {
	defer perf.Track(nil, "exec.FilterAbstractComponents")()

	if componentsMap == nil {
		return []string{}
	}
	components := make([]string, 0)
	for _, k := range lo.Keys(componentsMap) {
		componentMap, ok := componentsMap[k].(map[string]any)
		if !ok {
			components = append(components, k)
			continue
		}

		metadata, ok := componentMap["metadata"].(map[string]any)
		if !ok {
			components = append(components, k)
			continue
		}
		if componentType, ok := metadata["type"].(string); ok && componentType == "abstract" {
			continue
		}
		if componentEnabled, ok := metadata["enabled"].(bool); ok && !componentEnabled {
			continue
		}
		components = append(components, k)
	}
	return components
}
