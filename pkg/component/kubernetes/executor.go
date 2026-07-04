package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"os"
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
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	tfgenerate "github.com/cloudposse/atmos/pkg/terraform/generate"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	initCliConfig                    = cfg.InitCliConfig
	setupComponentAuthForCLI         = e.SetupComponentAuthForCLI
	processStacks                    = e.ProcessStacks
	executeDescribeStacks            = e.ExecuteDescribeStacks
	executeAffectedWithRepoPath      = e.ExecuteDescribeAffectedWithTargetRepoPath
	executeAffectedWithRefClone      = e.ExecuteDescribeAffectedWithTargetRefClone
	executeAffectedWithRefCheckout   = e.ExecuteDescribeAffectedWithTargetRefCheckout
	executeGraph                     = component.ExecuteGraph
	affectedKubernetesComponentsFunc = affectedKubernetesComponents
	provisionAndResolveComponentPath = component.ProvisionAndResolveComponentPath
	dependenciesForComponent         = dependencies.ForComponent
	getHooks                         = hooks.GetHooks
	runAllHooks                      = func(hookSet *hooks.Hooks, event hooks.HookEvent, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
		return hookSet.RunAll(event, atmosConfig, info, nil, nil)
	}
	runKubernetesCIHookFunc = runKubernetesCIHook
	newKubernetesSDKClient  = newSDKClient
)

func Execute(ctx *component.ExecutionContext, operation Operation) error {
	defer perf.Track(ctx.AtmosConfig, "kubernetes.ExecuteOperation")()

	info := ctx.ConfigAndStacksInfo
	command := ctx.SubCommand
	if command == "" {
		command = string(operation)
	}
	info.ComponentType = cfg.KubernetesComponentType
	info.SubCommand = command
	info.CliArgs = []string{cfg.KubernetesComponentType, command}

	atmosConfig, err := initCliConfig(info, true)
	if err != nil {
		return err
	}
	normalizeGlobalConfig(&atmosConfig)

	if info.All || info.Affected {
		return executeBulk(ctx, &atmosConfig, &info, operation)
	}

	if err := processStacksWithAuth(&atmosConfig, &info); err != nil {
		return err
	}
	if !info.ComponentIsEnabled {
		log.Info("Component is not enabled and skipped", "component", info.ComponentFromArg)
		return nil
	}

	source, err := validateAndResolveComponent(&atmosConfig, &info)
	if err != nil {
		return err
	}

	if err := renderManifestInputTemplates(&atmosConfig, info.ComponentSection); err != nil {
		return err
	}

	restoreEnv, err := prepareComponentEnvironment(&atmosConfig, &info)
	if err != nil {
		return err
	}
	defer restoreEnv()

	return runWithHooks(ctx, &atmosConfig, &info, operation, source)
}

// manifestSource bundles the resolved provider and component path used to load manifests.
type manifestSource struct {
	provider      string
	componentPath string
}

// runWithHooks runs the before/after hooks around loading manifests and executing the operation.
func runWithHooks(
	ctx *component.ExecutionContext,
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	operation Operation,
	source manifestSource,
) error {
	hookSet, err := getHooks(atmosConfig, info)
	if err != nil {
		return err
	}
	before, after := eventsForCommand(info.SubCommand, operation)
	if err := runAllHooks(hookSet, before, atmosConfig, info); err != nil {
		runKubernetesCIHookFunc(after, atmosConfig, info, nil, err)
		return err
	}

	objects, err := loadManifestObjects(source, info)
	if err != nil {
		runKubernetesCIHookFunc(after, atmosConfig, info, nil, err)
		return err
	}

	result, err := runOperation(ctx, atmosConfig, info, operation, objects)
	if err != nil {
		runKubernetesCIHookFunc(after, atmosConfig, info, result, err)
		return err
	}

	if err := runAllHooks(hookSet, after, atmosConfig, info); err != nil {
		runKubernetesCIHookFunc(after, atmosConfig, info, result, err)
		return err
	}
	runKubernetesCIHookFunc(after, atmosConfig, info, result, nil)
	return nil
}

// validateAndResolveComponent validates the component and resolves its on-disk path,
// auto-generating files when configured and confirming a usable input source exists.
func validateAndResolveComponent(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (manifestSource, error) {
	if err := (&ComponentProvider{}).ValidateComponent(info.ComponentSection); err != nil {
		return manifestSource{}, err
	}

	provider := resolveProvider(atmosConfig, info.ComponentSection)
	if provider != ProviderKubectl && provider != ProviderKustomize {
		return manifestSource{}, fmt.Errorf("%w: provider must be %q or %q", errUtils.ErrComponentValidationFailed, ProviderKubectl, ProviderKustomize)
	}

	componentPath, componentPathExists, err := resolveComponentPath(atmosConfig, info)
	if err != nil {
		return manifestSource{}, err
	}

	if err := maybeAutoGenerateFiles(atmosConfig, info, componentPath); err != nil {
		return manifestSource{}, err
	}

	if err := ensureComponentInputExists(atmosConfig, info, componentPath, componentPathExists); err != nil {
		return manifestSource{}, err
	}

	return manifestSource{provider: provider, componentPath: componentPath}, nil
}

// ensureComponentInputExists verifies the component has a usable input source:
// an existing component directory or configured manifests/paths.
func ensureComponentInputExists(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	componentPath string,
	componentPathExists bool,
) error {
	if !componentPathExists {
		if stat, statErr := os.Stat(componentPath); statErr == nil && stat.IsDir() {
			componentPathExists = true
		}
	}

	manifestsInput, err := asAnySlice(info.ComponentSection["manifests"])
	if err != nil {
		return err
	}
	pathsInput, err := asStringSlice(info.ComponentSection["paths"])
	if err != nil {
		return err
	}
	if componentPathExists || len(manifestsInput) > 0 || len(pathsInput) > 0 {
		return nil
	}

	basePath, _ := u.GetComponentBasePath(atmosConfig, cfg.KubernetesComponentType)
	return fmt.Errorf(
		"%w: '%s' points to the Kubernetes component '%s', but it does not exist in '%s'",
		errUtils.ErrInvalidComponent,
		info.ComponentFromArg,
		info.FinalComponent,
		basePath,
	)
}

// loadManifestObjects loads the Kubernetes objects for the component from the
// resolved component path and configured manifest sources.
func loadManifestObjects(source manifestSource, info *schema.ConfigAndStacksInfo) ([]*unstructured.Unstructured, error) {
	loader := manifestLoader{
		componentPath: source.componentPath,
		provider:      source.provider,
	}
	objects, err := loader.Load(info.ComponentSection)
	if err != nil {
		return nil, err
	}
	if len(objects) == 0 {
		return nil, fmt.Errorf("%w: no Kubernetes manifests found for component %q", errUtils.ErrInvalidComponent, info.ComponentFromArg)
	}
	return objects, nil
}

// runOperation dispatches the rendered objects to the requested Kubernetes operation.
// Apply/deploy resolves the configured provision target (cluster by default, or a
// delivery destination such as git); render/diff/delete remain cluster-local.
func runOperation(ctx *component.ExecutionContext, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, operation Operation, objects []*unstructured.Unstructured) (*schema.KubernetesCIResult, error) {
	result := newKubernetesCIResult(info, objects)
	results, err := executeKubernetesOperation(ctx, atmosConfig, info, operation, objects)
	if operation == OperationValidate && err != nil && len(results) == 0 && errors.Is(err, errUtils.ErrKubernetesValidationFailed) {
		results = objectsToResults("invalid", objects)
	}
	result.Actions = objectCIResults(results)
	result.ActionCounts = countKubernetesActions(result.Actions)
	return result, err
}

func executeKubernetesOperation(ctx *component.ExecutionContext, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, operation Operation, objects []*unstructured.Unstructured) ([]objectResult, error) {
	switch operation {
	case OperationRender:
		err := renderObjects(objects, resolveRenderOptions(ctx.Flags, info.ComponentSection))
		if err == nil {
			return objectsToResults("rendered", objects), nil
		}
		return nil, err
	case OperationDiff:
		return runDiff(objects)
	case OperationApply:
		// Auto-gate apply/deploy: fail fast on structurally invalid manifests
		// before contacting the cluster or delivering to a provision target.
		if err := validateObjectsStructural(objects); err != nil {
			return nil, err
		}
		return deliverApply(atmosConfig, info, ctx.Flags, objects)
	case OperationDelete:
		return runDelete(objects)
	case OperationValidate:
		return runValidate(objects, resolveValidateOptions(ctx.Flags))
	default:
		return nil, fmt.Errorf("%w: %q", errUtils.ErrKubernetesUnsupportedOperation, operation)
	}
}

func normalizeGlobalConfig(atmosConfig *schema.AtmosConfiguration) {
	if atmosConfig.Components.Kubernetes.BasePath == "" {
		atmosConfig.Components.Kubernetes.BasePath = DefaultConfig().BasePath
	}
	if atmosConfig.Components.Kubernetes.Provider == "" {
		atmosConfig.Components.Kubernetes.Provider = DefaultConfig().Provider
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

func resolveProvider(atmosConfig *schema.AtmosConfiguration, componentSection map[string]any) string {
	if provider, ok := componentSection["provider"].(string); ok && provider != "" {
		return provider
	}
	if atmosConfig.Components.Kubernetes.Provider != "" {
		return atmosConfig.Components.Kubernetes.Provider
	}
	return DefaultConfig().Provider
}

func resolveComponentPath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, bool, error) {
	initialPath, err := u.GetComponentPath(atmosConfig, cfg.KubernetesComponentType, info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return "", false, errors.Join(errUtils.ErrPathResolution, fmt.Errorf("component path: %w", err))
	}

	provisionCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return provisionAndResolveComponentPath(provisionCtx, atmosConfig, info, cfg.KubernetesComponentType, initialPath)
}

func maybeAutoGenerateFiles(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string) error {
	if !atmosConfig.Components.Kubernetes.AutoGenerateFiles {
		return nil
	}

	generateSection := tfgenerate.GetGenerateSectionFromComponent(info.ComponentSection)
	if generateSection == nil {
		return nil
	}

	if err := os.MkdirAll(componentPath, dirPerm); err != nil {
		return fmt.Errorf("%w: %q: %w", errUtils.ErrKubernetesComponentDir, componentPath, err)
	}

	templateContext := tfgenerate.BuildTemplateContext(info)
	_, err := tfgenerate.GenerateFiles(generateSection, componentPath, templateContext, tfgenerate.GenerateConfig{})
	return err
}

func runApply(objects []*unstructured.Unstructured) ([]objectResult, error) {
	client, err := newKubernetesSDKClient()
	if err != nil {
		return nil, err
	}
	results, err := client.Apply(context.Background(), objects)
	if err != nil {
		return nil, err
	}
	printResults(results)
	return results, nil
}

func runDelete(objects []*unstructured.Unstructured) ([]objectResult, error) {
	client, err := newKubernetesSDKClient()
	if err != nil {
		return nil, err
	}
	results, err := client.Delete(context.Background(), objects)
	if err != nil {
		return nil, err
	}
	printResults(results)
	return results, nil
}

func runDiff(objects []*unstructured.Unstructured) ([]objectResult, error) {
	client, err := newKubernetesSDKClient()
	if err != nil {
		return nil, err
	}
	results, err := client.Diff(context.Background(), objects)
	if err != nil {
		return nil, err
	}
	printResults(results)
	return results, nil
}

func printResults(results []objectResult) {
	for _, result := range results {
		line := fmt.Sprintf("%s %s", result.Action, objectResultRef(&result))
		// For plan/diff, print the unified diff body beneath the action line.
		// Empty for apply/delete/no-change/Secret objects. Lines with diffs
		// remain data output so the diff body stays pipeable.
		if result.Diff != "" {
			_ = data.Writef("%s\n", line)
			_ = data.Writef("%s\n", result.Diff)
			continue
		}
		ui.Success(line)
	}
}

func objectResultRef(result *objectResult) string {
	if result.Namespace == "" {
		return fmt.Sprintf("%s %s", result.Resource, result.Name)
	}
	return fmt.Sprintf("%s %s/%s", result.Resource, result.Namespace, result.Name)
}

func eventsFor(operation Operation) (hooks.HookEvent, hooks.HookEvent) {
	return eventsForCommand(string(operation), operation)
}

func eventsForCommand(command string, operation Operation) (hooks.HookEvent, hooks.HookEvent) {
	switch command {
	case "plan":
		return hooks.BeforeKubernetesPlan, hooks.AfterKubernetesPlan
	case "deploy":
		return hooks.BeforeKubernetesDeploy, hooks.AfterKubernetesDeploy
	}
	switch operation {
	case OperationRender:
		return hooks.BeforeKubernetesRender, hooks.AfterKubernetesRender
	case OperationDiff:
		return hooks.BeforeKubernetesDiff, hooks.AfterKubernetesDiff
	case OperationApply:
		return hooks.BeforeKubernetesApply, hooks.AfterKubernetesApply
	case OperationDelete:
		return hooks.BeforeKubernetesDelete, hooks.AfterKubernetesDelete
	case OperationValidate:
		return hooks.BeforeKubernetesValidate, hooks.AfterKubernetesValidate
	default:
		return hooks.HookEvent(""), hooks.HookEvent("")
	}
}

func newKubernetesCIResult(info *schema.ConfigAndStacksInfo, objects []*unstructured.Unstructured) *schema.KubernetesCIResult {
	result := &schema.KubernetesCIResult{
		ObjectsTotal: len(objects),
		ActionCounts: map[string]int{},
	}
	if info != nil {
		result.Stack = info.Stack
		result.Component = info.ComponentFromArg
		result.Command = info.SubCommand
	}
	return result
}

func objectCIResults(results []objectResult) []schema.KubernetesObjectCIResult {
	out := make([]schema.KubernetesObjectCIResult, 0, len(results))
	for _, result := range results {
		out = append(out, schema.KubernetesObjectCIResult{
			Action:    result.Action,
			Resource:  result.Resource,
			Namespace: result.Namespace,
			Name:      result.Name,
			Diff:      result.Diff,
		})
	}
	return out
}

func countKubernetesActions(results []schema.KubernetesObjectCIResult) map[string]int {
	counts := make(map[string]int)
	for _, result := range results {
		counts[result.Action]++
	}
	return counts
}

func objectsToResults(action string, objects []*unstructured.Unstructured) []objectResult {
	results := make([]objectResult, 0, len(objects))
	for _, obj := range objects {
		results = append(results, objectResult{
			Action:    action,
			Resource:  resourceID(obj),
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
		})
	}
	return results
}

func runKubernetesCIHook(
	event hooks.HookEvent,
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	result *schema.KubernetesCIResult,
	commandErr error,
) {
	if result == nil {
		result = newKubernetesCIResult(info, nil)
	}
	result.ExitCode = errUtils.GetExitCode(commandErr)
	if commandErr != nil {
		result.Error = commandErr.Error()
	}
	if err := hooks.RunCIHooks(&hooks.RunCIHooksOptions{
		Event:        event,
		AtmosConfig:  atmosConfig,
		Info:         info,
		ForceCIMode:  viper.GetBool("ci"),
		CommandError: commandErr,
		ExitCode:     result.ExitCode,
		Aggregate:    result,
	}); err != nil {
		log.Warn("Kubernetes CI summary skipped", "event", event, "error", err)
	}
}
