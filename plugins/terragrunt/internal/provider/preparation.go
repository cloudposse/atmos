package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/gofrs/flock"
	"github.com/zclconf/go-cty/cty"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/engine"
	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
	"github.com/cloudposse/atmos/sdk/component"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

func (p *Provider) componentRequest(ctx context.Context, request *pluginv1.ComponentRequest) (model.Request, error) {
	if err := validate(ctx, request); err != nil {
		return model.Request{}, err
	}
	req, err := operation(request.RootDirectory, request.Provider, request.Operation, request.Invocation)
	if err != nil {
		return req, err
	}
	if !filepath.IsLocal(request.Source) {
		return req, ErrUnsupported
	}
	req.Path = filepath.Join(request.RootDirectory, filepath.FromSlash(request.Source))
	if err := withFeatures(&req, request.ProviderConfiguration); err != nil {
		return req, err
	}
	var config struct {
		Settings struct {
			Terragrunt struct {
				GenerationRoot string `json:"generation_root"`
			} `json:"terragrunt"`
		} `json:"settings"`
	}
	if err := decode(request.Configuration, &config); err != nil {
		return req, err
	}
	if generation := config.Settings.Terragrunt.GenerationRoot; generation != "" {
		if !filepath.IsLocal(generation) {
			return req, ErrUnsupported
		}
		req.GenerationRoot = filepath.Join(req.BasePath, filepath.FromSlash(generation))
	}
	return req, nil
}

// Prepare resolves current inputs after dependencies and returns literal files.
// Terraform execution remains owned by Atmos.
func (p *Provider) Prepare(ctx context.Context, request *pluginv1.ComponentRequest) (*pluginv1.PrepareComponentResponse, error) {
	ctx, sink := withLogs(ctx, request.GetInvocation(), request.GetRootDirectory(), request.GetConfiguration(), request.GetProviderConfiguration())
	response, err := p.prepare(ctx, request)
	err = scoped(err, "prepare", request.GetRootDirectory(), request.GetAddress())
	if response != nil {
		response.Logs = logEntries(sink)
	}
	return response, withFailureLogs(err, sink)
}

func (p *Provider) prepare(ctx context.Context, request *pluginv1.ComponentRequest) (*pluginv1.PrepareComponentResponse, error) {
	// Executable mode runs units with the terragrunt executable; native preparation must never mix in.
	if request.GetOperation().GetMode() == ModeExecutable {
		return nil, rpcError(&model.Problem{
			Code: model.CodeUnsupportedSetting, Summary: "Units in executable mode are not prepared natively", Safe: true, Err: ErrUnsupported,
			Hints: []string{"Executable mode runs each unit with the terragrunt executable. Set `components.terraform.terragrunt.mode` to `native` to adapt units to Atmos instead."},
		})
	}
	req, err := p.componentRequest(ctx, request)
	if err != nil {
		return nil, rpcError(err)
	}
	if !preparationEffects(request.GetEffects()) {
		return nil, rpcError(ErrUnsupported)
	}
	// The role is resolved before any evaluation that can call AWS functions, so the
	// probe runs before stack generation.
	if req.Phase == "iam" {
		return p.probeIAM(ctx, &req)
	}
	// All provider processes generating the same stack must retain the lock
	// until the subsequent unit parse has finished.
	unlock, err := p.generateLocked(ctx, req)
	if err != nil {
		return nil, rpcError(err)
	}
	defer unlock()
	result, err := p.evaluate(ctx, req)
	if err != nil {
		return nil, rpcError(err)
	}
	response, err := preparedResponse(req, result)
	if err != nil {
		return nil, rpcError(err)
	}
	return response, nil
}

func preparationEffects(effects *pluginv1.ComponentEffects) bool {
	return effects.GetCommands() && effects.GetWrites() && effects.GetStateReads() && effects.GetNetwork() && effects.GetSecrets()
}

// generateLocked generates the owning stack under a cross-process lock and
// returns the function that releases it. Units outside a stack need no lock.
func (p *Provider) generateLocked(ctx context.Context, req model.Request) (func(), error) {
	if req.GenerationRoot == "" {
		return func() {}, nil
	}
	lock, err := generationLock(ctx, req)
	if err != nil {
		return nil, err
	}
	generation := req
	generation.Path, generation.Phase, generation.Command = req.GenerationRoot, "discover", "generate"
	if _, err := p.evaluate(ctx, generation); err != nil {
		_ = lock.Unlock()
		return nil, err
	}
	return func() { _ = lock.Unlock() }, nil
}

// generationLock keys the lock by the physical generation root, so processes
// that spell the project root differently still serialize.
func generationLock(ctx context.Context, req model.Request) (*flock.Flock, error) {
	root, err := filepath.EvalSymlinks(req.BasePath)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(req.BasePath, req.GenerationRoot)
	if err != nil {
		return nil, err
	}
	generationPath := filepath.Join(root, rel)
	if resolved, resolveErr := filepath.EvalSymlinks(generationPath); resolveErr == nil {
		generationPath = resolved
	}
	key := sha256.Sum256([]byte(generationPath))
	lock := flock.New(filepath.Join(os.TempDir(), fmt.Sprintf("atmos-terragrunt-%x.lock", key)))
	locked, err := lock.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, ctx.Err()
	}
	return lock, nil
}

func preparedResponse(req model.Request, result *model.Resolution) (*pluginv1.PrepareComponentResponse, error) {
	values := result.Inputs
	if values == nil {
		values = map[string]any{}
	}
	inputs, err := encodeSensitive(values, result.SensitiveInputs)
	if err != nil {
		return nil, err
	}
	state, err := stateIdentity(req, result)
	if err != nil {
		return nil, err
	}
	watched, err := sources(req.BasePath, result)
	if err != nil {
		return nil, err
	}
	response := &pluginv1.PrepareComponentResponse{Engine: engineName(req.Tofu), ModuleSource: result.Source, WorkingDirectory: result.WorkingDir, Inputs: inputs, State: state, Sources: watched, BackendGenerated: result.BackendGenerated, Execution: executionResponse(result.Execution), RegistryResolutionNanos: result.RegistryResolutionNanos}
	if err := parityResponse(response, req, result); err != nil {
		return nil, rpcError(err)
	}
	if response.Iam, err = iamResponse(result.IAM); err != nil {
		return nil, rpcError(err)
	}
	if err = encodeRegistryAuth(response, result.RegistryAuth); err != nil {
		return nil, rpcError(err)
	}
	response.Diagnostics = unitWarnings(req.BasePath, result.Warnings)
	for _, name := range slices.Sorted(maps.Keys(result.Generated)) {
		policy := result.GeneratedPolicies[name]
		if policy == "" {
			policy = "overwrite"
		}
		response.Files = append(response.Files, &pluginv1.ComponentGeneratedFile{Path: name, Content: []byte(result.Generated[name]), IfExists: policy, Sensitive: result.SensitiveGenerated[name], UnitFileCopy: result.UnitFileCopies[name]})
	}
	return response, nil
}

func encodeRegistryAuth(response *pluginv1.PrepareComponentResponse, auth *model.RegistryAuth) error {
	if auth == nil {
		return nil
	}
	value, err := component.Encode(cty.StringVal(auth.Authorization).Mark(component.Sensitive))
	if err != nil {
		return err
	}
	response.ModuleSourceAuthorization = value
	response.ModuleSourceAuthOrigin = auth.Origin
	return nil
}

func (p *Provider) probeIAM(ctx context.Context, req *model.Request) (*pluginv1.PrepareComponentResponse, error) {
	result, err := p.evaluate(ctx, *req)
	if err != nil {
		return nil, rpcError(err)
	}
	iam, err := iamResponse(result.IAM)
	if err != nil {
		return nil, rpcError(err)
	}
	return &pluginv1.PrepareComponentResponse{Iam: iam, Diagnostics: unitWarnings(req.BasePath, result.Warnings)}, nil
}

func stateIdentity(req model.Request, result *model.Resolution) (*pluginv1.ComponentStateIdentity, error) {
	backend := result.Backend
	if backend == nil {
		backend = map[string]any{}
	}
	backendType := result.BackendType
	if backendType == "" {
		backendType = "local"
	}
	encoded, err := encode(backend)
	if err != nil {
		return nil, err
	}
	workspace := req.Environment["TF_WORKSPACE"]
	if workspace == "" {
		workspace = "default"
	}
	return &pluginv1.ComponentStateIdentity{BackendType: backendType, Backend: encoded, Workspace: workspace, WorkingDirectory: result.WorkingDir}, nil
}

// Resolve reads backend identity in an overlay, independently of dependency outputs.
func (p *Provider) Resolve(ctx context.Context, request *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
	ctx, sink := withLogs(ctx, request.GetInvocation(), request.GetRootDirectory(), request.GetConfiguration(), request.GetProviderConfiguration())
	response, err := p.resolveState(ctx, request)
	err = scoped(err, "resolve state", request.GetRootDirectory(), request.GetAddress())
	if response != nil {
		response.Logs = logEntries(sink)
	}
	return response, withFailureLogs(err, sink)
}

func (p *Provider) resolveState(ctx context.Context, request *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
	req, err := p.componentRequest(ctx, request)
	if err != nil {
		return nil, rpcError(err)
	}
	req.Phase = "backend"
	result, err := p.evaluate(ctx, req)
	if err != nil {
		return nil, rpcError(err)
	}
	state, err := stateIdentity(req, result)
	if err != nil {
		return nil, rpcError(err)
	}
	watched, err := sources(req.BasePath, result)
	if err != nil {
		return nil, rpcError(err)
	}
	return &pluginv1.ResolveComponentStateResponse{State: state, Sources: watched}, nil
}

// executionResponse hands evaluated hooks and extra arguments to the host unchanged and in
// declaration order. The plugin never runs them.
func executionResponse(execution *model.Execution) *pluginv1.ComponentExecutionConfiguration {
	if execution == nil {
		return nil
	}
	result := &pluginv1.ComponentExecutionConfiguration{
		BeforeHooks: hooksResponse(execution.BeforeHooks),
		AfterHooks:  hooksResponse(execution.AfterHooks),
		ErrorHooks:  hooksResponse(execution.ErrorHooks),
		Apply:       executionResponse(execution.Apply),
	}
	for i := range execution.ExtraArguments {
		extra := &execution.ExtraArguments[i]
		result.ExtraArguments = append(result.ExtraArguments, &pluginv1.ComponentExtraArguments{
			Commands:         slices.Clone(extra.Commands),
			Arguments:        slices.Clone(extra.Arguments),
			RequiredVarFiles: slices.Clone(extra.RequiredVarFiles),
			OptionalVarFiles: slices.Clone(extra.OptionalVarFiles),
			Environment:      maps.Clone(extra.Environment),
		})
	}
	return result
}

func hooksResponse(hooks []model.Hook) []*pluginv1.ComponentHook {
	var result []*pluginv1.ComponentHook
	for i := range hooks {
		hook := &hooks[i]
		result = append(result, &pluginv1.ComponentHook{
			Name:             hook.Name,
			Commands:         slices.Clone(hook.Commands),
			Execute:          slices.Clone(hook.Execute),
			WorkingDirectory: hook.WorkingDirectory,
			RunOnError:       hook.RunOnError,
			SuppressStdout:   hook.SuppressStdout,
			OnErrors:         slices.Clone(hook.OnErrors),
		})
	}
	return result
}

// iamResponse hands the role to the host, which assumes it. The web identity token travels as a
// sensitive value.
func iamResponse(iam *model.IAM) (*pluginv1.ComponentIam, error) {
	if iam == nil {
		return nil, nil
	}
	response := &pluginv1.ComponentIam{Role: iam.Role, SessionName: iam.SessionName, DurationSeconds: uint32(max(0, min(iam.DurationSeconds, math.MaxUint32)))}
	if iam.WebIdentityToken != "" {
		token, err := component.Encode(cty.StringVal(iam.WebIdentityToken).Mark(component.Sensitive))
		if err != nil {
			return nil, err
		}
		response.WebIdentityToken = token
	}
	return response, nil
}

// featureEnvironment is the variable Terragrunt reads feature overrides from.
const featureEnvironment = "TG_FEATURE"

// featureSettings renders the `features` setting as strings. Values must be booleans, numbers, or
// strings, because a feature is a scalar.
func featureSettings(value any) (map[string]string, error) {
	if value == nil {
		return nil, nil
	}
	entries, ok := value.(map[string]any)
	if !ok {
		return nil, settingProblem("features", "Invalid value for `components.terraform.terragrunt.features`", "Set `features` to a map of feature names to booleans, numbers, or strings.")
	}
	result := make(map[string]string, len(entries))
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		switch typed := entries[name].(type) {
		case bool:
			result[name] = strconv.FormatBool(typed)
		case string:
			result[name] = typed
		case json.Number:
			result[name] = typed.String()
		default:
			return nil, settingProblem("features."+name, fmt.Sprintf("Invalid value for the `%s` feature override", name), "Use a boolean, a number, or a string.")
		}
	}
	return result, nil
}

// withFeatures sets the request's feature overrides. Overrides from the `features` setting apply
// first; TG_FEATURE in the invocation environment replaces them per name, as environment
// variables outrank configuration files.
func withFeatures(req *model.Request, configuration *pluginv1.ComponentValue) error {
	var config map[string]any
	if configuration != nil {
		if err := decode(configuration, &config); err != nil {
			return err
		}
	}
	features, err := featureSettings(config["features"])
	if err != nil {
		return err
	}
	if raw := req.Environment[featureEnvironment]; raw != "" {
		fromEnvironment, err := engine.ParseFeatureEnvironment(raw)
		if err != nil {
			return err
		}
		if features == nil {
			features = map[string]string{}
		}
		maps.Copy(features, fromEnvironment)
	}
	// Options of the command line rank above the environment and the configuration, per name.
	if len(req.Features) > 0 {
		if features == nil {
			features = map[string]string{}
		}
		maps.Copy(features, req.Features)
	}
	req.Features = features
	return nil
}

// unitEngine is the engine a unit runs with: its terraform_binary when it sets one, otherwise the
// host's selection.
func unitEngine(req model.Request, resolved *model.Resolution) string {
	if resolved != nil && resolved.TerraformBinary != "" {
		return engineName(resolved.TerraformBinary)
	}
	return engineName(req.Tofu)
}

// discoveryParity adds what the host needs before a unit is prepared: the executable the unit
// selects, as the component's `command` so a stack-level `command` still overrides it, and the
// exclude block that bulk commands consult.
func discoveryParity(config map[string]any, resolved *model.Resolution) {
	if resolved.TerraformBinary != "" {
		config["command"] = resolved.TerraformBinary
	}
	if resolved.Exclude == nil {
		return
	}
	settings, _ := config["settings"].(map[string]any)
	terragrunt, _ := settings["terragrunt"].(map[string]any)
	terragrunt["exclude"] = map[string]any{
		"actions":              slices.Clone(resolved.Exclude.Actions),
		"exclude_dependencies": resolved.Exclude.ExcludeDependencies,
	}
}

// operationCommand is the command name Terragrunt matches exclude actions against.
func operationCommand(command string, arguments []string) string {
	return engine.OperationCommand(command, arguments)
}

// parityResponse adds the evaluated terraform_binary, errors, exclude, and feature flags to a
// Prepare response. Exclude carries only the outcome for this operation: the block's no_run
// semantics are applied here, so it is set exactly when the host must not run the command.
func parityResponse(response *pluginv1.PrepareComponentResponse, req model.Request, resolved *model.Resolution) error {
	response.TerraformBinary = resolved.TerraformBinary
	response.FeatureFlags = maps.Clone(resolved.FeatureFlags)
	errorsResponse, err := errorsResponse(resolved.Errors)
	if err != nil {
		return err
	}
	response.Errors = errorsResponse
	command := operationCommand(req.Command, req.Arguments)
	if engine.PreventsRun(resolved.Exclude, command) {
		response.Exclude = &pluginv1.ComponentExclude{Condition: true, Actions: []string{command}, ExcludeDependencies: resolved.Exclude.ExcludeDependencies}
	}
	return nil
}

func errorsResponse(policies *model.Errors) (*pluginv1.ComponentErrorsConfiguration, error) {
	if policies == nil {
		return nil, nil
	}
	result := &pluginv1.ComponentErrorsConfiguration{}
	for i := range policies.Retry {
		retry := &policies.Retry[i]
		result.Retry = append(result.Retry, &pluginv1.ComponentRetryPolicy{
			Name: retry.Name, RetryableErrors: slices.Clone(retry.Patterns), MaxAttempts: uint32(retry.MaxAttempts), SleepIntervalSeconds: uint32(retry.SleepIntervalSec), //nolint:gosec // Validated as positive when the unit is evaluated.
		})
	}
	for i := range policies.Ignore {
		ignore := &policies.Ignore[i]
		signals := make(map[string]*pluginv1.ComponentValue, len(ignore.Signals))
		for _, name := range slices.Sorted(maps.Keys(ignore.Signals)) {
			value, err := encode(ignore.Signals[name])
			if err != nil {
				return nil, err
			}
			signals[name] = value
		}
		result.Ignore = append(result.Ignore, &pluginv1.ComponentIgnorePolicy{Name: ignore.Name, IgnorableErrors: slices.Clone(ignore.Patterns), Message: ignore.Message, Signals: signals})
	}
	return result, nil
}

// unitWarnings reports findings about one unit as warnings, with paths relative to the project.
func unitWarnings(root string, warnings []model.Warning) []*pluginv1.Diagnostic {
	relativeWarnings := slices.Clone(warnings)
	for i := range relativeWarnings {
		if relativeWarnings[i].Path == "" {
			continue
		}
		if path, err := relative(root, relativeWarnings[i].Path); err == nil {
			relativeWarnings[i].Path = path
		}
	}
	diagnostics := warningDiagnostics(relativeWarnings)
	for _, diagnostic := range diagnostics {
		diagnostic.Detail.Operation = "prepare"
	}
	return diagnostics
}
