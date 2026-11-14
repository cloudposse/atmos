package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/engine"
	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
	"github.com/cloudposse/atmos/sdk/component"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

func componentRequest(t *testing.T) *pluginv1.ComponentRequest {
	t.Helper()
	config, err := encode(map[string]any{"settings": map[string]any{"terragrunt": map[string]any{"generation_root": "live/terragrunt.stack.hcl"}}})
	require.NoError(t, err)
	options, err := encode(map[string]any{"enabled": true, "discovery": map[string]any{"enabled": true}})
	require.NoError(t, err)
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return &pluginv1.ComponentRequest{Provider: "terragrunt", RootDirectory: root, Source: "live/.terragrunt-stack/app/terragrunt.hcl", Configuration: config, ProviderConfiguration: options, Operation: &pluginv1.ComponentOperation{Mode: "native", Command: "apply"}, Effects: &pluginv1.ComponentEffects{Commands: true, Network: true, Secrets: true, Writes: true, StateReads: true}, Invocation: &pluginv1.Invocation{Environment: map[string]string{"ATMOS_TERRAFORM_BINARY": "tofu", "TF_WORKSPACE": "stage"}}}
}

func TestPreparationAndStateUseSeparatePhases(t *testing.T) {
	request := componentRequest(t)
	phases := []string{}
	p := New(func(_ context.Context, req model.Request) (*model.Resolution, error) {
		require.Equal(t, "stage", req.Environment["TF_WORKSPACE"])
		require.Equal(t, "tofu", req.Tofu)
		phases = append(phases, req.Phase)
		if req.Phase == "discover" {
			require.Equal(t, "generate", req.Command)
			require.Equal(t, filepath.Join(request.RootDirectory, "live/terragrunt.stack.hcl"), req.Path)
		}
		return &model.Resolution{Source: filepath.Join(request.RootDirectory, "module"), WorkingDir: filepath.Join(request.RootDirectory, "cache"), Inputs: map[string]any{"payload": "fresh"}, BackendType: "local", Backend: map[string]any{"path": "relative.tfstate"}, Generated: map[string]string{"z.tf": "{{ literal }}", "a.tf": "${expression}"}, Files: []string{filepath.Join(request.RootDirectory, "root.hcl")}}, nil
	})
	result, err := p.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, []string{"discover", ""}, phases)
	require.Equal(t, "opentofu", result.Engine)
	require.Equal(t, "stage", result.State.Workspace)
	require.Equal(t, "a.tf", result.Files[0].Path)
	require.Equal(t, "z.tf", result.Files[1].Path)
	require.Equal(t, "{{ literal }}", string(result.Files[1].Content))
	phases = nil
	state, err := p.Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, []string{"backend"}, phases)
	require.Equal(t, result.State.WorkingDirectory, state.State.WorkingDirectory)
}

func TestIAMProbeReturnsOnlyRoleSettings(t *testing.T) {
	request := componentRequest(t)
	request.Operation.Mode = "iam-probe"
	p := New(func(_ context.Context, req model.Request) (*model.Resolution, error) {
		require.Equal(t, "iam", req.Phase)
		return &model.Resolution{IAM: &model.IAM{Role: "arn:aws:iam::1:role/x"}}, nil
	})
	result, err := p.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "arn:aws:iam::1:role/x", result.Iam.Role)
	require.Empty(t, result.ModuleSource)
	require.Nil(t, result.State)
}

func TestPreparationCarriesSensitiveRegistryAuthorization(t *testing.T) {
	request := componentRequest(t)
	p := New(func(_ context.Context, _ model.Request) (*model.Resolution, error) {
		return &model.Resolution{Source: "https://registry.example/archive.tar.gz", WorkingDir: request.RootDirectory, RegistryAuth: &model.RegistryAuth{Origin: "https://registry.example", Authorization: "Bearer secret"}, Inputs: map[string]any{}, BackendType: "local", Backend: map[string]any{}}, nil
	})
	response, err := p.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "https://registry.example", response.ModuleSourceAuthOrigin)
	require.Len(t, response.ModuleSourceAuthorization.SensitivePaths, 1)
	require.Empty(t, response.ModuleSourceAuthorization.SensitivePaths[0].Steps)
	require.NotContains(t, response.ModuleSource, "secret")
}

func TestRequestsRejectUnsupportedBeforeEvaluation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*pluginv1.ComponentRequest)
	}{
		{"provider", func(r *pluginv1.ComponentRequest) { r.Provider = "other" }},
		{"relative root", func(r *pluginv1.ComponentRequest) { r.RootDirectory = "relative" }},
		{"missing operation", func(r *pluginv1.ComponentRequest) { r.Operation = nil }},
		{"cli fallback", func(r *pluginv1.ComponentRequest) { r.Operation.Mode = "cli" }},
		{"options", func(r *pluginv1.ComponentRequest) {
			r.Operation.Options = map[string]*pluginv1.ComponentValue{"unsupported": {}}
		}},
		{"source traversal", func(r *pluginv1.ComponentRequest) { r.Source = "../escape" }},
		{"missing effects", func(r *pluginv1.ComponentRequest) { r.Effects = nil }},
		{"partial effects", func(r *pluginv1.ComponentRequest) { r.Effects.StateReads = false }},
		{"bad settings", func(r *pluginv1.ComponentRequest) { r.ProviderConfiguration = &pluginv1.ComponentValue{} }},
		{"bad configuration", func(r *pluginv1.ComponentRequest) { r.Configuration = &pluginv1.ComponentValue{} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := componentRequest(t)
			test.change(r)
			p := New(func(context.Context, model.Request) (*model.Resolution, error) {
				t.Fatal("evaluation must not start")
				return nil, nil
			})
			_, err := p.Prepare(t.Context(), r)
			require.Error(t, err)
		})
	}
	p := New(func(context.Context, model.Request) (*model.Resolution, error) {
		t.Fatal("evaluation must not start")
		return nil, nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := p.Validate(ctx, componentRequest(t))
	require.Equal(t, codes.Canceled, status.Code(err))
	// Prepare and Resolve keep the validation status instead of re-mapping it to InvalidArgument.
	_, err = p.Prepare(ctx, componentRequest(t))
	require.Equal(t, codes.Canceled, status.Code(err))
	unsupported := componentRequest(t)
	unsupported.Operation.Mode = "cli"
	_, err = p.Resolve(t.Context(), unsupported)
	require.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestEvaluatorErrorsDoNotExposePayloads(t *testing.T) {
	for _, test := range []struct {
		err  error
		code codes.Code
	}{{errors.New("secret-evaluated-value"), codes.InvalidArgument}, {context.Canceled, codes.Canceled}, {context.DeadlineExceeded, codes.DeadlineExceeded}, {ErrUnsupported, codes.Unimplemented}} {
		p := New(func(context.Context, model.Request) (*model.Resolution, error) { return nil, test.err })
		_, err := p.Prepare(t.Context(), componentRequest(t))
		require.Equal(t, test.code, status.Code(err))
		require.NotContains(t, err.Error(), "secret-evaluated-value")
	}
}

func TestPrepareReturnsExecutionInDeclarationOrder(t *testing.T) {
	request := componentRequest(t)
	execution := &model.Execution{
		ExtraArguments: []model.ExtraArguments{
			{Commands: []string{"plan"}, Arguments: []string{"-lock=false"}},
			{Commands: []string{"apply", "plan"}, RequiredVarFiles: []string{"a.tfvars"}, OptionalVarFiles: []string{"b.tfvars"}, Environment: map[string]string{"TF_LOG": "INFO"}},
		},
		BeforeHooks: []model.Hook{
			{Name: "second", Commands: []string{"plan"}, Execute: []string{"tool", "2"}},
			{Name: "first", Commands: []string{"plan"}, Execute: []string{"tool", "1"}, WorkingDirectory: "sub", SuppressStdout: true},
		},
		AfterHooks: []model.Hook{{Name: "after", Commands: []string{"plan"}, Execute: []string{"tool"}, RunOnError: true}},
		ErrorHooks: []model.Hook{{Name: "oops", Commands: []string{"apply"}, Execute: []string{"tool"}, OnErrors: []string{"x", "y"}}},
	}
	p := New(func(_ context.Context, req model.Request) (*model.Resolution, error) {
		return &model.Resolution{Execution: execution, Source: filepath.Join(request.RootDirectory, "module"), WorkingDir: request.RootDirectory, BackendType: "local", Backend: map[string]any{}}, nil
	})
	result, err := p.Prepare(t.Context(), request)
	require.NoError(t, err)
	got := result.GetExecution()
	require.NotNil(t, got)

	require.Len(t, got.ExtraArguments, 2)
	require.Equal(t, []string{"-lock=false"}, got.ExtraArguments[0].Arguments)
	require.Equal(t, []string{"apply", "plan"}, got.ExtraArguments[1].Commands)
	require.Equal(t, []string{"a.tfvars"}, got.ExtraArguments[1].RequiredVarFiles)
	require.Equal(t, []string{"b.tfvars"}, got.ExtraArguments[1].OptionalVarFiles)
	require.Equal(t, map[string]string{"TF_LOG": "INFO"}, got.ExtraArguments[1].Environment)

	require.Len(t, got.BeforeHooks, 2)
	require.Equal(t, "second", got.BeforeHooks[0].Name)
	require.Equal(t, "first", got.BeforeHooks[1].Name)
	require.Equal(t, "sub", got.BeforeHooks[1].WorkingDirectory)
	require.True(t, got.BeforeHooks[1].SuppressStdout)
	require.Equal(t, []string{"tool", "1"}, got.BeforeHooks[1].Execute)
	require.Len(t, got.AfterHooks, 1)
	require.True(t, got.AfterHooks[0].RunOnError)
	require.Len(t, got.ErrorHooks, 1)
	require.Equal(t, []string{"x", "y"}, got.ErrorHooks[0].OnErrors)
}

// The apply-time evaluation crosses the boundary distinctly, including an empty one, which
// means apply runs nothing rather than the plan-time metadata.
func TestPrepareReturnsApplyTimeExecution(t *testing.T) {
	request := componentRequest(t)
	execution := &model.Execution{
		BeforeHooks: []model.Hook{{Name: "plan_only", Commands: []string{"plan"}, Execute: []string{"tool", "plan"}}},
		Apply:       &model.Execution{AfterHooks: []model.Hook{{Name: "notify", Commands: []string{"apply"}, Execute: []string{"tool", "notify"}}}},
	}
	p := New(func(context.Context, model.Request) (*model.Resolution, error) {
		return &model.Resolution{Execution: execution, Source: request.RootDirectory, WorkingDir: request.RootDirectory, BackendType: "local", Backend: map[string]any{}}, nil
	})
	result, err := p.Prepare(t.Context(), request)
	require.NoError(t, err)
	got := result.GetExecution()
	require.Equal(t, "plan_only", got.GetBeforeHooks()[0].GetName())
	require.Empty(t, got.GetAfterHooks())
	require.Len(t, got.GetApply().GetAfterHooks(), 1)
	require.Equal(t, "notify", got.GetApply().GetAfterHooks()[0].GetName())
	require.Equal(t, []string{"tool", "notify"}, got.GetApply().GetAfterHooks()[0].GetExecute())
	require.Nil(t, got.GetApply().GetApply())

	execution.Apply = &model.Execution{}
	result, err = p.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result.GetExecution().GetApply(), "an empty apply-time evaluation must be preserved")
	require.Empty(t, result.GetExecution().GetApply().GetBeforeHooks())
}

func TestPrepareOmitsExecutionWithoutHooks(t *testing.T) {
	request := componentRequest(t)
	p := New(func(context.Context, model.Request) (*model.Resolution, error) {
		return &model.Resolution{Source: request.RootDirectory, WorkingDir: request.RootDirectory, BackendType: "local", Backend: map[string]any{}}, nil
	})
	result, err := p.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.Nil(t, result.GetExecution())
}

// An unsupported hook must reach the user with its specific reason, unlike generic evaluator
// failures whose payloads can hold secrets.
func TestUnsupportedExecutionIsReportedSpecifically(t *testing.T) {
	cause := &model.Problem{Code: model.CodeUnsupportedHook, Summary: "Unsupported hook command `init-from-module` in before_hook \"setup\"", Key: "before_hook \"setup\"", Safe: true, Detail: "never runs", Hints: []string{"Remove it."}, File: "live/unit/terragrunt.hcl", Err: engine.ErrUnsupportedExecution}
	p := New(func(context.Context, model.Request) (*model.Resolution, error) { return nil, cause })
	request := componentRequest(t)
	request.Address = &pluginv1.ComponentAddress{Stack: "live", Component: "app", ComponentType: "terraform"}
	_, err := p.Prepare(t.Context(), request)
	require.Equal(t, codes.Unimplemented, status.Code(err))
	require.Contains(t, err.Error(), "init-from-module")
	require.Contains(t, err.Error(), "setup")
	detail, ok := component.FailureDetail(err)
	require.True(t, ok)
	require.Equal(t, "prepare", detail.Operation)
	require.Equal(t, "live/unit/terragrunt.hcl", detail.Location.File)
	require.True(t, detail.UserSafe)
	require.Equal(t, []string{"Remove it."}, detail.Hints)
	require.NotEmpty(t, detail.Unit)
}

func TestPrepareReturnsIAMAsDataWithASensitiveToken(t *testing.T) {
	request := componentRequest(t)
	p := New(func(_ context.Context, req model.Request) (*model.Resolution, error) {
		if req.Phase == "discover" {
			return &model.Resolution{}, nil
		}
		return &model.Resolution{Source: "git::https://example.test/m.git", WorkingDir: filepath.Join(request.RootDirectory, "cache"), IAM: &model.IAM{Role: "arn:aws:iam::1:role/x", SessionName: "s", DurationSeconds: 7200, WebIdentityToken: "token-secret"}}, nil
	})
	result, err := p.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "arn:aws:iam::1:role/x", result.Iam.Role)
	require.Equal(t, "s", result.Iam.SessionName)
	require.Equal(t, uint32(7200), result.Iam.DurationSeconds)
	token, err := component.Decode(result.Iam.WebIdentityToken)
	require.NoError(t, err)
	require.True(t, token.ContainsMarked(), "the token is a sensitive value")
	require.Len(t, result.Iam.WebIdentityToken.SensitivePaths, 1)
	require.Empty(t, result.Iam.WebIdentityToken.SensitivePaths[0].Steps, "the whole value is sensitive")
}

func TestIAMResponseOmitsWhatIsNotSet(t *testing.T) {
	none, err := iamResponse(nil)
	require.NoError(t, err)
	require.Nil(t, none)
	bare, err := iamResponse(&model.IAM{Role: "r", DurationSeconds: -5})
	require.NoError(t, err)
	require.Nil(t, bare.WebIdentityToken)
	require.Zero(t, bare.DurationSeconds)
	huge, err := iamResponse(&model.IAM{Role: "r", DurationSeconds: 1 << 40})
	require.NoError(t, err)
	require.Equal(t, uint32(1<<32-1), huge.DurationSeconds)
}

func TestFeatureSettingsAndEnvironmentMerge(t *testing.T) {
	config, err := encode(map[string]any{"enabled": true, "features": map[string]any{"flag": true, "size": 3, "tier": "gold", "env": "config"}})
	require.NoError(t, err)
	req := model.Request{Environment: map[string]string{"TG_FEATURE": "env=environment, extra=1"}}
	require.NoError(t, withFeatures(&req, config))
	require.Equal(t, map[string]string{"flag": "true", "size": "3", "tier": "gold", "env": "environment", "extra": "1"}, req.Features,
		"TG_FEATURE replaces the configured value per name")

	none := model.Request{}
	require.NoError(t, withFeatures(&none, nil))
	require.Nil(t, none.Features)

	for name, bad := range map[string]any{
		"not a map":    map[string]any{"features": "x"},
		"nested value": map[string]any{"features": map[string]any{"a": map[string]any{"b": 1}}},
		"list value":   map[string]any{"features": map[string]any{"a": []any{1}}},
	} {
		t.Run(name, func(t *testing.T) {
			value, err := encode(bad)
			require.NoError(t, err)
			var request model.Request
			require.Error(t, withFeatures(&request, value))
		})
	}
	unknown, err := encode(map[string]any{"features": map[string]any{}, "typo": 1})
	require.NoError(t, err)
	require.Error(t, settings(unknown))
	invalid := model.Request{Environment: map[string]string{"TG_FEATURE": "novalue"}}
	require.Error(t, withFeatures(&invalid, nil))
}

func TestOperationCommandNamesTheTerragruntAction(t *testing.T) {
	for _, test := range []struct {
		command   string
		arguments []string
		want      string
	}{
		{"plan", nil, "plan"},
		{"deploy", nil, "apply"},
		{"destroy", nil, "destroy"},
		{"apply", []string{"-destroy"}, "destroy"},
		{"apply", []string{"--destroy=true"}, "destroy"},
		{"apply", []string{"-destroy=false"}, "apply"},
		{"apply", []string{"-destroy", "-destroy=false"}, "apply"},
		{"plan", []string{"-destroy"}, "plan"},
	} {
		require.Equal(t, test.want, operationCommand(test.command, test.arguments), "%s %v", test.command, test.arguments)
	}
}

func TestPrepareReturnsParityData(t *testing.T) {
	request := componentRequest(t)
	yes := true
	p := New(func(_ context.Context, req model.Request) (*model.Resolution, error) {
		if req.Phase == "discover" {
			return &model.Resolution{}, nil
		}
		return &model.Resolution{
			Source: filepath.Join(request.RootDirectory, "module"), WorkingDir: filepath.Join(request.RootDirectory, "cache"),
			Inputs: map[string]any{}, BackendType: "local", Backend: map[string]any{},
			TerraformBinary: "tofu", FeatureFlags: map[string]string{"flag": "true"},
			Exclude: &model.Exclude{Actions: []string{"all"}, NoRun: &yes, ExcludeDependencies: true},
			Errors: &model.Errors{
				Retry:  []model.RetryPolicy{{Name: "net", Patterns: []string{"timeout"}, MaxAttempts: 3, SleepIntervalSec: 5}},
				Ignore: []model.IgnorePolicy{{Name: "known", Patterns: []string{"exists"}, Message: "fine", Signals: map[string]any{"note": "kept", "nothing": nil}}},
			},
			Warnings: []model.Warning{{Code: "read_config_before_hook_ignored", Path: filepath.Join(request.RootDirectory, "live", "terragrunt.hcl"), Message: "never runs"}},
		}, nil
	})
	result, err := p.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "tofu", result.TerraformBinary)
	require.Equal(t, map[string]string{"flag": "true"}, result.FeatureFlags)
	require.Equal(t, []string{"apply"}, result.Exclude.Actions, "the exclusion carries this operation, with no_run applied")
	require.True(t, result.Exclude.Condition)
	require.True(t, result.Exclude.ExcludeDependencies)
	require.Equal(t, "net", result.Errors.Retry[0].Name)
	require.EqualValues(t, 3, result.Errors.Retry[0].MaxAttempts)
	require.EqualValues(t, 5, result.Errors.Retry[0].SleepIntervalSeconds)
	require.Equal(t, []string{"exists"}, result.Errors.Ignore[0].IgnorableErrors)
	require.Contains(t, result.Errors.Ignore[0].Signals, "note")
	require.Contains(t, result.Errors.Ignore[0].Signals, "nothing")
	require.Len(t, result.Diagnostics, 1)
	require.Equal(t, "live/terragrunt.hcl", result.Diagnostics[0].Field)
	require.Equal(t, pluginv1.Severity_SEVERITY_WARNING, result.Diagnostics[0].Detail.Severity)
	require.Equal(t, "prepare", result.Diagnostics[0].Detail.Operation)
}

func TestPrepareOmitsExclusionWhenTheRunIsNotPrevented(t *testing.T) {
	request := componentRequest(t)
	no := false
	for name, exclude := range map[string]*model.Exclude{
		"no block":          nil,
		"no_run false":      {Actions: []string{"apply"}, NoRun: &no},
		"other action":      {Actions: []string{"plan"}},
		"all, no_run unset": {Actions: []string{"all"}},
	} {
		t.Run(name, func(t *testing.T) {
			p := New(func(_ context.Context, req model.Request) (*model.Resolution, error) {
				return &model.Resolution{Inputs: map[string]any{}, Backend: map[string]any{}, Exclude: exclude, WorkingDir: req.BasePath}, nil
			})
			result, err := p.Prepare(t.Context(), request)
			require.NoError(t, err)
			require.Nil(t, result.Exclude)
			require.Nil(t, result.Errors)
		})
	}
}

func TestDiscoveryPublishesCommandAndExclude(t *testing.T) {
	config := map[string]any{"settings": map[string]any{"terragrunt": map[string]any{}}}
	discoveryParity(config, &model.Resolution{})
	require.NotContains(t, config, "command")
	require.NotContains(t, config["settings"].(map[string]any)["terragrunt"], "exclude")

	discoveryParity(config, &model.Resolution{TerraformBinary: "tofu", Exclude: &model.Exclude{Actions: []string{"plan"}, ExcludeDependencies: true}})
	require.Equal(t, "tofu", config["command"])
	require.Equal(t, map[string]any{"actions": []string{"plan"}, "exclude_dependencies": true}, config["settings"].(map[string]any)["terragrunt"].(map[string]any)["exclude"])

	require.Equal(t, "opentofu", unitEngine(model.Request{Tofu: "terraform"}, &model.Resolution{TerraformBinary: "/opt/bin/tofu"}))
	require.Equal(t, "terraform", unitEngine(model.Request{Tofu: "terraform"}, &model.Resolution{}))
	require.Equal(t, "opentofu", unitEngine(model.Request{Tofu: "tofu"}, nil))
}

func TestPreparationTransmitsUnitFileCopyProvenance(t *testing.T) {
	for _, copied := range []bool{false, true} {
		t.Run(map[bool]string{false: "explicit generation", true: "adjacent copy"}[copied], func(t *testing.T) {
			request := componentRequest(t)
			p := New(func(_ context.Context, _ model.Request) (*model.Resolution, error) {
				return &model.Resolution{
					Generated:         map[string]string{".terraform.lock.hcl": "# pinned\n"},
					GeneratedPolicies: map[string]string{".terraform.lock.hcl": "overwrite"},
					UnitFileCopies:    map[string]bool{".terraform.lock.hcl": copied},
				}, nil
			})
			result, err := p.Prepare(t.Context(), request)
			require.NoError(t, err)
			require.Len(t, result.Files, 1)
			wire, err := proto.Marshal(result)
			require.NoError(t, err)
			decoded := &pluginv1.PrepareComponentResponse{}
			require.NoError(t, proto.Unmarshal(wire, decoded))
			require.Equal(t, copied, decoded.Files[0].UnitFileCopy)
			require.Equal(t, "overwrite", result.Files[0].IfExists)
		})
	}
}

func TestPreparationsSerializeGenerationAndParsing(t *testing.T) {
	request := componentRequest(t)
	parsing := make(chan struct{})
	release := make(chan struct{})
	secondGeneration := make(chan struct{}, 1)
	var calls atomic.Int32
	p := New(func(ctx context.Context, req model.Request) (*model.Resolution, error) {
		if req.Phase == "discover" {
			if calls.Add(1) == 2 {
				secondGeneration <- struct{}{}
			}
		} else if calls.Load() == 1 {
			close(parsing)
			<-release
		}
		return &model.Resolution{Source: request.RootDirectory, WorkingDir: request.RootDirectory}, nil
	})
	first := make(chan error, 1)
	go func() { _, err := p.Prepare(t.Context(), request); first <- err }()
	<-parsing
	second := make(chan error, 1)
	go func() { _, err := p.Prepare(t.Context(), request); second <- err }()
	select {
	case <-secondGeneration:
		t.Fatal("second generation started before the first parse finished")
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-first)
	require.NoError(t, <-second)
	select {
	case <-secondGeneration:
	default:
		t.Fatal("second generation did not run")
	}
}

// A unit that fails discovery is reported as a diagnostic for that unit; neither evaluated
// values nor the absolute project path reach the response.
func TestDiscoveryDiagnosticsRedactPayloads(t *testing.T) {
	for _, test := range []struct{ name, content string }{
		{"inline", "locals { password = \"secret-evaluated-value\" }"},
		{"parse", "locals { password = \"secret-evaluated-value\""},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := componentRequest(t)
			require.NoError(t, os.WriteFile(filepath.Join(request.RootDirectory, "terragrunt.hcl"), []byte(test.content), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(request.RootDirectory, "main.tf"), []byte("terraform {}"), 0o600))
			response, err := New(nil).Discover(t.Context(), &pluginv1.DiscoverComponentsRequest{
				RootDirectory: request.RootDirectory, Provider: request.Provider,
				ProviderConfiguration: request.ProviderConfiguration, Operation: request.Operation, IncludePaths: []string{"**"},
			})
			require.NoError(t, err)
			require.Empty(t, response.GetComponents())
			require.NotEmpty(t, response.GetDiagnostics(), "the failed unit is reported")
			serialized := prototext.Format(response)
			require.NotContains(t, serialized, "secret-evaluated-value")
			require.NotContains(t, serialized, request.RootDirectory)
		})
	}
}

// Scheduling edges must be available from discovery, before preparation can read outputs.
func TestDiscoveryOutputDependencyOrdering(t *testing.T) {
	for _, test := range []struct {
		name, blocks string
		kinds        []pluginv1.ComponentDependency_Kind
	}{
		{"output only", `dependency "network" { config_path = "../network" }`, []pluginv1.ComponentDependency_Kind{pluginv1.ComponentDependency_OUTPUT}},
		{"output and ordering", `dependency "network" { config_path = "../network" }
dependencies { paths = ["../network", "../network"] }`, []pluginv1.ComponentDependency_Kind{pluginv1.ComponentDependency_OUTPUT}},
		{"ordering only", `dependencies { paths = ["../network"] }`, []pluginv1.ComponentDependency_Kind{pluginv1.ComponentDependency_ORDER}},
		{"skip outputs", `dependency "network" {
 config_path = "../network"
 skip_outputs = true
}`, []pluginv1.ComponentDependency_Kind{pluginv1.ComponentDependency_ORDER}},
		{"disabled", `dependency "network" {
 config_path = "../network"
 enabled = false
}`, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			for _, dir := range []string{"module", "live/app", "live/network"} {
				require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0o755))
			}
			config := "terraform { source = \"../../module\" }\n"
			require.NoError(t, os.WriteFile(filepath.Join(root, "live/network/terragrunt.hcl"), []byte(config), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(root, "live/app/terragrunt.hcl"), []byte(config+test.blocks+"\ninputs = { deferred = run_cmd(\"must-not-execute\") }\n"), 0o600))
			opts, err := encode(map[string]any{"enabled": true, "mode": "native", "discovery": map[string]any{"enabled": true}})
			require.NoError(t, err)
			var phases []string
			p := New(func(ctx context.Context, req model.Request) (*model.Resolution, error) {
				phases = append(phases, req.Phase)
				return engine.Resolve(ctx, req)
			})
			response, err := p.Discover(t.Context(), &pluginv1.DiscoverComponentsRequest{
				RootDirectory: root, Provider: "terragrunt", ProviderConfiguration: opts,
				IncludePaths: []string{"live/**"}, Operation: &pluginv1.ComponentOperation{Mode: "native", Command: "plan"},
				Invocation: &pluginv1.Invocation{Environment: map[string]string{"ATMOS_TERRAFORM_BINARY": "must-not-execute"}},
			})
			require.NoError(t, err)
			require.Empty(t, response.Diagnostics)
			require.Equal(t, []string{"inventory"}, phases)
			require.Len(t, response.Components, 2)
			app := response.Components[0]
			network := response.Components[1]
			require.Equal(t, "app", app.Address.Component)
			require.Equal(t, "network", network.Address.Component)
			require.Len(t, app.Dependencies, len(test.kinds))
			for i, kind := range test.kinds {
				require.Equal(t, kind, app.Dependencies[i].Kind)
				require.Equal(t, network.Address, app.Dependencies[i].Target)
			}
			require.Empty(t, network.Dependencies)
			require.NoDirExists(t, filepath.Join(root, "live/app/.terragrunt-cache"))
		})
	}
}

// The engine package and the provider classify executables with one rule.
func TestEngineNameMatchesTheSharedOpenTofuRule(t *testing.T) {
	for _, binary := range []string{"tofu", "opentofu", "tofu-wrapper", "TOFU.exe", "terraform", "", "/opt/tofu/bin/terraform"} {
		want := "terraform"
		if engine.IsOpenTofu(binary) {
			want = "opentofu"
		}
		require.Equal(t, want, engineName(binary), binary)
	}
	require.Equal(t, "opentofu", engineName("TOFU.exe"))
	require.Equal(t, "terraform", engineName("/opt/tofu/bin/terraform"))
}

func TestRPCErrorPreservesDestroyProtection(t *testing.T) {
	err := rpcError(fmt.Errorf("private evaluator context: %w", engine.ErrPreventDestroy))
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.NotContains(t, status.Convert(err).Message(), "private evaluator context")
}

func TestPreparationExportsOverlayModes(t *testing.T) {
	request := componentRequest(t)
	p := New(func(_ context.Context, _ model.Request) (*model.Resolution, error) {
		return &model.Resolution{
			Source: filepath.Join(request.RootDirectory, "module"), WorkingDir: filepath.Join(request.RootDirectory, "cache"), Inputs: map[string]any{}, BackendType: "local", Backend: map[string]any{"path": "relative.tfstate"},
			Overlay:      map[string][]byte{"bin/run.sh": []byte("#!/bin/sh\n"), "plain.tf": []byte("# plain"), "shadow.sh": []byte("old")},
			OverlayModes: map[string]uint32{"bin/run.sh": 0o755, "shadow.sh": 0o755},
			Generated:    map[string]string{"shadow.sh": "new"},
		}, nil
	})
	result, err := p.Prepare(t.Context(), request)
	require.NoError(t, err)
	modes := map[string]uint32{}
	for _, file := range result.Files {
		modes[file.Path] = file.Mode
	}
	require.Equal(t, map[string]uint32{"bin/run.sh": 0o755, "plain.tf": 0, "shadow.sh": 0}, modes)
	require.Equal(t, "bin/run.sh", result.Files[0].Path)
	require.Equal(t, "shadow.sh", result.Files[len(result.Files)-1].Path)
}
