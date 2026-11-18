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
			require.Equal(t, filepath.Join(request.RootDirectory, "live", "terragrunt.stack.hcl"), req.Path)
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
			require.NoError(t, os.WriteFile(filepath.Join(root, "live", "network", "terragrunt.hcl"), []byte(config), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(root, "live", "app", "terragrunt.hcl"), []byte(config+test.blocks+"\ninputs = { deferred = run_cmd(\"must-not-execute\") }\n"), 0o600))
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
			require.NoDirExists(t, filepath.Join(root, "live", "app", ".terragrunt-cache"))
		})
	}
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

func TestDiscoveryRejectsScopeAndConfiguration(t *testing.T) {
	for _, name := range []string{"provider", "settings", "empty scope", "imports", "unknown option", "non-native mode"} {
		t.Run(name, func(t *testing.T) {
			base := componentRequest(t)
			req := &pluginv1.DiscoverComponentsRequest{RootDirectory: base.RootDirectory, Provider: base.Provider, ProviderConfiguration: base.ProviderConfiguration, Operation: base.Operation, IncludePaths: []string{"live/**"}}
			switch name {
			case "provider":
				req.Provider = "other"
			case "settings":
				req.ProviderConfiguration = nil
			case "empty scope":
				req.IncludePaths = nil
			case "imports":
				req.Imports = []string{"terragrunt://live"}
			case "unknown option":
				req.ProviderConfiguration, _ = encode(map[string]any{"binary": "terragrunt"})
			case "non-native mode":
				req.ProviderConfiguration, _ = encode(map[string]any{"mode": "cli"})
			}
			p := New(func(context.Context, model.Request) (*model.Resolution, error) {
				t.Fatal("invalid requests must not evaluate HCL")
				return nil, nil
			})
			_, err := p.Discover(t.Context(), req)
			require.Error(t, err)
		})
	}
}

func TestDiscoveryRejectsUnrepresentableInventory(t *testing.T) {
	for _, name := range []string{"unit outside root", "module outside root", "missing module", "generation outside root", "watch outside root", "dependency outside scope", "evaluator failure"} {
		t.Run(name, func(t *testing.T) {
			base := componentRequest(t)
			outside := t.TempDir()
			unit := model.DiscoveredUnit{Stack: "live", Name: "app", Config: model.Resolution{Path: filepath.Join(base.RootDirectory, "live", "app", "terragrunt.hcl"), Folders: []string{filepath.Join(base.RootDirectory, "module")}}}
			switch name {
			case "unit outside root":
				unit.Config.Path = filepath.Join(outside, "terragrunt.hcl")
			case "module outside root":
				unit.Config.Folders = []string{outside}
			case "missing module":
				unit.Config.Folders = nil
			case "generation outside root":
				unit.GenerationRoot = filepath.Join(outside, "terragrunt.stack.hcl")
			case "watch outside root":
				unit.Config.Files = []string{filepath.Join(outside, "secret.hcl")}
			case "dependency outside scope":
				unit.Config.Dependencies = []string{filepath.Join(base.RootDirectory, "other", "terragrunt.hcl")}
				require.NoError(t, os.MkdirAll(filepath.Dir(unit.Config.Dependencies[0]), 0o700))
				require.NoError(t, os.WriteFile(unit.Config.Dependencies[0], []byte(""), 0o600))
			}
			p := New(func(context.Context, model.Request) (*model.Resolution, error) {
				if name == "evaluator failure" {
					return nil, context.Canceled
				}
				return &model.Resolution{Inventory: []model.DiscoveredUnit{unit}}, nil
			})
			result, err := p.Discover(t.Context(), &pluginv1.DiscoverComponentsRequest{RootDirectory: base.RootDirectory, Provider: base.Provider, ProviderConfiguration: base.ProviderConfiguration, Operation: base.Operation, IncludePaths: []string{"live/**"}})
			if name == "evaluator failure" {
				require.Nil(t, result)
				require.Equal(t, codes.Canceled, status.Code(err))
				return
			}
			// A unit that cannot be represented is reported on its own and never hides healthy units.
			require.NoError(t, err)
			if name == "dependency outside scope" {
				// The unit is kept with its out-of-scope dependency; the host fails it, naming the target.
				require.Empty(t, result.Diagnostics)
				require.Len(t, result.Components, 1)
				component := result.Components[0]
				require.Equal(t, "live", component.Address.Stack)
				require.Equal(t, "app", component.Address.Component)
				require.Len(t, component.Dependencies, 1)
				target := component.Dependencies[0].Target
				require.Equal(t, "default", target.Stack)
				require.Equal(t, "other", target.Component)
				require.Equal(t, "terraform", target.ComponentType)
				require.Equal(t, "terraform", component.Address.ComponentType)
				require.Equal(t, pluginv1.ComponentDependency_ORDER, component.Dependencies[0].Kind)
				return
			}
			require.Empty(t, result.Components)
			require.Len(t, result.Diagnostics, 1)
			detail := result.Diagnostics[0].Detail
			require.Equal(t, "app", detail.Unit)
			require.Equal(t, "live", detail.Stack)
			require.Equal(t, "discover", detail.Operation)
			require.NotEqual(t, pluginv1.Severity_SEVERITY_WARNING, detail.Severity)
		})
	}
}

func TestPreparationWithoutInputsAndDefaultState(t *testing.T) {
	req := componentRequest(t)
	req.Invocation = nil
	var err error
	req.Configuration, err = encode(map[string]any{})
	require.NoError(t, err)
	p := New(func(_ context.Context, request model.Request) (*model.Resolution, error) {
		require.Equal(t, "terraform", request.Tofu)
		require.Empty(t, request.Environment)
		return &model.Resolution{Source: filepath.Join(req.RootDirectory, "module"), WorkingDir: filepath.Join(req.RootDirectory, "cache")}, nil
	})
	result, err := p.Prepare(t.Context(), req)
	require.NoError(t, err)
	inputs, err := component.Decode(result.Inputs)
	require.NoError(t, err)
	require.False(t, inputs.IsNull())
	require.True(t, inputs.Type().IsObjectType())
	require.Empty(t, inputs.AsValueMap())
	require.Equal(t, "terraform", result.Engine)
	require.Equal(t, "default", result.State.Workspace)
	require.Equal(t, "local", result.State.BackendType)
}

func TestStateAndPreparationRejectEscapedProvenance(t *testing.T) {
	req := componentRequest(t)
	var err error
	req.Configuration, err = encode(map[string]any{})
	require.NoError(t, err)
	p := New(func(context.Context, model.Request) (*model.Resolution, error) {
		return &model.Resolution{Inputs: map[string]any{}, Files: []string{filepath.Join(filepath.Dir(req.RootDirectory), "outside.hcl")}}, nil
	})
	_, err = p.Prepare(t.Context(), req)
	require.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = p.Resolve(t.Context(), req)
	require.Equal(t, codes.Unimplemented, status.Code(err))
	req.Source = "../outside"
	_, err = p.Resolve(t.Context(), req)
	require.Equal(t, codes.Unimplemented, status.Code(err))
	p = New(func(context.Context, model.Request) (*model.Resolution, error) { return nil, context.Canceled })
	req.Source = "live/unit/terragrunt.hcl"
	_, err = p.Resolve(t.Context(), req)
	require.Equal(t, codes.Canceled, status.Code(err))
	req.Configuration, err = encode(map[string]any{"settings": map[string]any{"terragrunt": map[string]any{"generation_root": "../escape"}}})
	require.NoError(t, err)
	_, err = p.Prepare(t.Context(), req)
	require.Equal(t, codes.Unimplemented, status.Code(err))
}

// Watched paths reach the host as slash-separated, project-relative strings. Terragrunt reports paths with
// mixed separators on Windows (a native directory plus a slash-separated suffix), so every spelling of the
// same file must normalize to one slash-separated relative path.
func TestRelativeNormalizesMixedSeparators(t *testing.T) {
	// Terragrunt reports physical paths, so compare from the physical root.
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	for name, spelling := range map[string]string{
		"native":       filepath.Join(root, "live", "terragrunt.stack.hcl"),
		"slash suffix": root + "/live/terragrunt.stack.hcl",
		"redundant":    root + "/live/./sub/../terragrunt.stack.hcl",
	} {
		t.Run(name, func(t *testing.T) {
			got, err := relative(root, spelling)
			require.NoError(t, err)
			require.Equal(t, "live/terragrunt.stack.hcl", got)
		})
	}
	for _, outside := range []string{filepath.Dir(root), root + "/../elsewhere/terragrunt.hcl"} {
		_, err := relative(root, outside)
		require.ErrorIs(t, err, ErrUnsupported)
	}
}
