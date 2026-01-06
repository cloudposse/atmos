package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// DescribeLocalsArgs holds the arguments for the describe locals command.
type DescribeLocalsArgs struct {
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
	pageCreator           pager.PageCreator
	isTTYSupportForStdout func() bool
	printOrWriteToFile    func(atmosConfig *schema.AtmosConfiguration, format string, file string, data any) error
	executeDescribeLocals func(
		atmosConfig *schema.AtmosConfiguration,
		filterByStack string,
	) (map[string]any, error)
}

// NewDescribeLocalsExec creates a new DescribeLocalsExec instance.
func NewDescribeLocalsExec() DescribeLocalsExec {
	defer perf.Track(nil, "exec.NewDescribeLocalsExec")()

	return &describeLocalsExec{
		pageCreator:           pager.New(),
		isTTYSupportForStdout: term.IsTTYSupportForStdout,
		printOrWriteToFile:    printOrWriteToFile,
		executeDescribeLocals: ExecuteDescribeLocals,
	}
}

// Execute executes the describe locals command.
func (d *describeLocalsExec) Execute(atmosConfig *schema.AtmosConfiguration, args *DescribeLocalsArgs) error {
	defer perf.Track(atmosConfig, "exec.DescribeLocalsExec.Execute")()

	finalLocalsMap, err := d.executeDescribeLocals(
		atmosConfig,
		args.FilterByStack,
	)
	if err != nil {
		return err
	}

	var res any

	if args.Query != "" {
		res, err = u.EvaluateYqExpression(atmosConfig, finalLocalsMap, args.Query)
		if err != nil {
			return err
		}
	} else {
		res = finalLocalsMap
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

// ExecuteDescribeLocals processes stack manifests and returns the locals for all stacks.
// It reads the raw YAML files directly since locals are stripped during normal stack processing.
func ExecuteDescribeLocals(
	atmosConfig *schema.AtmosConfiguration,
	filterByStack string,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.ExecuteDescribeLocals")()

	finalLocalsMap := make(map[string]any)

	// Process each stack config file directly.
	for _, filePath := range atmosConfig.StackConfigFilesAbsolutePaths {
		stackName, stackLocals, err := processStackFileForLocals(atmosConfig, filePath, filterByStack)
		if err != nil {
			return nil, err
		}

		// Skip if no locals or filtered out.
		if stackName == "" || len(stackLocals) == 0 {
			continue
		}

		finalLocalsMap[stackName] = stackLocals
	}

	return finalLocalsMap, nil
}

// processStackFileForLocals reads a stack file and extracts its locals.
// Returns empty stackName if the file should be skipped (filtered, unparseable, or no locals).
func processStackFileForLocals(
	atmosConfig *schema.AtmosConfiguration,
	filePath string,
	filterByStack string,
) (string, map[string]any, error) {
	// Read the raw YAML file.
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read stack file %s: %w", filePath, err)
	}

	// Parse the YAML to extract structure.
	// Unmarshal errors are expected for non-YAML files, so we skip them silently.
	var rawConfig map[string]any
	_ = yaml.Unmarshal(content, &rawConfig)

	if rawConfig == nil {
		return "", nil, nil
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
		return "", nil, nil
	}

	// Extract locals from the raw config.
	localsCtx, err := ProcessStackLocals(atmosConfig, rawConfig, filePath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to process locals for stack %s: %w", stackFileName, err)
	}

	// Build locals entry for this stack.
	stackLocals := buildStackLocalsFromContext(localsCtx)

	return stackName, stackLocals, nil
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

	// Remove the extension.
	return strings.TrimSuffix(relPath, filepath.Ext(relPath))
}

// deriveStackName derives the stack name using the same logic as describe stacks.
func deriveStackName(
	atmosConfig *schema.AtmosConfiguration,
	stackFileName string,
	varsSection map[string]any,
	stackSectionMap map[string]any,
) string {
	defer perf.Track(atmosConfig, "exec.deriveStackName")()

	// Check for explicit name in manifest.
	if nameValue, ok := stackSectionMap[cfg.NameSectionName]; ok {
		if name, ok := nameValue.(string); ok && name != "" {
			return name
		}
	}

	// Use name template if configured.
	if atmosConfig.Stacks.NameTemplate != "" {
		stackName, err := ProcessTmpl(atmosConfig, "describe-locals-name-template", atmosConfig.Stacks.NameTemplate, varsSection, false)
		if err == nil && stackName != "" {
			return stackName
		}
	}

	// Use name pattern if configured.
	if GetStackNamePattern(atmosConfig) != "" {
		context := cfg.GetContextFromVars(varsSection)
		stackName, err := cfg.GetContextPrefix(stackFileName, context, GetStackNamePattern(atmosConfig), stackFileName)
		if err == nil && stackName != "" {
			return stackName
		}
	}

	// Default: use stack filename.
	return stackFileName
}
