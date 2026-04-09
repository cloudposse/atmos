package exec

import (
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// componentInfoKey is the key used for component info in stack sections.
const componentInfoKey = "component_info"

// logFieldStack is the log field key for stack names.
const logFieldStack = "stack"

type DescribeStacksArgs struct {
	Query                string
	FilterByStack        string
	Components           []string
	ComponentTypes       []string
	Sections             []string
	IgnoreMissingFiles   bool
	ProcessTemplates     bool
	ProcessYamlFunctions bool
	IncludeEmptyStacks   bool
	Skip                 []string
	Format               string
	File                 string
	AuthManager          auth.AuthManager // Optional: Auth manager for credential management (from --identity flag).
}

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type DescribeStacksExec interface {
	Execute(atmosConfig *schema.AtmosConfiguration, args *DescribeStacksArgs) error
}

type describeStacksExec struct {
	pageCreator           pager.PageCreator
	isTTYSupportForStdout func() bool
	printOrWriteToFile    func(atmosConfig *schema.AtmosConfiguration, format string, file string, data any) error
	executeDescribeStacks func(
		atmosConfig *schema.AtmosConfiguration,
		filterByStack string,
		components []string,
		componentTypes []string,
		sections []string,
		ignoreMissingFiles bool,
		processTemplates bool,
		processYamlFunctions bool,
		includeEmptyStacks bool,
		skip []string,
		authManager auth.AuthManager,
	) (map[string]any, error)
}

func NewDescribeStacksExec() DescribeStacksExec {
	defer perf.Track(nil, "exec.NewDescribeStacksExec")()

	return &describeStacksExec{
		pageCreator:           pager.New(),
		isTTYSupportForStdout: term.IsTTYSupportForStdout,
		printOrWriteToFile:    printOrWriteToFile,
		executeDescribeStacks: ExecuteDescribeStacks,
	}
}

// Execute executes `describe stacks` command.
func (d *describeStacksExec) Execute(atmosConfig *schema.AtmosConfiguration, args *DescribeStacksArgs) error {
	defer perf.Track(atmosConfig, "exec.DescribeStacksExec.Execute")()

	finalStacksMap, err := d.executeDescribeStacks(
		atmosConfig,
		args.FilterByStack,
		args.Components,
		args.ComponentTypes,
		args.Sections,
		false,
		args.ProcessTemplates,
		args.ProcessYamlFunctions,
		args.IncludeEmptyStacks,
		args.Skip,
		args.AuthManager,
	)
	if err != nil {
		return err
	}

	var res any

	if args.Query != "" {
		res, err = u.EvaluateYqExpression(atmosConfig, finalStacksMap, args.Query)
		if err != nil {
			return err
		}
	} else {
		res = finalStacksMap
	}

	return viewWithScroll(&viewWithScrollProps{
		pageCreator:           d.pageCreator,
		isTTYSupportForStdout: d.isTTYSupportForStdout,
		printOrWriteToFile:    d.printOrWriteToFile,
		atmosConfig:           atmosConfig,
		displayName:           "Stacks",
		format:                args.Format,
		file:                  args.File,
		res:                   res,
	})
}

// ExecuteDescribeStacks processes stack manifests and returns the final map of stacks and components.
func ExecuteDescribeStacks(
	atmosConfig *schema.AtmosConfiguration,
	filterByStack string,
	components []string,
	componentTypes []string,
	sections []string,
	ignoreMissingFiles bool,
	processTemplates bool,
	processYamlFunctions bool,
	includeEmptyStacks bool,
	skip []string,
	authManager auth.AuthManager,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.ExecuteDescribeStacks")()

	stacksMap, _, err := FindStacksMap(atmosConfig, ignoreMissingFiles)
	if err != nil {
		return nil, err
	}

	processor := newDescribeStacksProcessor(
		atmosConfig,
		filterByStack,
		components, componentTypes, sections,
		processTemplates, processYamlFunctions, includeEmptyStacks,
		skip,
		authManager,
	)

	for stackFileName, stackSection := range stacksMap {
		stackMap, ok := stackSection.(map[string]any)
		if !ok {
			continue
		}

		// Skip stacks without components or imports when includeEmptyStacks is false.
		if !includeEmptyStacks && !hasStackExplicitComponents(stackMap) && !hasStackImports(stackMap) {
			continue
		}

		if err := processor.processStackFile(stackFileName, stackMap); err != nil {
			return nil, err
		}
	}

	if err := filterEmptyFinalStacks(processor.finalStacksMap, processor.includeEmptyStacks); err != nil {
		return nil, err
	}

	return processor.finalStacksMap, nil
}

// getComponentBasePath returns the base path for a component kind from atmos config.
func getComponentBasePath(atmosConfig *schema.AtmosConfiguration, componentKind string) string {
	switch componentKind {
	case cfg.TerraformSectionName:
		return atmosConfig.Components.Terraform.BasePath
	case cfg.HelmfileSectionName:
		return atmosConfig.Components.Helmfile.BasePath
	case cfg.PackerSectionName:
		return atmosConfig.Components.Packer.BasePath
	case cfg.AnsibleSectionName:
		return atmosConfig.Components.Ansible.BasePath
	default:
		return ""
	}
}

// buildComponentInfo constructs the component_info map with component_path for a component.
// It uses the base component name from componentSection[cfg.ComponentSectionName] to resolve the path,
// which handles both base components and derived components correctly.
// The component_path is returned as a relative path from the project root using forward slashes.
func buildComponentInfo(atmosConfig *schema.AtmosConfiguration, componentSection map[string]any, componentKind string) map[string]any {
	defer perf.Track(atmosConfig, "exec.buildComponentInfo")()

	componentInfo := map[string]any{
		"component_type": componentKind,
	}

	// Get the actual component name to use for path resolution.
	// For derived components, this is the base component from componentSection[cfg.ComponentSectionName].
	// For base components, this is just the component name itself.
	finalComponent := ""
	if comp, ok := componentSection[cfg.ComponentSectionName].(string); ok && comp != "" {
		finalComponent = comp
	}

	if finalComponent == "" {
		return componentInfo
	}

	// Get the component folder prefix if it exists in metadata.
	componentFolderPrefix := ""
	if metadata, ok := componentSection[cfg.MetadataSectionName].(map[string]any); ok {
		if prefix, ok := metadata["component_folder_prefix"].(string); ok {
			componentFolderPrefix = strings.TrimSpace(prefix)
		}
	}

	// Build the relative component path directly from config.
	// This avoids returning absolute paths which are environment-specific.
	basePath := getComponentBasePath(atmosConfig, componentKind)
	if basePath == "" {
		return componentInfo
	}

	// Build path parts, filtering empty strings.
	parts := []string{basePath}
	if componentFolderPrefix != "" {
		parts = append(parts, componentFolderPrefix)
	}
	parts = append(parts, finalComponent)

	// Join parts and normalize to forward slashes for consistent cross-platform output.
	relativePath := filepath.ToSlash(filepath.Clean(filepath.Join(parts...)))
	componentInfo[cfg.ComponentPathSectionName] = relativePath

	return componentInfo
}

// propagateAuth populates AuthContext and AuthManager on configAndStacksInfo
// from the provided AuthManager. This bridges the auth system with per-component
// YAML function processing so that functions like !terraform.state can use
// authenticated credentials (e.g., AWS SSO).
func propagateAuth(configAndStacksInfo *schema.ConfigAndStacksInfo, authManager auth.AuthManager) {
	if authManager == nil {
		return
	}
	configAndStacksInfo.AuthManager = authManager
	managerStackInfo := authManager.GetStackInfo()
	if managerStackInfo != nil && managerStackInfo.AuthContext != nil {
		configAndStacksInfo.AuthContext = managerStackInfo.AuthContext
	}
}
