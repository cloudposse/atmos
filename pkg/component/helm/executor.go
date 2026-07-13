package helm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/hooks"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/manifest"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
	tfgenerate "github.com/cloudposse/atmos/pkg/terraform/generate"
	u "github.com/cloudposse/atmos/pkg/utils"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Diff flag keys (as stored in ExecutionContext.Flags) and the special baseline
// selector value for the live deployed release.
const (
	flagAgainst      = "against"
	flagFromManifest = "from-manifest"
	flagContext      = "context"
	againstRelease   = "release"
)

// Seams for testing.
var (
	initCliConfig                    = cfg.InitCliConfig
	setupComponentAuthForCLI         = e.SetupComponentAuthForCLI
	processStacks                    = e.ProcessStacks
	executeDescribeStacks            = e.ExecuteDescribeStacks
	executeAffectedWithRepoPath      = e.ExecuteDescribeAffectedWithTargetRepoPath
	executeAffectedWithRefClone      = e.ExecuteDescribeAffectedWithTargetRefClone
	executeAffectedWithRefCheckout   = e.ExecuteDescribeAffectedWithTargetRefCheckout
	executeGraph                     = component.ExecuteGraph
	affectedHelmComponentsFunc       = affectedHelmComponents
	provisionAndResolveComponentPath = component.ProvisionAndResolveComponentPath
	dependenciesForComponent         = dependencies.ForComponent
	getHooks                         = hooks.GetHooks
	runCIHooks                       = hooks.RunCIHooks
	renderChartManifest              = renderManifest
	applyHelmRelease                 = applyRelease
	deleteHelmRelease                = deleteRelease
	setupRepositories                = setupHelmRepositories
)

// renderTimeout bounds a single chart render/locate (which may download remote charts).
const renderTimeout = 5 * time.Minute

// Execute runs a single Helm component operation.
func Execute(ctx *component.ExecutionContext, operation Operation) error {
	defer perf.Track(ctx.AtmosConfig, "helm.ExecuteOperation")()

	info := ctx.ConfigAndStacksInfo
	info.ComponentType = cfg.HelmComponentType
	if info.SubCommand == "" {
		info.SubCommand = ctx.SubCommand
	}
	if info.SubCommand == "" {
		info.SubCommand = string(operation)
	}
	info.CliArgs = []string{cfg.HelmComponentType, info.SubCommand}

	atmosConfig, err := initCliConfig(info, true)
	if err != nil {
		return err
	}
	normalizeGlobalConfig(&atmosConfig)

	if info.All || info.Affected {
		return executeBulk(ctx, &atmosConfig, &info, operation)
	}

	return executeSingle(ctx, &atmosConfig, &info, operation)
}

// executeSingle runs the operation for a single Helm component (the non-bulk path).
func executeSingle(
	ctx *component.ExecutionContext,
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	operation Operation,
) error {
	if err := processStacksWithAuth(atmosConfig, info); err != nil {
		return err
	}
	if !info.ComponentIsEnabled {
		log.Info("Component is not enabled and skipped", "component", info.ComponentFromArg)
		return nil
	}

	if err := (&ComponentProvider{}).ValidateComponent(info.ComponentSection); err != nil {
		return err
	}

	componentPath, err := resolveComponentPath(atmosConfig, info)
	if err != nil {
		return err
	}

	if err := renderInputTemplates(atmosConfig, info.ComponentSection); err != nil {
		return err
	}

	if err := maybeAutoGenerateFiles(atmosConfig, info, componentPath); err != nil {
		return err
	}

	tenv, err := dependenciesForComponent(atmosConfig, cfg.HelmComponentType, info.StackSection, info.ComponentSection)
	if err != nil {
		return err
	}
	envRestore := applyEnvironment(info.ComponentEnvSection, tenv.EnvVars())
	authEnvRestore, err := applyAuthEnvironment(info)
	if err != nil {
		envRestore()
		return err
	}
	defer func() {
		authEnvRestore()
		envRestore()
	}()

	return runWithHooks(ctx, atmosConfig, info, operation, componentPath)
}

// runWithHooks runs the before/after hooks around chart rendering and the operation.
func runWithHooks(
	ctx *component.ExecutionContext,
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	operation Operation,
	componentPath string,
) error {
	hookSet, err := getHooks(atmosConfig, info)
	if err != nil {
		return err
	}
	before, after := eventsFor(info.SubCommand, operation)
	if err := hookSet.RunAll(before, atmosConfig, info, nil, nil); err != nil {
		return err
	}

	spec, err := buildChartSpec(atmosConfig, info, componentPath)
	if err != nil {
		return err
	}
	if spec.ReleaseName == "" {
		return errUtils.ErrHelmReleaseNameRequired
	}
	if operation != OperationDelete {
		if err := setupRepositories(spec.Repositories); err != nil {
			return err
		}
	}

	summary, opErr := runOperation(ctx, atmosConfig, info, operation, spec)
	runHelmCIHook(helmCIHookParams{
		ctx:         ctx,
		atmosConfig: atmosConfig,
		info:        info,
		event:       after,
		summary:     summary,
		commandErr:  opErr,
	})
	if opErr != nil {
		return opErr
	}

	if err := hookSet.RunAll(after, atmosConfig, info, nil, nil); err != nil {
		return err
	}

	return nil
}

// runOperation dispatches to the requested Helm operation.
func runOperation(
	ctx *component.ExecutionContext,
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	operation Operation,
	spec *chartSpec,
) (map[string]any, error) {
	summary := helmSummary(info, spec, ctx.Flags)
	switch operation {
	case OperationTemplate:
		objects, err := runTemplate(ctx, atmosConfig, info, spec)
		addObjectsToSummary(summary, objects)
		return summary, err
	case OperationDiff:
		diffText, err := runDiff(atmosConfig, info, ctx.Flags, spec)
		summary["diff"] = diffText
		return summary, err
	case OperationApply:
		applySummary, err := deliverApply(atmosConfig, info, ctx.Flags, spec)
		mergeSummary(summary, applySummary)
		return summary, err
	case OperationDelete:
		err := deleteHelmRelease(spec.ReleaseName, spec.Namespace)
		return summary, err
	default:
		return summary, fmt.Errorf("%w: %q", errUtils.ErrHelmUnsupportedOperation, operation)
	}
}

// runTemplate renders the chart and writes the manifests per the render options.
func runTemplate(ctx *component.ExecutionContext, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, spec *chartSpec) ([]*unstructured.Unstructured, error) {
	objects, err := renderObjects(spec)
	if err != nil {
		return nil, err
	}
	options := resolveRenderOptions(ctx.Flags, info.ComponentSection)
	options.AtmosConfig = atmosConfig
	if err := manifest.WriteObjects(objects, options); err != nil {
		return objects, err
	}
	return objects, nil
}

// runDiff renders the chart client-side (no cluster) and computes a unified diff
// against a baseline. The baseline source is selected by flags: a local file
// (--from-manifest), the git deployment-repo provision target (--against=target),
// or the currently deployed release (default; the only path that needs a cluster).
// The diff is written to the data channel (secrets are redacted) and returned for
// the CI job summary.
func runDiff(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	flags map[string]any,
	spec *chartSpec,
) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), renderTimeout)
	defer cancel()

	desired, err := renderChartManifest(ctx, spec)
	if err != nil {
		return "", err
	}

	baseline, err := resolveDiffBaseline(atmosConfig, info, flags, spec)
	if err != nil {
		return "", err
	}

	diffText, _, err := unifiedDiff(baseline, desired, spec.Namespace, diffContextFromFlags(flags))
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(diffText) == "" {
		_ = data.Writeln("No changes. The rendered chart matches the baseline.")
	} else {
		_ = data.Write(colorizeUnifiedDiff(diffText))
	}
	return diffText, nil
}

// resolveDiffBaseline resolves the "current" manifest to diff against based on the
// flags, in precedence order: --from-manifest (file), --against=target (GitOps),
// otherwise the deployed release.
func resolveDiffBaseline(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	flags map[string]any,
	spec *chartSpec,
) (string, error) {
	if path := flagString(flags, flagFromManifest); path != "" {
		content, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("%w: %w", errUtils.ErrHelmBaselineRead, err)
		}
		return string(content), nil
	}

	against := flagString(flags, flagAgainst)
	if against != "" && against != againstRelease {
		return fetchTargetBaseline(atmosConfig, info, against)
	}

	return getDeployedManifest(spec.ReleaseName, spec.Namespace)
}

// fetchTargetBaseline reads the current manifests from a non-cluster provision
// target (e.g. the git deployment repository) so a render can be diffed against
// the live GitOps state offline. The value is "target" (the default/selected
// target) or "target:<name>".
func fetchTargetBaseline(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, against string) (string, error) {
	targetName := ""
	if _, name, ok := strings.Cut(against, ":"); ok {
		targetName = name
	}

	provisionSection, _ := info.ComponentSection["provision"].(map[string]any)
	selected, err := target.SelectTarget(provisionSection, targetName)
	if err != nil {
		return "", err
	}
	if selected.Kind == target.KindKubernetes {
		return "", fmt.Errorf("%w: --against=target requires a non-cluster provision target such as a git deployment repository", errUtils.ErrHelmDiffFailed)
	}

	ctx, cancel := context.WithTimeout(context.Background(), deliveryTimeout)
	defer cancel()

	artifact, err := target.Fetch(ctx, selected.Kind, &target.FetchInput{
		AtmosConfig:  atmosConfig,
		TargetName:   selected.Name,
		TargetConfig: selected.Config,
		EnvProvider:  authManagerFor(info),
	})
	if err != nil {
		return "", err
	}
	return joinManifests(artifact.Files), nil
}

// joinManifests concatenates artifact file contents (sorted by path) into a single
// multi-document manifest string suitable for diffing.
func joinManifests(files map[string][]byte) string {
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, key := range keys {
		content := strings.TrimSpace(string(files[key]))
		if content == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n---\n")
		}
		b.WriteString(content)
	}
	return b.String()
}

func flagString(flags map[string]any, key string) string {
	value, _ := flags[key].(string)
	return value
}

func diffContextFromFlags(flags map[string]any) int {
	if n, ok := flags[flagContext].(int); ok {
		return n
	}
	return 0
}

// renderObjects renders the chart to manifest objects (client-side, no cluster).
func renderObjects(spec *chartSpec) ([]*unstructured.Unstructured, error) {
	ctx, cancel := context.WithTimeout(context.Background(), renderTimeout)
	defer cancel()

	rendered, err := renderChartManifest(ctx, spec)
	if err != nil {
		return nil, err
	}
	objects, err := manifest.DecodeObjects([]byte(rendered))
	if err != nil {
		return nil, err
	}
	if len(objects) == 0 {
		return nil, fmt.Errorf("%w: chart %q rendered no objects", errUtils.ErrHelmRenderFailed, spec.Chart)
	}
	return objects, nil
}

func normalizeGlobalConfig(atmosConfig *schema.AtmosConfiguration) {
	if atmosConfig.Components.Helm.BasePath == "" {
		atmosConfig.Components.Helm.BasePath = DefaultConfig().BasePath
	}
}

func processStacksWithAuth(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	var authManager auth.AuthManager
	if info.Identity != "" {
		var err error
		authManager, err = setupComponentAuthForCLI(atmosConfig, info)
		if err != nil {
			return err
		}
	}

	processedInfo, err := processStacks(atmosConfig, *info, true, true, true, nil, authManager)
	if err != nil {
		return err
	}

	*info = processedInfo
	return nil
}

func resolveComponentPath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, error) {
	initialPath, err := u.GetComponentPath(atmosConfig, cfg.HelmComponentType, info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return "", errors.Join(errUtils.ErrPathResolution, fmt.Errorf("component path: %w", err))
	}

	provisionCtx, cancel := context.WithTimeout(context.Background(), renderTimeout)
	defer cancel()
	path, _, err := provisionAndResolveComponentPath(provisionCtx, atmosConfig, info, cfg.HelmComponentType, initialPath)
	return path, err
}

func maybeAutoGenerateFiles(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string) error {
	if !atmosConfig.Components.Helm.AutoGenerateFiles {
		return nil
	}

	generateSection := tfgenerate.GetGenerateSectionFromComponent(info.ComponentSection)
	if generateSection == nil {
		return nil
	}

	if err := os.MkdirAll(componentPath, dirPerm); err != nil {
		return fmt.Errorf("failed to create Helm component directory: %w", err)
	}

	templateContext := tfgenerate.BuildTemplateContext(info)
	_, err := tfgenerate.GenerateFiles(generateSection, componentPath, templateContext, tfgenerate.GenerateConfig{})
	return err
}

func eventsFor(command string, operation Operation) (hooks.HookEvent, hooks.HookEvent) {
	if command == "deploy" {
		return hooks.BeforeHelmDeploy, hooks.AfterHelmDeploy
	}
	switch operation {
	case OperationTemplate:
		return hooks.BeforeHelmTemplate, hooks.AfterHelmTemplate
	case OperationDiff:
		return hooks.BeforeHelmDiff, hooks.AfterHelmDiff
	case OperationApply:
		return hooks.BeforeHelmApply, hooks.AfterHelmApply
	case OperationDelete:
		return hooks.BeforeHelmDelete, hooks.AfterHelmDelete
	default:
		return hooks.HookEvent(""), hooks.HookEvent("")
	}
}

// helmCIHookParams bundles the inputs for running a CI hook around a Helm operation.
type helmCIHookParams struct {
	ctx         *component.ExecutionContext
	atmosConfig *schema.AtmosConfiguration
	info        *schema.ConfigAndStacksInfo
	event       hooks.HookEvent
	summary     map[string]any
	commandErr  error
}

func runHelmCIHook(p helmCIHookParams) {
	if p.event == "" {
		return
	}
	summary := p.summary
	if summary == nil {
		summary = map[string]any{}
	}
	if err := runCIHooks(&hooks.RunCIHooksOptions{
		Event:        p.event,
		AtmosConfig:  p.atmosConfig,
		Info:         p.info,
		ForceCIMode:  helmCIModeEnabled(p.ctx.Flags),
		CommandError: p.commandErr,
		ExitCode:     errUtils.GetExitCode(p.commandErr),
		Aggregate:    summary,
	}); err != nil {
		log.Warn("CI hook execution failed", "component", p.info.ComponentFromArg, "error", err)
	}
}

func helmSummary(info *schema.ConfigAndStacksInfo, spec *chartSpec, flags map[string]any) map[string]any {
	target := "kubernetes"
	if value, ok := flags["target"].(string); ok && value != "" {
		target = value
	}
	return map[string]any{
		"component":    info.ComponentFromArg,
		"stack":        info.Stack,
		"command":      info.SubCommand,
		"chart":        spec.Chart,
		"release_name": spec.ReleaseName,
		"namespace":    spec.Namespace,
		"target":       target,
	}
}

func mergeSummary(dst, src map[string]any) {
	for key, value := range src {
		dst[key] = value
	}
}

func addObjectsToSummary(summary map[string]any, objects []*unstructured.Unstructured) {
	if len(objects) == 0 {
		return
	}
	kinds := make(map[string]struct{})
	for _, obj := range objects {
		if obj == nil {
			continue
		}
		if kind := obj.GetKind(); kind != "" {
			kinds[kind] = struct{}{}
		}
	}
	list := make([]string, 0, len(kinds))
	for kind := range kinds {
		list = append(list, kind)
	}
	sort.Strings(list)
	summary["object_count"] = len(objects)
	summary["object_kinds"] = list
}

func helmCIModeEnabled(flags map[string]any) bool {
	if value, ok := flags["ci"].(bool); ok && value {
		return true
	}
	return ciEnvEnabled("ATMOS_CI") || ciEnvEnabled("CI")
}

func ciEnvEnabled(key string) bool {
	//nolint:forbidigo // Standard CI env vars (ATMOS_CI/CI), read directly for CI auto-detection.
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return value != "" && value != "false" && value != "0" && value != "no"
}

func applyEnvironment(componentEnv map[string]any, toolchainEnv []string) func() {
	original := make(map[string]*string)
	setEnv := func(key, value string) {
		if _, ok := original[key]; !ok {
			if existing, exists := os.LookupEnv(key); exists {
				existingCopy := existing
				original[key] = &existingCopy
			} else {
				original[key] = nil
			}
		}
		_ = os.Setenv(key, value)
	}

	for key, value := range componentEnv {
		setEnv(key, fmt.Sprintf("%v", value))
	}
	for _, item := range toolchainEnv {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			setEnv(key, value)
		}
	}

	return func() {
		for key, value := range original {
			if value == nil {
				_ = os.Unsetenv(key)
			} else {
				_ = os.Setenv(key, *value)
			}
		}
	}
}
