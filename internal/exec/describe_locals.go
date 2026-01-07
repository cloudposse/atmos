package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
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
func (d *describeLocalsExec) executeForComponent(
	atmosConfig *schema.AtmosConfiguration,
	args *DescribeLocalsArgs,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.DescribeLocalsExec.executeForComponent")()

	// Stack is required when component is specified.
	if args.FilterByStack == "" {
		return nil, errUtils.ErrStackRequiredWithComponent
	}

	// Use ExecuteDescribeComponent to get component info (including type).
	componentSection, err := d.executeDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            args.Component,
		Stack:                args.FilterByStack,
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe component %s in stack %s: %w", args.Component, args.FilterByStack, err)
	}

	// Get the component type from the component section.
	componentType := "terraform" // Default to terraform.
	if ct, ok := componentSection["component_type"].(string); ok && ct != "" {
		componentType = ct
	}

	// Now get the locals for the stack and return the merged locals for this component type.
	stackLocals, err := d.executeDescribeLocals(atmosConfig, args.FilterByStack)
	if err != nil {
		return nil, err
	}

	// Find the stack in the results.
	for stackName, localsData := range stackLocals {
		if localsMap, ok := localsData.(map[string]any); ok {
			// Get the component-type-specific merged locals.
			result := getLocalsForComponentType(localsMap, componentType)
			return map[string]any{
				"component":      args.Component,
				"stack":          stackName,
				"component_type": componentType,
				"locals":         result,
			}, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", errUtils.ErrStackHasNoLocals, args.FilterByStack)
}

// getLocalsForComponentType extracts the appropriate merged locals for a component type.
func getLocalsForComponentType(stackLocals map[string]any, componentType string) map[string]any {
	// If there are section-specific locals, use them (they already include global).
	if sectionLocals, ok := stackLocals[componentType].(map[string]any); ok {
		return sectionLocals
	}

	// Fall back to merged or global.
	if merged, ok := stackLocals["merged"].(map[string]any); ok {
		return merged
	}

	if global, ok := stackLocals["global"].(map[string]any); ok {
		return global
	}

	return map[string]any{}
}

// stackFileLocalsResult holds the result of processing a stack file for locals.
type stackFileLocalsResult struct {
	StackName   string         // Derived stack name (empty if filtered out or unparseable).
	StackLocals map[string]any // Locals extracted from the stack file.
	Found       bool           // Whether the stack matched the filter (even if no locals).
}

// ExecuteDescribeLocals processes stack manifests and returns the locals for all stacks.
// It reads the raw YAML files directly since locals are stripped during normal stack processing.
func ExecuteDescribeLocals(
	atmosConfig *schema.AtmosConfiguration,
	filterByStack string,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.ExecuteDescribeLocals")()

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

	// If filtering and no stack was found, return specific error.
	if filterByStack != "" && !stackFound {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrStackNotFound, filterByStack)
	}

	return finalLocalsMap, nil
}

// processStackFileForLocals reads a stack file and extracts its locals.
// Returns a result struct with stack name, locals, and whether the stack matched the filter.
func processStackFileForLocals(
	atmosConfig *schema.AtmosConfiguration,
	filePath string,
	filterByStack string,
) (*stackFileLocalsResult, error) {
	// Read the raw YAML file.
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read stack file %s: %w", filePath, err)
	}

	// Parse the YAML to extract structure.
	// Unmarshal errors indicate malformed YAML, log at debug level for troubleshooting.
	var rawConfig map[string]any
	if err := yaml.Unmarshal(content, &rawConfig); err != nil {
		log.Debug("Skipping file with YAML parse error", "file", filePath, "error", err)
	}

	if rawConfig == nil {
		return &stackFileLocalsResult{}, nil
	}

	// Derive stack name from the file path.
	stackFileName := deriveStackFileName(atmosConfig, filePath)

	// Extract vars for stack name derivation.
	var varsSection map[string]any
	if vs, ok := rawConfig[cfg.VarsSectionName].(map[string]any); ok {
		varsSection = vs
	}

	// Derive stack name using same logic as describe stacks.
	stackName := deriveStackName(atmosConfig, stackFileName, varsSection, rawConfig)

	// Apply filter if specified.
	if filterByStack != "" && filterByStack != stackFileName && filterByStack != stackName {
		return &stackFileLocalsResult{}, nil
	}

	// Extract locals from the raw config.
	localsCtx, err := ProcessStackLocals(atmosConfig, rawConfig, filePath)
	if err != nil {
		return &stackFileLocalsResult{Found: true}, fmt.Errorf("failed to process locals for stack %s: %w", stackFileName, err)
	}

	// Build locals entry for this stack.
	stackLocals := buildStackLocalsFromContext(localsCtx)

	return &stackFileLocalsResult{
		StackName:   stackName,
		StackLocals: stackLocals,
		Found:       true,
	}, nil
}

// buildStackLocalsFromContext converts a LocalsContext to a map suitable for output.
// Returns an empty map if localsCtx is nil or has no locals.
func buildStackLocalsFromContext(localsCtx *LocalsContext) map[string]any {
	stackLocals := make(map[string]any)

	if localsCtx == nil {
		return stackLocals
	}

	if len(localsCtx.Global) > 0 {
		stackLocals["global"] = localsCtx.Global
	}

	if localsCtx.HasTerraformLocals && len(localsCtx.Terraform) > 0 {
		stackLocals["terraform"] = localsCtx.Terraform
	}

	if localsCtx.HasHelmfileLocals && len(localsCtx.Helmfile) > 0 {
		stackLocals["helmfile"] = localsCtx.Helmfile
	}

	if localsCtx.HasPackerLocals && len(localsCtx.Packer) > 0 {
		stackLocals["packer"] = localsCtx.Packer
	}

	// Add merged view for convenience.
	merged := localsCtx.MergeForTemplateContext()
	if len(merged) > 0 {
		stackLocals["merged"] = merged
	}

	return stackLocals
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
