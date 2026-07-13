package exec

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/degradation"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ErrInvalidErrorMode is returned when a --error-mode flag value is not one of
// "strict", "warn", or "silent".
var ErrInvalidErrorMode = errors.New("invalid error mode")

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
	ErrorMode            string           // How to handle recoverable errors: "strict" (default), "warn", or "silent".
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
		authDisabled bool,
		errOptions DescribeStacksErrorOptions,
	) (map[string]any, error)
}

func NewDescribeStacksExec() DescribeStacksExec {
	defer perf.Track(nil, "exec.NewDescribeStacksExec")()

	return &describeStacksExec{
		pageCreator:           pager.New(),
		isTTYSupportForStdout: term.IsTTYSupportForStdout,
		printOrWriteToFile:    printOrWriteToFile,
		executeDescribeStacks: ExecuteDescribeStacksWithOptions,
	}
}

// Execute executes `describe stacks` command.
func (d *describeStacksExec) Execute(atmosConfig *schema.AtmosConfiguration, args *DescribeStacksArgs) error {
	defer perf.Track(atmosConfig, "exec.DescribeStacksExec.Execute")()

	errOptions, collector := ErrorOptionsFromMode(args.ErrorMode)

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
		false,
		errOptions,
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

	if err := viewWithScroll(&viewWithScrollProps{
		pageCreator:           d.pageCreator,
		isTTYSupportForStdout: d.isTTYSupportForStdout,
		printOrWriteToFile:    d.printOrWriteToFile,
		atmosConfig:           atmosConfig,
		displayName:           "Stacks",
		format:                args.Format,
		file:                  args.File,
		res:                   res,
	}); err != nil {
		return err
	}

	PrintErrorModeSummary(args.ErrorMode, collector)
	return nil
}

// OnErrorMode selects how ExecuteDescribeStacksWithOptions handles a recoverable per-value
// YAML function error (e.g. a Terraform backend that has not been provisioned yet).
type OnErrorMode string

const (
	// OnErrorStrict fails the whole describe-stacks call on the first error. This is the
	// zero value and matches the historical behavior of ExecuteDescribeStacks /
	// ExecuteDescribeStacksWithAuthDisabled.
	OnErrorStrict OnErrorMode = "strict"
	// OnErrorWarn substitutes degradation.AtmosComputedValue{} for an unresolved value
	// classified recoverable, reports it via DescribeStacksErrorOptions.OnWarning, and
	// continues processing the rest of the component/stack instead of aborting.
	OnErrorWarn OnErrorMode = "warn"
)

// DescribeStacksErrorOptions configures how ExecuteDescribeStacksWithOptions handles
// recoverable per-value YAML function errors. The zero value is OnErrorStrict, matching
// ExecuteDescribeStacks's historical fail-fast behavior.
type DescribeStacksErrorOptions struct {
	OnError   OnErrorMode
	OnWarning func(DegradationWarning)
}

// ResolveErrorMode determines the effective --error-mode value using the documented
// configuration precedence (CLI flags → ENV vars → config files → defaults, see CLAUDE.md):
// flagValue (already CLI-flag/env-var resolved by the caller's flag parser) wins if
// non-empty; otherwise settingValue (the caller's own atmos.yaml default — e.g.
// atmosConfig.List.ErrorMode for list commands, atmosConfig.Describe.ErrorMode for describe
// commands) is used if set; otherwise "warn".
//
// The error_mode setting is deliberately scoped per command family rather than shared off
// one global setting: `list` and `describe` are independent command groups that may want independent
// defaults (e.g. strict in CI-driven `describe`, warn in interactive `list`). Callers pass
// in their own section's value rather than this function reaching into a shared field, so
// that code paths shared between families (e.g. `list affected` / `describe affected`) can
// still resolve against the correct section at the call site.
//
// Callers must register their --error-mode pflag/StandardParser default as "" (not "warn")
// so an unset flag/env is distinguishable here from an explicit choice.
func ResolveErrorMode(flagValue, settingValue string) string {
	defer perf.Track(nil, "exec.ResolveErrorMode")()

	if flagValue != "" {
		return flagValue
	}
	if settingValue != "" {
		return settingValue
	}
	return string(OnErrorWarn)
}

// ErrorOptionsFromMode is the canonical conversion from a CLI --error-mode flag value
// ("strict", "warn", or "silent") to DescribeStacksErrorOptions, plus the
// degradation.Collector backing its OnWarning callback. The returned Collector is nil for
// "strict" (or any other unrecognized value), since nothing is ever degraded in that mode.
//
// "warn" and "silent" both enable lenient substitution via the same Collector.Add
// callback; they differ only in whether the caller ends up printing a summary — silent
// mode intentionally never does (see PrintErrorModeSummary), so no end-of-command warning
// is shown, while full detail remains available via --logs-level=Debug in both modes.
//
// Every command exposing --error-mode (list stacks/components/settings, describe stacks,
// describe affected, list affected, describe dependents) shares this one implementation.
// A single Collector must be reused across every ExecuteDescribeStacksWithOptions call
// within one command invocation (e.g. describe affected's HEAD-side and BASE-side calls)
// so the end-of-command summary reports one combined count, not one per call site.
func ErrorOptionsFromMode(errorMode string) (DescribeStacksErrorOptions, *degradation.Collector) {
	defer perf.Track(nil, "exec.ErrorOptionsFromMode")()

	if errorMode != string(OnErrorWarn) && errorMode != "silent" {
		return DescribeStacksErrorOptions{}, nil
	}
	collector := &degradation.Collector{}
	return DescribeStacksErrorOptions{
		OnError:   OnErrorWarn,
		OnWarning: collector.Add,
	}, collector
}

// PrintErrorModeSummary prints the collector's end-of-command summary only when errorMode
// is "warn". Safe to call with a nil collector (e.g. when errorMode is "strict"/"silent").
func PrintErrorModeSummary(errorMode string, collector *degradation.Collector) {
	defer perf.Track(nil, "exec.PrintErrorModeSummary")()

	if errorMode == string(OnErrorWarn) {
		collector.Summary()
	}
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
	return executeDescribeStacks(atmosConfig, filterByStack, components, componentTypes, sections, ignoreMissingFiles, processTemplates, processYamlFunctions, includeEmptyStacks, skip, authManager, false, DescribeStacksErrorOptions{})
}

// ExecuteDescribeStacksWithAuthDisabled processes stack manifests with auth explicitly disabled.
//
//nolint:revive // Signature intentionally mirrors ExecuteDescribeStacks with one compatibility parameter.
func ExecuteDescribeStacksWithAuthDisabled(
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
	authDisabled bool,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.ExecuteDescribeStacksWithAuthDisabled")()

	return executeDescribeStacks(atmosConfig, filterByStack, components, componentTypes, sections, ignoreMissingFiles, processTemplates, processYamlFunctions, includeEmptyStacks, skip, authManager, authDisabled, DescribeStacksErrorOptions{})
}

// ExecuteDescribeStacksWithOptions is ExecuteDescribeStacksWithAuthDisabled plus opt-in
// graceful degradation for recoverable per-value YAML function errors (see
// DescribeStacksErrorOptions). Existing callers of ExecuteDescribeStacks /
// ExecuteDescribeStacksWithAuthDisabled are unaffected — they implicitly pass
// DescribeStacksErrorOptions{} (OnErrorStrict), which reproduces the original behavior
// exactly.
//
//nolint:revive // Signature intentionally mirrors ExecuteDescribeStacksWithAuthDisabled with one added options parameter.
func ExecuteDescribeStacksWithOptions(
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
	authDisabled bool,
	errOptions DescribeStacksErrorOptions,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.ExecuteDescribeStacksWithOptions")()

	return executeDescribeStacks(atmosConfig, filterByStack, components, componentTypes, sections, ignoreMissingFiles, processTemplates, processYamlFunctions, includeEmptyStacks, skip, authManager, authDisabled, errOptions)
}

//nolint:revive // Internal wrapper preserves the existing ExecuteDescribeStacks call shape.
func executeDescribeStacks(
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
	authDisabled bool,
	errOptions DescribeStacksErrorOptions,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.ExecuteDescribeStacks")()

	stacksMap, _, err := FindStacksMap(atmosConfig, ignoreMissingFiles)
	if err != nil {
		return nil, err
	}

	processor := newDescribeStacksProcessorWithAuthDisabled(
		atmosConfig,
		filterByStack,
		components, componentTypes, sections,
		processTemplates, processYamlFunctions, includeEmptyStacks,
		skip,
		authManager,
		authDisabled,
	)
	if errOptions.OnError == OnErrorWarn {
		processor.withDegradation(errOptions.OnWarning)
	}

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
	case cfg.ContainerSectionName:
		// The typed `components.container` config (ContainerConfig) exposes no
		// base_path field, so container components always use the conventional
		// base path.
		return "components/container"
	case cfg.EmulatorSectionName:
		// Emulator components are stack-defined services, not filesystem-backed
		// component source trees. Leave component_path unset until a real source
		// path is configured.
		return ""
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
