package list

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/git"
	ghactions "github.com/cloudposse/atmos/pkg/github/actions"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/extract"
	"github.com/cloudposse/atmos/pkg/list/filter"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/importresolver"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/matrix"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Default columns for list instances if not specified in atmos.yaml.
var defaultInstanceColumns = []column.Config{
	{Name: "Component", Value: "{{ .component }}"},
	{Name: "Stack", Value: "{{ .stack }}"},
}

// InstancesCommandOptions contains options for the list instances command.
type InstancesCommandOptions struct {
	Info        *schema.ConfigAndStacksInfo
	Cmd         *cobra.Command
	Args        []string
	ShowImports bool
	ColumnsFlag []string
	// Format selects the output format (table, json, yaml, csv, tsv, tree, matrix).
	// Authoritative source — must reach this struct via viper so ATMOS_FORMAT
	// is honored. Do not re-read from cmd.Flags() inside the impl.
	Format string
	// Upload toggles upload of the instance inventory to Atmos Pro.
	// Authoritative source — must reach this struct via viper so ATMOS_UPLOAD
	// is honored. Do not re-read from cmd.Flags() inside the impl.
	Upload bool
	// Stack is an optional glob pattern (path.Match syntax) that filters
	// instances to stacks whose names match. Empty means no filter.
	// Matches the semantics used by `list components`.
	Stack       string
	FilterSpec  string
	SortSpec    string
	Delimiter   string
	Query       string
	AuthManager auth.AuthManager
	// AuthDisabled is true when the caller explicitly used --identity=false.
	// It prevents per-component auth auto-detection while still allowing
	// templates and YAML functions that do not require credentials to run.
	AuthDisabled bool
	OutputFile   string
	// ProcessTemplates toggles Go template processing of stack manifests
	// (controls the `processTemplates` parameter of `ExecuteDescribeStacks`).
	// Default true for parity with `describe affected` / `describe stacks`.
	// Go template functions include `atmos.Component(...)`.
	ProcessTemplates bool
	// ProcessFunctions toggles YAML function evaluation in stack manifests
	// (controls the `processYamlFunctions` parameter of `ExecuteDescribeStacks`).
	// YAML functions include `!terraform.state`, `!terraform.output`, `!store`,
	// `!aws.*`, etc. Default true for parity with `describe affected`; set to
	// false to avoid requiring `tofu` / `terraform` on $PATH when the only
	// YAML functions in the manifests are terraform-output-shaped.
	ProcessFunctions bool
	// Skip lists individual YAML functions to bypass during evaluation
	// (controls the `skip` parameter of `ExecuteDescribeStacks`).
	// Use to skip a specific function (e.g. `terraform.state`) while leaving
	// the rest of YAML function processing enabled.
	Skip []string
}

// parseColumnsFlag parses column specifications from CLI flag.
// Each flag value should be in the format: "Name=TemplateExpression"
// Example: --columns "Component={{ .component }}" --columns "Stack={{ .stack }}"
// Returns error if any column specification is invalid.
func parseColumnsFlag(columnsFlag []string) ([]column.Config, error) {
	if len(columnsFlag) == 0 {
		return defaultInstanceColumns, nil
	}

	columns := make([]column.Config, 0, len(columnsFlag))
	for i, spec := range columnsFlag {
		// Split on first '=' to separate name from template
		parts := strings.SplitN(spec, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("%w: column spec %d must be in format 'Name=Template', got: %q",
				errUtils.ErrInvalidConfig, i+1, spec)
		}

		name := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if name == "" {
			return nil, fmt.Errorf("%w: column spec %d has empty name", errUtils.ErrInvalidConfig, i+1)
		}
		if value == "" {
			return nil, fmt.Errorf("%w: column spec %d has empty template", errUtils.ErrInvalidConfig, i+1)
		}

		columns = append(columns, column.Config{
			Name:  name,
			Value: value,
		})
	}

	return columns, nil
}

// processComponentConfig processes a single component configuration and returns an instance if valid.
func processComponentConfig(stackName, componentName, componentType string, componentConfig interface{}) *schema.Instance {
	componentConfigMap, ok := componentConfig.(map[string]any)
	if !ok {
		return nil
	}
	return createInstance(stackName, componentName, componentType, componentConfigMap)
}

// processComponentType processes all components of a specific type in a stack.
func processComponentType(stackName, componentType string, typeComponents interface{}) []schema.Instance {
	typeComponentsMap, ok := typeComponents.(map[string]any)
	if !ok {
		return nil
	}

	var instances []schema.Instance
	for componentName, componentConfig := range typeComponentsMap {
		if instance := processComponentConfig(stackName, componentName, componentType, componentConfig); instance != nil {
			instances = append(instances, *instance)
		}
	}
	return instances
}

// processStackComponents processes all components in a stack.
func processStackComponents(stackName string, stackConfig interface{}) []schema.Instance {
	stackConfigMap, ok := stackConfig.(map[string]any)
	if !ok {
		return nil
	}

	components, ok := stackConfigMap["components"].(map[string]any)
	if !ok {
		return nil
	}

	var instances []schema.Instance
	for componentType, typeComponents := range components {
		if typeInstances := processComponentType(stackName, componentType, typeComponents); typeInstances != nil {
			instances = append(instances, typeInstances...)
		}
	}
	return instances
}

// matchStackPattern reports whether stackName matches the glob pattern using
// path.Match semantics. Both inputs are normalized with filepath.ToSlash so
// behavior is consistent across platforms. An empty pattern matches everything.
func matchStackPattern(stackName, pattern string) (bool, error) {
	if pattern == "" {
		return true, nil
	}
	matched, err := path.Match(filepath.ToSlash(pattern), filepath.ToSlash(stackName))
	if err != nil {
		return false, invalidStackPatternError(pattern, err)
	}
	return matched, nil
}

func validateStackPattern(pattern string) error {
	if pattern == "" {
		return nil
	}
	if _, err := path.Match(filepath.ToSlash(pattern), ""); err != nil {
		return invalidStackPatternError(pattern, err)
	}
	return nil
}

func invalidStackPatternError(pattern string, err error) error {
	return fmt.Errorf("%w: invalid --stack pattern %q: %w", errUtils.ErrInvalidFlag, pattern, err)
}

// filterStacksMapByPattern returns a new map containing only the entries of
// stacksMap whose stack name matches the glob pattern. When pattern is empty
// the original map is returned unchanged (no allocation).
func filterStacksMapByPattern(stacksMap map[string]any, pattern string) (map[string]any, error) {
	if pattern == "" {
		return stacksMap, nil
	}
	if err := validateStackPattern(pattern); err != nil {
		return nil, err
	}
	filtered := make(map[string]any, len(stacksMap))
	for stackName, stackConfig := range stacksMap {
		matched, err := matchStackPattern(stackName, pattern)
		if err != nil {
			return nil, err
		}
		if matched {
			filtered[stackName] = stackConfig
		}
	}
	return filtered, nil
}

// collectInstances collects all instances from the stacks map. When
// stackPattern is non-empty, stacks whose names do not match the glob are
// skipped.
func collectInstances(stacksMap map[string]interface{}, stackPattern string) ([]schema.Instance, error) {
	if err := validateStackPattern(stackPattern); err != nil {
		return nil, err
	}

	var instances []schema.Instance
	for stackName, stackConfig := range stacksMap {
		matched, err := matchStackPattern(stackName, stackPattern)
		if err != nil {
			return nil, err
		}
		if !matched {
			continue
		}
		if stackInstances := processStackComponents(stackName, stackConfig); stackInstances != nil {
			instances = append(instances, stackInstances...)
		}
	}
	return instances, nil
}

// createInstance creates an instance from the component configuration.
func createInstance(stackName, componentName, componentType string, componentConfigMap map[string]any) *schema.Instance {
	instance := &schema.Instance{
		Component:     componentName,
		Stack:         stackName,
		ComponentType: componentType,
		Settings:      make(map[string]any),
		Vars:          make(map[string]any),
		Env:           make(map[string]any),
		Backend:       make(map[string]any),
		Metadata:      make(map[string]any),
	}

	if settings, ok := componentConfigMap["settings"].(map[string]any); ok {
		instance.Settings = settings
	}
	if vars, ok := componentConfigMap["vars"].(map[string]any); ok {
		instance.Vars = vars
	}
	if env, ok := componentConfigMap["env"].(map[string]any); ok {
		instance.Env = env
	}
	if backend, ok := componentConfigMap["backend"].(map[string]any); ok {
		instance.Backend = backend
	}
	if metadata, ok := componentConfigMap["metadata"].(map[string]any); ok {
		instance.Metadata = metadata
	}

	// Skip abstract components.
	if metadataType, ok := instance.Metadata["type"].(string); ok && metadataType == "abstract" {
		return nil
	}

	return instance
}

// Stack-section keys used when collapsing the Atmos Pro enabled hierarchy.
const (
	proSettingsKey    = "pro"
	enabledKey        = "enabled"
	driftDetectionKey = "drift_detection"
)

// metadataEnabled reports whether a component's metadata marks it enabled.
// Mirrors isComponentEnabled (internal/exec/component_utils.go): a component is
// enabled by default; only an explicit metadata.enabled: false disables it.
func metadataEnabled(metadata map[string]any) bool {
	if enabled, ok := metadata[enabledKey].(bool); ok {
		return enabled
	}
	return true
}

// proSettingEnabled reads settings.pro.enabled, defaulting to true.
// Only an explicit boolean false disables; a missing or non-boolean value
// (e.g. the string "true") defaults to true, matching the Atmos Pro server-side
// default. The caller is responsible for confirming a pro block exists.
func proSettingEnabled(pro map[string]any) bool {
	if enabled, ok := pro[enabledKey].(bool); ok {
		return enabled
	}
	return true
}

// driftSettingEnabled reads settings.pro.drift_detection.enabled, defaulting to
// false. Only an explicit boolean true enables drift detection.
// The drift block is normalized with sanitizeForJSON first: nested YAML maps can
// arrive as map[interface{}]interface{}, which would otherwise fail the
// map[string]any assertion and read as "no drift block".
func driftSettingEnabled(pro map[string]any) bool {
	drift, ok := sanitizeForJSON(pro[driftDetectionKey]).(map[string]any)
	if !ok {
		return false
	}
	enabled, ok := drift[enabledKey].(bool)
	return ok && enabled
}

// effectiveEnabledState collapses the enabled hierarchy
// metadata.enabled > settings.pro.enabled > settings.pro.drift_detection.enabled.
// An outer disable forces all inner levels off, so the single signal Atmos Pro
// persists already reflects the component's resolved state:
//
//	proEnabled   = metadata.enabled && pro.enabled                     (both default true)
//	driftEnabled = proEnabled       && pro.drift_detection.enabled     (drift defaults false)
//
// A missing pro block means the instance is not Pro-enabled (and therefore not
// drift-enabled). This is the single source of truth shared by both the upload
// payload (extractProSettings) and the success-toast counts, so the two can
// never diverge.
func effectiveEnabledState(settings, metadata map[string]any) (proEnabled, driftEnabled bool) {
	// Normalize with sanitizeForJSON first: the pro subtree parsed from YAML can
	// be map[interface{}]interface{}, which would otherwise fail the
	// map[string]any assertion and incorrectly read as "no pro block".
	pro, ok := sanitizeForJSON(settings[proSettingsKey]).(map[string]any)
	if !ok {
		return false, false
	}
	proEnabled = metadataEnabled(metadata) && proSettingEnabled(pro)
	driftEnabled = proEnabled && driftSettingEnabled(pro)
	return proEnabled, driftEnabled
}

// metadataDisabledPro reports whether metadata.enabled: false is the reason an
// otherwise Pro-enabled instance collapses to disabled. It is true only when the
// pro block itself would be enabled (pro.enabled true or defaulted) but
// metadata.enabled is explicitly false, i.e. the outer metadata disable is what
// squashes pro.enabled to false in the upload payload. Used only for a debug log
// so operators can trace why a component is uploaded as disabled.
func metadataDisabledPro(settings, metadata map[string]any) bool {
	pro, ok := sanitizeForJSON(settings[proSettingsKey]).(map[string]any)
	if !ok {
		return false
	}
	return proSettingEnabled(pro) && !metadataEnabled(metadata)
}

// isProEnabled reports whether an instance is effectively Atmos Pro enabled,
// honoring the metadata.enabled > pro.enabled precedence.
func isProEnabled(instance *schema.Instance) bool {
	proEnabled, _ := effectiveEnabledState(instance.Settings, instance.Metadata)
	return proEnabled
}

// isDriftEnabled reports whether an instance is effectively drift-enabled.
// Drift requires the instance to be effectively Pro-enabled, so an outer
// metadata.enabled: false or pro.enabled: false disables drift regardless of
// settings.pro.drift_detection.enabled.
func isDriftEnabled(instance *schema.Instance) bool {
	_, driftEnabled := effectiveEnabledState(instance.Settings, instance.Metadata)
	return driftEnabled
}

// countEnabledDisabled returns counts of pro-enabled and non-enabled instances,
// plus the number with drift detection enabled. Counts use the effective state
// (metadata.enabled > pro.enabled > drift_detection.enabled), so they match
// exactly what is uploaded to Atmos Pro.
// "Disabled" covers explicit `settings.pro.enabled: false`, instances disabled
// via `metadata.enabled: false`, and instances with no `pro` config at all.
// Drift is counted only when the instance is effectively pro-enabled.
func countEnabledDisabled(instances []schema.Instance) (enabled, disabled, drift int) {
	for i := range instances {
		if isProEnabled(&instances[i]) {
			enabled++
		} else {
			disabled++
		}
		if isDriftEnabled(&instances[i]) {
			drift++
		}
	}
	return enabled, disabled, drift
}

// sortInstances sorts instances by stack and component.
func sortInstances(instances []schema.Instance) []schema.Instance {
	sort.SliceStable(instances, func(i, j int) bool {
		if instances[i].Stack != instances[j].Stack {
			return instances[i].Stack < instances[j].Stack
		}
		return instances[i].Component < instances[j].Component
	})
	return instances
}

// getInstanceColumns returns column configuration from CLI flag, atmos.yaml, or defaults.
// Returns error if CLI flag parsing fails.
// Precedence: CLI flag > list.instances.columns > components.list.columns (deprecated) > defaults.
func getInstanceColumns(atmosConfig *schema.AtmosConfiguration, columnsFlag []string) ([]column.Config, error) {
	// If --columns flag is provided, parse it and return.
	if len(columnsFlag) > 0 {
		columns, err := parseColumnsFlag(columnsFlag)
		if err != nil {
			return nil, err
		}
		return columns, nil
	}

	// Check new config path: list.instances.columns.
	if len(atmosConfig.List.Instances.Columns) > 0 {
		columns := make([]column.Config, len(atmosConfig.List.Instances.Columns))
		for i, col := range atmosConfig.List.Instances.Columns {
			columns[i] = column.Config{
				Name:  col.Name,
				Value: col.Value,
				Width: col.Width,
			}
		}
		return columns, nil
	}

	// Backward compatibility: check old config path components.list.columns.
	// This is deprecated but supported for existing configurations.
	if len(atmosConfig.Components.List.Columns) > 0 {
		columns := make([]column.Config, len(atmosConfig.Components.List.Columns))
		for i, col := range atmosConfig.Components.List.Columns {
			columns[i] = column.Config{
				Name:  col.Name,
				Value: col.Value,
				Width: col.Width,
			}
		}
		return columns, nil
	}

	// Return default columns.
	return defaultInstanceColumns, nil
}

// uploadInstancesWithDeps uploads instances to Atmos Pro API using injected dependencies.
// This function is testable via mocks. Use uploadInstances() for production code.
func uploadInstancesWithDeps(
	instances []schema.Instance,
	gitOps git.RepositoryOperations,
	configLoader cfg.Loader,
	clientFactory pro.ClientFactory,
) error {
	repo, err := gitOps.GetLocalRepo()
	if err != nil {
		log.Error(errUtils.ErrFailedToGetLocalRepo.Error(), "error", err)
		return errors.Join(errUtils.ErrFailedToGetLocalRepo, err)
	}

	repoInfo, err := gitOps.GetRepoInfo(repo)
	if err != nil {
		log.Error(errUtils.ErrFailedToGetRepoInfo.Error(), "error", err)
		return errors.Join(errUtils.ErrFailedToGetRepoInfo, err)
	}

	if repoInfo.RepoUrl == "" || repoInfo.RepoName == "" || repoInfo.RepoOwner == "" || repoInfo.RepoHost == "" {
		log.Warn("Git repo info is incomplete; upload may be rejected.", "repo_url", repoInfo.RepoUrl, "repo_name", repoInfo.RepoName, "repo_owner", repoInfo.RepoOwner, "repo_host", repoInfo.RepoHost)
	}

	// Initialize CLI config for API client.
	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := configLoader.InitCliConfig(&configInfo, false)
	if err != nil {
		log.Error(errUtils.ErrFailedToInitConfig.Error(), "error", err)
		return errors.Join(errUtils.ErrFailedToInitConfig, err)
	}

	apiClient, err := clientFactory.NewClient(&atmosConfig)
	if err != nil {
		log.Error(errUtils.ErrFailedToCreateAPIClient.Error(), "error", err)
		return errors.Join(errUtils.ErrFailedToCreateAPIClient, err)
	}

	// Convert schema.Instance to dtos.UploadInstance at the upload boundary.
	// UploadInstance is an allowlist — only fields Atmos Pro needs are included.
	// Sensitive data (vars, env, backend) never leaves this boundary.
	uploadInstances := make([]dtos.UploadInstance, len(instances))
	for i, inst := range instances {
		if metadataDisabledPro(inst.Settings, inst.Metadata) {
			log.Debug("Collapsing pro.enabled to false for upload: metadata.enabled is false",
				KeyComponent, inst.Component, KeyStack, inst.Stack)
		}
		uploadInstances[i] = dtos.UploadInstance{
			Component:     inst.Component,
			Stack:         inst.Stack,
			ComponentType: inst.ComponentType,
			Settings:      extractProSettings(inst.Settings, inst.Metadata),
		}
	}

	req := dtos.InstancesUploadRequest{
		RepoURL:   repoInfo.RepoUrl,
		RepoName:  repoInfo.RepoName,
		RepoOwner: repoInfo.RepoOwner,
		RepoHost:  repoInfo.RepoHost,
		Instances: uploadInstances,
	}

	err = apiClient.UploadInstances(&req)
	if err != nil {
		log.Error(errUtils.ErrFailedToUploadInstances.Error(), "error", err)
		return errors.Join(errUtils.ErrFailedToUploadInstances, err)
	}

	enabled, disabled, drift := countEnabledDisabled(instances)
	u.PrintfMessageToTUI("Successfully uploaded %d instances to Atmos Pro API (%d enabled, %d disabled, %d drift enabled).", len(instances), enabled, disabled, drift)
	return nil
}

// uploadInstances uploads instances to Atmos Pro API.
// This is a convenience wrapper around uploadInstancesWithDeps() for production use.
func uploadInstances(instances []schema.Instance) error {
	return uploadInstancesWithDeps(
		instances,
		&git.DefaultRepositoryOperations{},
		&cfg.DefaultLoader{},
		&pro.DefaultClientFactory{},
	)
}

// processInstancesWithDeps collects, filters, and sorts instances using injected dependencies.
// This function is testable via mocks. Use processInstances() for production code.
//
// Template processing (`processTemplates`) controls Go-template evaluation,
// which includes the `atmos.Component(...)` template function — NOT the YAML
// functions like `!terraform.state` / `!terraform.output`. Those are
// controlled by `processYamlFunctions`. The two are independent.
//
// The CLI defaults both flags to `true` via `--process-templates` /
// `--process-functions` (env: `ATMOS_PROCESS_TEMPLATES` /
// `ATMOS_PROCESS_FUNCTIONS`), matching the describe command family. Callers
// running without `tofu` / `terraform` on `$PATH` should pass
// `--process-functions=false` to skip YAML-function evaluation while still
// letting templates expand stack names and metadata.
//
//nolint:revive // argument-limit: positional fan-out kept for test-seam clarity.
func processInstancesWithDeps(
	atmosConfig *schema.AtmosConfiguration,
	stacksProcessor e.StacksProcessor,
	authManager auth.AuthManager,
	processTemplates, processYamlFunctions bool,
	skip []string,
	stackPattern string,
	authDisabled bool,
) ([]schema.Instance, error) {
	stacksMap, err := executeDescribeStacksForInstances(
		atmosConfig,
		stacksProcessor,
		authManager,
		processTemplates,
		processYamlFunctions,
		skip,
		authDisabled,
	)
	if err != nil {
		log.Error(errUtils.ErrExecuteDescribeStacks.Error(), "error", err)
		return nil, errors.Join(errUtils.ErrExecuteDescribeStacks, err)
	}

	// Collect instances, applying the --stack glob filter when present.
	instances, err := collectInstances(stacksMap, stackPattern)
	if err != nil {
		return nil, err
	}

	// Sort instances.
	instances = sortInstances(instances)

	return instances, nil
}

type authDisabledStacksProcessor interface {
	ExecuteDescribeStacksWithAuthDisabled(
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
	) (map[string]any, error)
}

// executeDescribeStacksForInstances dispatches to ExecuteDescribeStacksWithAuthDisabled
// when authDisabled is requested AND the processor implements the optional
// authDisabledStacksProcessor interface; otherwise it falls back to the
// standard ExecuteDescribeStacks call. The `skip` list is forwarded to both
// paths so --skip continues to bypass the named YAML functions regardless of
// whether auth is disabled.
//
//nolint:revive // Helper mirrors the StacksProcessor call shape with skip + authDisabled passthrough.
func executeDescribeStacksForInstances(
	atmosConfig *schema.AtmosConfiguration,
	stacksProcessor e.StacksProcessor,
	authManager auth.AuthManager,
	processTemplates, processYamlFunctions bool,
	skip []string,
	authDisabled bool,
) (map[string]any, error) {
	if authDisabled {
		if processor, ok := stacksProcessor.(authDisabledStacksProcessor); ok {
			return processor.ExecuteDescribeStacksWithAuthDisabled(
				atmosConfig, "", nil, nil, nil,
				false, // ignoreMissingFiles
				processTemplates,
				processYamlFunctions,
				false, // includeEmptyStacks
				skip,
				authManager,
				authDisabled,
			)
		}
	}

	return stacksProcessor.ExecuteDescribeStacks(
		atmosConfig, "", nil, nil, nil,
		false, // ignoreMissingFiles
		processTemplates,
		processYamlFunctions,
		false, // includeEmptyStacks
		skip,
		authManager,
	)
}

// processInstances collects, filters, and sorts instances using the default
// DefaultStacksProcessor. This is the production entry point — tests use
// processInstancesWithDeps directly to inject a mocked StacksProcessor.
//
// The wrapper threads --skip / --stack glob / --identity=false through to the
// describe pipeline. The authDisabled flag short-circuits per-component auth
// resolution while still letting templates and credential-free YAML functions run.
//
//nolint:revive // argument-limit: positional fan-out matches processInstancesWithDeps.
func processInstances(
	atmosConfig *schema.AtmosConfiguration,
	authManager auth.AuthManager,
	processTemplates, processYamlFunctions bool,
	skip []string,
	stackPattern string,
	authDisabled bool,
) ([]schema.Instance, error) {
	return processInstancesWithDeps(
		atmosConfig,
		&e.DefaultStacksProcessor{},
		authManager,
		processTemplates,
		processYamlFunctions,
		skip,
		stackPattern,
		authDisabled,
	)
}

// ExecuteListInstancesCmd executes the list instances command.
//
//nolint:revive,cyclop,funlen // Complexity and length from format branching and upload handling (unavoidable pattern).
func ExecuteListInstancesCmd(opts *InstancesCommandOptions) error {
	defer perf.Track(nil, "list.ExecuteListInstancesCmd")()

	log.Trace("ExecuteListInstancesCmd starting")
	// Initialize CLI config.
	atmosConfig, err := cfg.InitCliConfig(*opts.Info, true)
	if err != nil {
		log.Error(errUtils.ErrFailedToInitConfig.Error(), "error", err)
		return errors.Join(errUtils.ErrFailedToInitConfig, err)
	}

	// Read flags from the options struct (populated via viper, so env vars
	// like ATMOS_FORMAT / ATMOS_UPLOAD are honored). Reading from
	// opts.Cmd.Flags() here would bypass viper precedence.
	upload := opts.Upload
	formatFlag := opts.Format

	// Handle matrix format specially - it bypasses the normal rendering pipeline.
	if formatFlag == string(format.FormatMatrix) {
		if upload {
			return fmt.Errorf("%w: --upload is not supported with --format=matrix", errUtils.ErrInvalidFlag)
		}
		if opts.FilterSpec != "" {
			return fmt.Errorf("%w: --filter is not supported with --format=matrix", errUtils.ErrInvalidFlag)
		}
		if opts.Query != "" {
			return fmt.Errorf("%w: --query is not supported with --format=matrix", errUtils.ErrInvalidFlag)
		}
		return executeMatrixFormat(&atmosConfig, opts)
	}

	// Reject --output-file for non-matrix formats — it would be silently ignored.
	if opts.OutputFile != "" {
		return fmt.Errorf("%w: --output-file is only supported with --format=matrix", errUtils.ErrInvalidFlag)
	}

	// Handle tree format specially - branch before calling processInstances to avoid double processing.
	log.Trace("Checking format flag", "format_flag", formatFlag, "format_tree", format.FormatTree, "match", formatFlag == string(format.FormatTree))
	if formatFlag == string(format.FormatTree) {
		// Tree format does not support --upload, --filter, or --query because it
		// renders the import hierarchy rather than per-row values; row-shaped
		// transforms have no meaningful target here.
		if upload {
			return fmt.Errorf("%w: --upload is not supported with --format=tree", errUtils.ErrInvalidFlag)
		}
		if opts.FilterSpec != "" {
			return fmt.Errorf("%w: --filter is not supported with --format=tree", errUtils.ErrInvalidFlag)
		}
		if opts.Query != "" {
			return fmt.Errorf("%w: --query is not supported with --format=tree", errUtils.ErrInvalidFlag)
		}

		// Enable provenance tracking to capture import chains.
		atmosConfig.TrackProvenance = true

		// Clear caches to ensure fresh processing with provenance enabled.
		e.ClearMergeContexts()
		e.ClearFindStacksMapCache()

		// Get all stacks for provenance-based import resolution (single call).
		// Honor the caller-supplied template/function flags so tree output is
		// consistent with non-tree runs of the same command invocation, matching
		// the behavior of `list stacks --format=tree`.
		stacksMap, err := e.ExecuteDescribeStacksWithAuthDisabled(
			&atmosConfig, "", nil, nil, nil,
			false, // ignoreMissingFiles
			opts.ProcessTemplates,
			opts.ProcessFunctions,
			false, // includeEmptyStacks
			opts.Skip,
			opts.AuthManager,
			opts.AuthDisabled,
		)
		if err != nil {
			log.Error(errUtils.ErrExecuteDescribeStacks.Error(), "error", err)
			return errors.Join(errUtils.ErrExecuteDescribeStacks, err)
		}

		// Apply --stack glob filter before provenance resolution so the tree
		// only contains matching stacks. Provenance keeps the full merge-context
		// cache so import chains for matching stacks still resolve correctly.
		stacksMap, err = filterStacksMapByPattern(stacksMap, opts.Stack)
		if err != nil {
			return err
		}

		// Resolve import trees using provenance system.
		importTrees, err := importresolver.ResolveImportTreeFromProvenance(stacksMap, &atmosConfig)
		if err != nil {
			return fmt.Errorf("failed to resolve import trees: %w", err)
		}

		// Render tree view.
		// Use showImports parameter from --provenance flag.
		output := format.RenderInstancesTree(importTrees, opts.ShowImports)
		return data.Writeln(output)
	}

	// For non-tree formats, process instances normally. The single call threads
	// every relevant option through one canonical path: --skip bypasses named
	// YAML functions, opts.Stack applies the glob filter post-describe, and
	// --identity=false (opts.AuthDisabled) short-circuits per-component auth.
	instances, err := processInstances(
		&atmosConfig,
		opts.AuthManager,
		opts.ProcessTemplates,
		opts.ProcessFunctions,
		opts.Skip,
		opts.Stack,
		opts.AuthDisabled,
	)
	if err != nil {
		log.Error(errUtils.ErrProcessInstances.Error(), "error", err)
		return errors.Join(errUtils.ErrProcessInstances, err)
	}

	// Extract instances into renderer-compatible format with metadata fields.
	data := extract.Metadata(instances)

	// Get column configuration.
	columns, err := getInstanceColumns(&atmosConfig, opts.ColumnsFlag)
	if err != nil {
		log.Error("failed to get columns", "error", err)
		return errors.Join(errUtils.ErrInvalidConfig, err)
	}

	// Create column selector.
	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("failed to create column selector: %w", err)
	}

	// Build filters from filter specification.
	filters, err := buildInstanceFilters(opts.FilterSpec, &atmosConfig)
	if err != nil {
		return fmt.Errorf("failed to build filters: %w", err)
	}

	// Append the --query projector after filters so it rewrites only the
	// rows that survived predicate filtering.
	if opts.Query != "" {
		proj, err := filter.NewYQProjector(opts.Query, &atmosConfig)
		if err != nil {
			return fmt.Errorf("failed to build query projector: %w", err)
		}
		filters = append(filters, proj)
	}

	// Build sorters from sort specification.
	// Pass columns to allow smart default sorting based on available columns.
	sorters, err := buildInstanceSorters(opts.SortSpec, columns)
	if err != nil {
		return fmt.Errorf("failed to build sorters: %w", err)
	}

	// Create renderer.
	r := renderer.New(filters, selector, sorters, format.Format(formatFlag), opts.Delimiter)

	// Render output.
	if err := r.Render(data); err != nil {
		return fmt.Errorf("failed to render instances: %w", err)
	}

	// Handle upload if requested.
	if upload {
		if len(instances) == 0 {
			ui.Info("No instances found; nothing to upload.")
			return nil
		}
		if uploadErr := uploadInstances(instances); uploadErr != nil {
			return uploadErr
		}
	}

	return nil
}

// extractProSettings extracts only the "pro" key from a settings map for upload.
// Returns nil if settings is nil or has no "pro" key.
// Sanitizes nested maps to ensure JSON compatibility (converting
// map[interface{}]interface{} from YAML to map[string]interface{}).
//
// Before upload it collapses the enabled hierarchy
// (metadata.enabled > pro.enabled > drift_detection.enabled) so the values
// Atmos Pro persists already reflect any outer disable. Atmos Pro's ingestion
// contract has no `metadata` field, so a component disabled via
// `metadata.enabled: false` would otherwise be uploaded as enabled and keep
// getting dispatched for drift detection. The collapse runs on the sanitized
// copy, so the source instance (used by the toast counts) is never mutated.
func extractProSettings(settings, metadata map[string]any) map[string]any {
	if settings == nil {
		return nil
	}

	pro, hasPro := settings[proSettingsKey]
	if !hasPro {
		return nil
	}

	sanitized := sanitizeForJSON(pro)

	// When pro is not a map (malformed config, e.g. a stray string), there is
	// nothing to collapse; pass it through unchanged.
	proMap, ok := sanitized.(map[string]any)
	if !ok {
		return map[string]any{proSettingsKey: sanitized}
	}

	proEnabled, driftEnabled := effectiveEnabledState(settings, metadata)
	proMap[enabledKey] = proEnabled
	// Only override an existing drift_detection block. When none exists, Atmos
	// Pro already defaults drift to false, so synthesizing one adds noise.
	if drift, ok := proMap[driftDetectionKey].(map[string]any); ok {
		drift[enabledKey] = driftEnabled
	}

	return map[string]any{proSettingsKey: proMap}
}

// sanitizeForJSON recursively converts map[interface{}]interface{} to
// map[string]interface{} for JSON compatibility.
func sanitizeForJSON(v any) any {
	switch val := v.(type) {
	case map[interface{}]interface{}:
		m := make(map[string]interface{}, len(val))
		for k, v := range val {
			m[fmt.Sprintf("%v", k)] = sanitizeForJSON(v)
		}
		return m
	case map[string]interface{}:
		m := make(map[string]interface{}, len(val))
		for k, v := range val {
			m[k] = sanitizeForJSON(v)
		}
		return m
	case []interface{}:
		s := make([]interface{}, len(val))
		for i, v := range val {
			s[i] = sanitizeForJSON(v)
		}
		return s
	default:
		return v
	}
}

// executeMatrixFormat handles the matrix output format for list instances.
// It produces GitHub Actions-compatible matrix JSON matching describe affected --format=matrix.
// When ci.enabled is true and no --output-file is provided, automatically writes to $GITHUB_OUTPUT.
func executeMatrixFormat(atmosConfig *schema.AtmosConfiguration, opts *InstancesCommandOptions) error {
	defer perf.Track(nil, "list.executeMatrixFormat")()

	// Get stacksMap to extract component_path from component_info. Honor the
	// caller-supplied template/function flags so matrix output stays consistent
	// with non-matrix runs of the same command invocation.
	stacksMap, err := e.ExecuteDescribeStacksWithAuthDisabled(
		atmosConfig, "", nil, nil, nil,
		false, // ignoreMissingFiles
		opts.ProcessTemplates,
		opts.ProcessFunctions,
		false, // includeEmptyStacks
		opts.Skip,
		opts.AuthManager,
		opts.AuthDisabled,
	)
	if err != nil {
		log.Error(errUtils.ErrExecuteDescribeStacks.Error(), "error", err)
		return errors.Join(errUtils.ErrExecuteDescribeStacks, err)
	}

	// Apply --stack glob filter before flattening so matrix entries only
	// reflect matching stacks.
	stacksMap, err = filterStacksMapByPattern(stacksMap, opts.Stack)
	if err != nil {
		return err
	}

	entries := extract.StacksMatrixEntries(stacksMap)

	// Resolve output file: explicit flag > CI auto-detect > stdout.
	outputFile := opts.OutputFile
	if outputFile == "" && atmosConfig.CI.Enabled {
		outputFile = ghactions.GetOutputPath()
	}

	return matrix.WriteOutput(entries, outputFile)
}

// buildInstanceFilters creates filters from a `--filter` specification. The
// spec is interpreted as a YQ expression evaluated per row; rows for which
// the expression is truthy are kept. An empty spec produces no filters.
func buildInstanceFilters(filterSpec string, atmosConfig *schema.AtmosConfiguration) ([]filter.Filter, error) {
	if filterSpec == "" {
		return nil, nil
	}
	f, err := filter.NewYQPredicateFilter(filterSpec, atmosConfig)
	if err != nil {
		return nil, err
	}
	return []filter.Filter{f}, nil
}

// buildInstanceSorters creates sorters from sort specification.
// When sortSpec is empty and columns contain default "Component" and "Stack",
// applies default sorting. Otherwise returns empty sorters (natural order).
func buildInstanceSorters(sortSpec string, columns []column.Config) ([]*listSort.Sorter, error) {
	// If user provided explicit sort spec, use it.
	if sortSpec != "" {
		return listSort.ParseSortSpec(sortSpec)
	}

	// Build map of available column names.
	columnNames := make(map[string]bool)
	for _, col := range columns {
		columnNames[col.Name] = true
	}

	// Only apply default sort if both Component and Stack columns exist.
	if columnNames["Component"] && columnNames["Stack"] {
		return []*listSort.Sorter{
			listSort.NewSorter("Component", listSort.Ascending),
			listSort.NewSorter("Stack", listSort.Ascending),
		}, nil
	}

	// No default sort for custom columns - return empty sorters (natural order).
	return nil, nil
}
