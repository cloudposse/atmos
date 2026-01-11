package exec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// DescribeLocalsArgs holds the arguments for the describe locals command.
type DescribeLocalsArgs struct {
	Component     string
	Query         string
	FilterByStack string
	Format        string
	File          string
}

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_describe_locals.go -package=$GOPACKAGE

// DescribeLocalsExec defines the interface for executing describe locals.
type DescribeLocalsExec interface {
	Execute(atmosConfig *schema.AtmosConfiguration, args *DescribeLocalsArgs) error
}

type describeLocalsExec struct {
	pageCreator              pager.PageCreator
	isTTYSupportForStdout    func() bool
	printOrWriteToFile       func(atmosConfig *schema.AtmosConfiguration, format string, file string, data any) error
	executeDescribeLocals    func(atmosConfig *schema.AtmosConfiguration, filterByStack string) (map[string]any, error)
	executeDescribeComponent func(params *ExecuteDescribeComponentParams) (map[string]any, error)
}

// NewDescribeLocalsExec creates a new DescribeLocalsExec instance.
func NewDescribeLocalsExec() DescribeLocalsExec {
	defer perf.Track(nil, "exec.NewDescribeLocalsExec")()

	return &describeLocalsExec{
		pageCreator:              pager.New(),
		isTTYSupportForStdout:    term.IsTTYSupportForStdout,
		printOrWriteToFile:       printOrWriteToFile,
		executeDescribeLocals:    ExecuteDescribeLocals,
		executeDescribeComponent: ExecuteDescribeComponent,
	}
}

// Execute executes the describe locals command.
func (d *describeLocalsExec) Execute(atmosConfig *schema.AtmosConfiguration, args *DescribeLocalsArgs) error {
	defer perf.Track(atmosConfig, "exec.DescribeLocalsExec.Execute")()

	var res any
	var err error

	// If component is specified, get locals for that specific component.
	if args.Component != "" {
		res, err = d.executeForComponent(atmosConfig, args)
		if err != nil {
			return err
		}
	} else {
		// Get locals for all stacks.
		finalLocalsMap, err := d.executeDescribeLocals(atmosConfig, args.FilterByStack)
		if err != nil {
			return err
		}
		res = finalLocalsMap
	}

	// Apply query if specified.
	if args.Query != "" {
		res, err = u.EvaluateYqExpression(atmosConfig, res, args.Query)
		if err != nil {
			return err
		}
	}

	return viewWithScroll(&viewWithScrollProps{
		pageCreator:           d.pageCreator,
		isTTYSupportForStdout: d.isTTYSupportForStdout,
		printOrWriteToFile:    d.printOrWriteToFile,
		atmosConfig:           atmosConfig,
		displayName:           "Locals",
		format:                args.Format,
		file:                  args.File,
		res:                   res,
	})
}

// executeForComponent gets the locals for a specific component in a stack.
// Component-level locals are merged with stack-level locals (component locals take precedence).
func (d *describeLocalsExec) executeForComponent(
	atmosConfig *schema.AtmosConfiguration,
	args *DescribeLocalsArgs,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.DescribeLocalsExec.executeForComponent")()

	if args.FilterByStack == "" {
		return nil, errUtils.ErrStackRequiredWithComponent
	}

	componentSection, err := d.executeDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            args.Component,
		Stack:                args.FilterByStack,
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe component %s in stack %s: %w", args.Component, args.FilterByStack, err)
	}

	componentType := getComponentType(componentSection)
	componentLocals := extractComponentLocals(componentSection)

	stackLocals, err := d.executeDescribeLocals(atmosConfig, args.FilterByStack)
	if err != nil {
		return nil, err
	}

	return buildComponentLocalsResult(args, stackLocals, componentType, componentLocals)
}

// getComponentType extracts the component type from a component section, defaulting to terraform.
func getComponentType(componentSection map[string]any) string {
	if ct, ok := componentSection["component_type"].(string); ok && ct != "" {
		return ct
	}
	return "terraform"
}

// extractComponentLocals extracts the locals section from a component section.
func extractComponentLocals(componentSection map[string]any) map[string]any {
	if cl, ok := componentSection[cfg.LocalsSectionName].(map[string]any); ok {
		return cl
	}
	return nil
}

// buildComponentLocalsResult builds the result map for component locals query.
// Output format matches Atmos stack manifest schema:
//
//	components:
//	  terraform:
//	    vpc:
//	      locals:
//	        foo: 123
func buildComponentLocalsResult(
	args *DescribeLocalsArgs,
	stackLocals map[string]any,
	componentType string,
	componentLocals map[string]any,
) (map[string]any, error) {
	// ExecuteDescribeLocals already filters by stack, so stackLocals should have at most one entry.
	// The key may be the logical stack name (e.g., "prod-us-west-2") which differs from
	// args.FilterByStack (e.g., "deploy/prod"). Try direct lookup first, then fall back to
	// using the single entry if present.
	var localsMap map[string]any

	if localsData, exists := stackLocals[args.FilterByStack]; exists {
		localsMap, _ = localsData.(map[string]any)
	} else if len(stackLocals) == 1 {
		// Single entry after filtering - use it regardless of key name.
		for _, localsData := range stackLocals {
			localsMap, _ = localsData.(map[string]any)
			break
		}
	}

	if localsMap != nil {
		stackTypeLocals := getLocalsForComponentType(localsMap, componentType)
		mergedLocals := mergeLocals(stackTypeLocals, componentLocals)
		return buildComponentSchemaOutput(args.Component, componentType, mergedLocals), nil
	}

	if len(componentLocals) > 0 {
		return buildComponentSchemaOutput(args.Component, componentType, componentLocals), nil
	}

	return nil, fmt.Errorf("%w: %s", errUtils.ErrStackHasNoLocals, args.FilterByStack)
}

// buildComponentSchemaOutput creates the Atmos schema-compliant output for component locals.
func buildComponentSchemaOutput(component, componentType string, locals map[string]any) map[string]any {
	return map[string]any{
		"components": map[string]any{
			componentType: map[string]any{
				component: map[string]any{
					cfg.LocalsSectionName: locals,
				},
			},
		},
	}
}

// mergeLocals deep-merges base and override locals maps.
// This uses the same deep-merge semantics as vars, settings, and other Atmos sections,
// so nested maps (e.g., tags: {env: dev, team: platform}) are recursively merged
// rather than entirely replaced.
func mergeLocals(base, override map[string]any) map[string]any {
	if base == nil {
		base = make(map[string]any)
	}
	if override == nil {
		return base
	}

	// Use pkg/merge for consistent deep-merge behavior with the rest of Atmos.
	// MergeWithOptions handles deep copying internally to avoid pointer mutation.
	result, err := m.MergeWithOptions(nil, []map[string]any{base, override}, false, false)
	if err != nil {
		// On merge error, fall back to shallow merge for robustness.
		log.Warn("Deep-merge failed, falling back to shallow merge", "error", err)
		result = make(map[string]any, len(base)+len(override))
		for k, v := range base {
			result[k] = v
		}
		for k, v := range override {
			result[k] = v
		}
	}

	return result
}

// getLocalsForComponentType extracts the appropriate merged locals for a component type.
// Input format is Atmos schema: locals: {...}, terraform: {locals: {...}}, etc.
func getLocalsForComponentType(stackLocals map[string]any, componentType string) map[string]any {
	result := make(map[string]any)

	// Start with global locals (root-level "locals:" key).
	if globalLocals, ok := stackLocals[cfg.LocalsSectionName].(map[string]any); ok {
		for k, v := range globalLocals {
			result[k] = v
		}
	}

	// Merge section-specific locals (e.g., "terraform: locals:").
	if sectionMap, ok := stackLocals[componentType].(map[string]any); ok {
		if sectionLocals, ok := sectionMap[cfg.LocalsSectionName].(map[string]any); ok {
			for k, v := range sectionLocals {
				result[k] = v
			}
		}
	}

	return result
}

// stackFileLocalsResult holds the result of processing a stack file for locals.
type stackFileLocalsResult struct {
	StackName   string         // Derived stack name (empty if filtered out or unparseable).
	StackLocals map[string]any // Locals extracted from the stack file.
	Found       bool           // Whether the stack matched the filter (even if no locals).
}

// validateFilteredLocalsResult checks if the filtered result is valid.
// Returns an error if filtering was requested but no stack was found or stack has no locals.
func validateFilteredLocalsResult(filterByStack string, stackFound bool, localsMap map[string]any) error {
	if filterByStack == "" {
		return nil
	}
	if !stackFound {
		return fmt.Errorf("%w: %s", errUtils.ErrStackNotFound, filterByStack)
	}
	if len(localsMap) == 0 {
		return fmt.Errorf("%w: %s", errUtils.ErrStackHasNoLocals, filterByStack)
	}
	return nil
}

// ExecuteDescribeLocals processes stack manifests and returns the locals for all stacks.
// It reads the raw YAML files directly since locals are stripped during normal stack processing.
func ExecuteDescribeLocals(
	atmosConfig *schema.AtmosConfiguration,
	filterByStack string,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.ExecuteDescribeLocals")()

	// Normalize path separators for Windows compatibility.
	// deriveStackFileName returns forward-slash paths, so we need to match that format.
	filterByStack = filepath.ToSlash(filterByStack)

	finalLocalsMap := make(map[string]any)
	stackFound := false

	// Process each stack config file directly.
	for _, filePath := range atmosConfig.StackConfigFilesAbsolutePaths {
		result, err := processStackFileForLocals(atmosConfig, filePath, filterByStack)
		if err != nil {
			return nil, err
		}

		// Track if we found a matching stack (even with no locals).
		if result.Found {
			stackFound = true
		}

		// Skip if no locals or filtered out.
		if result.StackName == "" || len(result.StackLocals) == 0 {
			continue
		}

		finalLocalsMap[result.StackName] = result.StackLocals
	}

	// Validate the result when filtering.
	if err := validateFilteredLocalsResult(filterByStack, stackFound, finalLocalsMap); err != nil {
		return nil, err
	}

	return finalLocalsMap, nil
}

// parseStackFileYAML reads and parses a stack file's YAML content.
// Returns the raw config, or nil if the file should be skipped.
// If filterMatchesFileName is true, parse errors return an error; otherwise they are logged and skipped.
func parseStackFileYAML(filePath string, filterMatchesFileName bool) (map[string]any, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, errors.Join(errUtils.ErrInvalidStackManifest, fmt.Errorf("failed to read stack file %s: %w", filePath, err))
	}

	var rawConfig map[string]any
	if err := yaml.Unmarshal(content, &rawConfig); err != nil {
		if filterMatchesFileName {
			return nil, errors.Join(errUtils.ErrInvalidStackManifest, fmt.Errorf("failed to parse YAML in %s: %w", filePath, err))
		}
		log.Warn("Skipping file with YAML parse error", "file", filePath, "error", err)
		return nil, nil //nolint:nilnil // nil config signals skip without error
	}

	return rawConfig, nil
}

// stackMatchesFilter checks if a stack matches the filter criteria.
// Returns true if no filter is specified or if the filter matches either the filename or derived name.
func stackMatchesFilter(filterByStack, stackFileName, stackName string) bool {
	if filterByStack == "" {
		return true
	}
	return filterByStack == stackFileName || filterByStack == stackName
}

// processStackFileForLocals reads a stack file and extracts its locals.
// Returns a result struct with stack name, locals, and whether the stack matched the filter.
func processStackFileForLocals(
	atmosConfig *schema.AtmosConfiguration,
	filePath string,
	filterByStack string,
) (*stackFileLocalsResult, error) {
	stackFileName := deriveStackFileName(atmosConfig, filePath)
	filterMatchesFileName := filterByStack != "" && filterByStack == stackFileName

	rawConfig, err := parseStackFileYAML(filePath, filterMatchesFileName)
	if err != nil {
		return nil, err
	}
	if rawConfig == nil {
		return &stackFileLocalsResult{}, nil
	}

	// Extract vars for stack name derivation.
	var varsSection map[string]any
	if vs, ok := rawConfig[cfg.VarsSectionName].(map[string]any); ok {
		varsSection = vs
	}

	stackName := deriveStackName(atmosConfig, stackFileName, varsSection, rawConfig)

	// Apply filter if specified.
	if !stackMatchesFilter(filterByStack, stackFileName, stackName) {
		return &stackFileLocalsResult{}, nil
	}

	localsCtx, err := ProcessStackLocals(atmosConfig, rawConfig, filePath)
	if err != nil {
		return &stackFileLocalsResult{Found: true}, fmt.Errorf("failed to process locals for stack %s: %w", stackFileName, err)
	}

	return &stackFileLocalsResult{
		StackName:   stackName,
		StackLocals: buildStackLocalsFromContext(localsCtx),
		Found:       true,
	}, nil
}

// buildStackLocalsFromContext converts a LocalsContext to a map using Atmos schema format.
// Returns an empty map if localsCtx is nil or has no locals.
// Output format matches Atmos stack manifest schema:
//
//	locals:
//	  foo: 123
//	terraform:
//	  locals:
//	    xyz: 123
func buildStackLocalsFromContext(localsCtx *LocalsContext) map[string]any {
	stackLocals := make(map[string]any)

	if localsCtx == nil {
		return stackLocals
	}

	// Global locals go under root "locals:" key.
	if len(localsCtx.Global) > 0 {
		stackLocals[cfg.LocalsSectionName] = localsCtx.Global
	}

	// Section-specific locals go under "terraform: locals:", "helmfile: locals:", etc.
	if localsCtx.HasTerraformLocals && len(localsCtx.Terraform) > 0 {
		stackLocals[cfg.TerraformSectionName] = map[string]any{
			cfg.LocalsSectionName: getSectionOnlyLocals(localsCtx.Terraform, localsCtx.Global),
		}
	}

	if localsCtx.HasHelmfileLocals && len(localsCtx.Helmfile) > 0 {
		stackLocals[cfg.HelmfileSectionName] = map[string]any{
			cfg.LocalsSectionName: getSectionOnlyLocals(localsCtx.Helmfile, localsCtx.Global),
		}
	}

	if localsCtx.HasPackerLocals && len(localsCtx.Packer) > 0 {
		stackLocals[cfg.PackerSectionName] = map[string]any{
			cfg.LocalsSectionName: getSectionOnlyLocals(localsCtx.Packer, localsCtx.Global),
		}
	}

	return stackLocals
}

// getSectionOnlyLocals extracts locals that are unique to a section (not inherited from global).
// Since section locals are already merged with global, we need to extract only the section-specific ones.
func getSectionOnlyLocals(sectionLocals, globalLocals map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range sectionLocals {
		// Include if key doesn't exist in global, or if value differs from global.
		if globalVal, exists := globalLocals[k]; !exists || !valuesEqual(v, globalVal) {
			result[k] = v
		}
	}
	return result
}

// valuesEqual compares two values for deep equality.
func valuesEqual(a, b any) bool {
	return reflect.DeepEqual(a, b)
}

// deriveStackFileName extracts the stack file name from the absolute path.
// It removes the stacks base path and file extension to get the relative stack name.
// The returned path always uses forward slashes for consistency across platforms.
func deriveStackFileName(atmosConfig *schema.AtmosConfiguration, filePath string) string {
	defer perf.Track(atmosConfig, "exec.deriveStackFileName")()

	// Get the relative path from the stacks base path.
	stacksBasePath := atmosConfig.StacksBaseAbsolutePath
	if stacksBasePath == "" {
		// Fallback: just use the file name without extension.
		base := filepath.Base(filePath)
		return strings.TrimSuffix(base, filepath.Ext(base))
	}

	// Get relative path.
	relPath, err := filepath.Rel(stacksBasePath, filePath)
	if err != nil {
		// Fallback: just use the file name without extension.
		base := filepath.Base(filePath)
		return strings.TrimSuffix(base, filepath.Ext(base))
	}

	// Remove the extension and normalize path separators to forward slashes.
	result := strings.TrimSuffix(relPath, filepath.Ext(relPath))
	return filepath.ToSlash(result)
}

// deriveStackName derives the stack name using the same logic as describe stacks.
func deriveStackName(
	atmosConfig *schema.AtmosConfiguration,
	stackFileName string,
	varsSection map[string]any,
	stackSectionMap map[string]any,
) string {
	defer perf.Track(atmosConfig, "exec.deriveStackName")()

	// Try explicit name from manifest first.
	if name := getExplicitStackName(stackSectionMap); name != "" {
		return name
	}

	// Try name template.
	if name := deriveStackNameFromTemplate(atmosConfig, stackFileName, varsSection); name != "" {
		return name
	}

	// Try name pattern.
	if name := deriveStackNameFromPattern(atmosConfig, stackFileName, varsSection); name != "" {
		return name
	}

	// Default: use stack filename.
	return stackFileName
}

// getExplicitStackName extracts an explicit name from the stack manifest if defined.
func getExplicitStackName(stackSectionMap map[string]any) string {
	nameValue, ok := stackSectionMap[cfg.NameSectionName]
	if !ok {
		return ""
	}
	name, ok := nameValue.(string)
	if !ok || name == "" {
		return ""
	}
	return name
}

// deriveStackNameFromTemplate derives a stack name using the configured name template.
// Returns empty string if template is not configured or evaluation fails.
func deriveStackNameFromTemplate(
	atmosConfig *schema.AtmosConfiguration,
	stackFileName string,
	varsSection map[string]any,
) string {
	if atmosConfig.Stacks.NameTemplate == "" {
		return ""
	}

	// Wrap varsSection in "vars" key to match template syntax: {{ .vars.environment }}.
	templateData := map[string]any{
		"vars": varsSection,
	}

	stackName, err := ProcessTmpl(atmosConfig, "describe-locals-name-template", atmosConfig.Stacks.NameTemplate, templateData, false)
	if err != nil {
		log.Debug("Failed to evaluate name template for stack", "file", stackFileName, "error", err)
		return ""
	}

	if stackName == "" {
		return ""
	}

	// If vars contain unresolved templates (e.g., "{{ .locals.* }}"), the result
	// will contain raw template markers. Fall back to empty (use filename).
	if strings.Contains(stackName, "{{") || strings.Contains(stackName, "}}") {
		log.Debug("Name template result contains unresolved templates, using filename", "file", stackFileName, "result", stackName)
		return ""
	}

	return stackName
}

// deriveStackNameFromPattern derives a stack name using the configured name pattern.
// Returns empty string if pattern is not configured or evaluation fails.
func deriveStackNameFromPattern(
	atmosConfig *schema.AtmosConfiguration,
	stackFileName string,
	varsSection map[string]any,
) string {
	pattern := GetStackNamePattern(atmosConfig)
	if pattern == "" {
		return ""
	}

	context := cfg.GetContextFromVars(varsSection)
	stackName, err := cfg.GetContextPrefix(stackFileName, context, pattern, stackFileName)
	if err != nil {
		log.Debug("Failed to evaluate name pattern for stack", "file", stackFileName, "error", err)
		return ""
	}

	return stackName
}
