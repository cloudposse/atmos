package exec

import (
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
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
}

type DescribeComponentExec struct {
	pageCreator              pager.PageCreator
	printOrWriteToFile       func(atmosConfig *schema.AtmosConfiguration, format string, file string, data any) error
	IsTTYSupportForStdout    func() bool
	initCliConfig            func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error)
	executeDescribeComponent func(component string, stack string, processTemplates bool, processYamlFunctions bool, skip []string) (map[string]any, error)
	evaluateYqExpression     func(atmosConfig *schema.AtmosConfiguration, data any, yq string) (any, error)
}

func NewDescribeComponentExec() *DescribeComponentExec {
	return &DescribeComponentExec{
		printOrWriteToFile:       printOrWriteToFile,
		IsTTYSupportForStdout:    term.IsTTYSupportForStdout,
		pageCreator:              pager.New(),
		initCliConfig:            cfg.InitCliConfig,
		executeDescribeComponent: ExecuteDescribeComponent,
		evaluateYqExpression:     u.EvaluateYqExpression,
	}
}

func (d *DescribeComponentExec) ExecuteDescribeComponentCmd(describeComponentParams DescribeComponentParams) error {
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
		result, err := ExecuteDescribeComponentWithContext(
			&atmosConfig, // Pass atmosConfig with TrackProvenance = true
			component,
			stack,
			processTemplates,
			processYamlFunctions,
			skip,
		)
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
		componentSection, err = d.executeDescribeComponent(
			component,
			stack,
			processTemplates,
			processYamlFunctions,
			skip,
		)
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
		output := p.RenderInlineProvenanceWithStackFile(res, mergeContext, &atmosConfig, stackFile)
		u.PrintfMessageToTUI("%s", output)
		return nil
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

// ExecuteDescribeComponent describes component config.
func ExecuteDescribeComponent(
	component string,
	stack string,
	processTemplates bool,
	processYamlFunctions bool,
	skip []string,
) (map[string]any, error) {
	result, err := ExecuteDescribeComponentWithContext(nil, component, stack, processTemplates, processYamlFunctions, skip)
	if err != nil {
		return nil, err
	}
	return result.ComponentSection, nil
}

// ExecuteDescribeComponentWithContext describes component config and returns the merge context.
func ExecuteDescribeComponentWithContext(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	processTemplates bool,
	processYamlFunctions bool,
	skip []string,
) (*DescribeComponentResult, error) {
	defer perf.Track(atmosConfig, "exec.ExecuteDescribeComponentWithContext")()

	var configAndStacksInfo schema.ConfigAndStacksInfo
	configAndStacksInfo.ComponentFromArg = component
	configAndStacksInfo.Stack = stack
	configAndStacksInfo.CliArgs = []string{"describe", "component"}
	configAndStacksInfo.ComponentSection = make(map[string]any)

	var err error
	// Use provided atmosConfig or initialize a new one
	if atmosConfig == nil {
		var config schema.AtmosConfiguration
		config, err = cfg.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			return nil, err
		}
		atmosConfig = &config
	}

	// Clear any previous merge context before processing
	ClearLastMergeContext()

	configAndStacksInfo.ComponentType = cfg.TerraformComponentType
	configAndStacksInfo, err = ProcessStacks(atmosConfig, configAndStacksInfo, true, processTemplates, processYamlFunctions, skip)
	configAndStacksInfo.ComponentSection[cfg.ComponentTypeSectionName] = cfg.TerraformComponentType
	if err != nil {
		configAndStacksInfo.ComponentType = cfg.HelmfileComponentType
		configAndStacksInfo, err = ProcessStacks(atmosConfig, configAndStacksInfo, true, processTemplates, processYamlFunctions, skip)
		configAndStacksInfo.ComponentSection[cfg.ComponentTypeSectionName] = cfg.HelmfileComponentType
		if err != nil {
			configAndStacksInfo.ComponentType = cfg.PackerComponentType
			configAndStacksInfo, err = ProcessStacks(atmosConfig, configAndStacksInfo, true, processTemplates, processYamlFunctions, skip)
			configAndStacksInfo.ComponentSection[cfg.ComponentTypeSectionName] = cfg.PackerComponentType
			if err != nil {
				configAndStacksInfo.ComponentSection[cfg.ComponentTypeSectionName] = ""
				return nil, err
			}
		}
	}

	// Get the merge context that was stored during processing
	mergeContext := GetLastMergeContext()

	// Record provenance for imports array if tracking is enabled.
	if atmosConfig.TrackProvenance && mergeContext != nil && mergeContext.IsProvenanceEnabled() {
		// Try []any first (most common after YAML unmarshaling), then []string
		var imports []string
		if importsAny, ok := configAndStacksInfo.ComponentSection["imports"].([]any); ok && len(importsAny) > 0 {
			imports = make([]string, len(importsAny))
			for idx, imp := range importsAny {
				imports[idx] = imp.(string)
			}
		} else if importsStr, ok := configAndStacksInfo.ComponentSection["imports"].([]string); ok && len(importsStr) > 0 {
			imports = importsStr
		}

		if len(imports) > 0 {
			for i, importPath := range imports {
				// Look up metadata recorded when the import was added to importsConfig.
				// Try __import_meta__ first (has accurate depth info but placeholder line numbers).
				metaKey := fmt.Sprintf("__import_meta__:%s", importPath)
				var entry *m.ProvenanceEntry

				if entries := mergeContext.GetProvenance(metaKey); entries != nil && len(entries) > 0 {
					// Found metadata from recursive import processing.
					entry = &entries[0]

					// Try to enhance with more accurate line number from YAML parsing.
					yamlKey := fmt.Sprintf("__import__:%s", importPath)
					if yamlEntries := mergeContext.GetProvenance(yamlKey); yamlEntries != nil && len(yamlEntries) > 0 {
						// Use line/column from YAML parsing, but keep depth from metadata.
						yamlEntry := yamlEntries[0]
						entry.Line = yamlEntry.Line
						entry.Column = yamlEntry.Column
						// Keep everything else from metadata (file, depth, type)
					}
				} else {
					// Fall back to YAML parsing data if metadata isn't available.
					yamlKey := fmt.Sprintf("__import__:%s", importPath)
					if yamlEntries := mergeContext.GetProvenance(yamlKey); yamlEntries != nil && len(yamlEntries) > 0 {
						entry = &yamlEntries[0]
					} else {
						// No direct provenance found.
						// Search all provenance paths for this import to find where it came from.
						allPaths := mergeContext.GetProvenancePaths()
						found := false

						for _, path := range allPaths {
							// Check if this is a meta or yaml key for this import
							if strings.HasPrefix(path, "__import_meta__:"+importPath) ||
								strings.HasPrefix(path, "__import__:"+importPath) {
								if entries := mergeContext.GetProvenance(path); len(entries) > 0 {
									entry = &entries[0]
									found = true
									break
								}
							}
						}

						if !found {
							// Still not found - this import wasn't tracked at all.
							// This likely means it came from a deeply nested import chain.
							// Don't add provenance for it - let the renderer skip it.
							continue
						}
					}
				}

				// Record provenance for this array element in the final output.
				arrayPath := fmt.Sprintf("imports[%d]", i)
				mergeContext.RecordProvenance(arrayPath, *entry)
			}

			// Also record provenance for the "imports" key itself (using first import's metadata if available).
			if len(imports) > 0 {
				firstMetaKey := fmt.Sprintf("__import_meta__:%s", imports[0])
				if entries := mergeContext.GetProvenance(firstMetaKey); entries != nil && len(entries) > 0 {
					mergeContext.RecordProvenance("imports", entries[0])
				} else {
					// Fall back to YAML parsing data.
					firstYAMLKey := fmt.Sprintf("__import__:%s", imports[0])
					if entries := mergeContext.GetProvenance(firstYAMLKey); entries != nil && len(entries) > 0 {
						mergeContext.RecordProvenance("imports", entries[0])
					}
				}
			}
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
		"vars":      true,
		"settings":  true,
		"env":       true,
		"backend":   true,
		"metadata":  true,
		"overrides": true,
		"providers": true,
		"imports":   true,
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
