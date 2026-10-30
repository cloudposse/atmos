package provider

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/engine"
	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
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
	cause := fmt.Errorf("%w: before_hook %q lists the %q lifecycle command", engine.ErrUnsupportedExecution, "setup", "init-from-module")
	p := New(func(context.Context, model.Request) (*model.Resolution, error) { return nil, cause })
	_, err := p.Prepare(t.Context(), componentRequest(t))
	require.Equal(t, codes.Unimplemented, status.Code(err))
	require.Contains(t, err.Error(), "init-from-module")
	require.Contains(t, err.Error(), "setup")
}
