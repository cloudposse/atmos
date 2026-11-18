package provider

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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

func TestDiscoveryDiagnosticsRedactPayloads(t *testing.T) {
	for _, test := range []struct{ name, content, reason string }{
		{"inline", "locals { password = \"secret-evaluated-value\" }", "MISSING_TERRAFORM_SOURCE"},
		{"parse", "locals { password = \"secret-evaluated-value\"", "EVALUATION_FAILED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := componentRequest(t)
			require.NoError(t, os.WriteFile(filepath.Join(request.RootDirectory, "terragrunt.hcl"), []byte(test.content), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(request.RootDirectory, "main.tf"), []byte("terraform {}"), 0o600))
			_, err := New(nil).Discover(t.Context(), &pluginv1.DiscoverComponentsRequest{
				RootDirectory: request.RootDirectory, Provider: request.Provider,
				ProviderConfiguration: request.ProviderConfiguration, Operation: request.Operation, IncludePaths: []string{"**"},
			})
			require.Equal(t, codes.InvalidArgument, status.Code(err))
			details := status.Convert(err).Details()
			require.Len(t, details, 1)
			info := details[0].(*errdetails.ErrorInfo)
			require.Equal(t, "atmos.terragrunt.discovery.v1", info.Domain)
			require.Equal(t, test.reason, info.Reason)
			require.Equal(t, map[string]string{"file": "terragrunt.hcl"}, info.Metadata)
			require.NotContains(t, status.Convert(err).Proto().String(), "secret-evaluated-value")
			require.NotContains(t, status.Convert(err).Proto().String(), request.RootDirectory)
		})
	}
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
			}
			p := New(func(context.Context, model.Request) (*model.Resolution, error) {
				if name == "evaluator failure" {
					return nil, context.Canceled
				}
				return &model.Resolution{Inventory: []model.DiscoveredUnit{unit}}, nil
			})
			result, err := p.Discover(t.Context(), &pluginv1.DiscoverComponentsRequest{RootDirectory: base.RootDirectory, Provider: base.Provider, ProviderConfiguration: base.ProviderConfiguration, Operation: base.Operation, IncludePaths: []string{"live/**"}})
			require.Nil(t, result)
			if name == "evaluator failure" {
				require.Equal(t, codes.Canceled, status.Code(err))
			} else {
				require.Equal(t, codes.Unimplemented, status.Code(err))
			}
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

func fixture(t *testing.T, name string) string {
	t.Helper()
	source, err := filepath.Abs(filepath.Join("..", "..", "fixtures", name))
	require.NoError(t, err)
	root := t.TempDir()
	require.NoError(t, os.CopyFS(root, os.DirFS(source)))
	root, err = filepath.EvalSymlinks(root)
	require.NoError(t, err)
	return root
}

func TestDiscoveryFixtures(t *testing.T) {
	for _, test := range []struct {
		name, scope string
		names       []string
	}{{"advanced", "live/dev/**", []string{"app", "cache", "database", "network"}}, {"explicit-stack", "live/**", []string{"dev/app", "dev/network", "prod/app", "prod/network"}}, {"explicit-stack", "**", []string{"dev/app", "dev/network", "prod/app", "prod/network"}}} {
		t.Run(test.name, func(t *testing.T) {
			root := fixture(t, test.name)
			opts, err := encode(map[string]any{"enabled": true, "mode": "native", "discovery": map[string]any{"enabled": true, "included_paths": []string{test.scope}}})
			require.NoError(t, err)
			// Exercise the real evaluator; include its diagnostics only inside local tests.
			p := New(func(ctx context.Context, req model.Request) (*model.Resolution, error) {
				result, err := engine.Resolve(ctx, req)
				require.NoError(t, err)
				return result, err
			})
			request := &pluginv1.DiscoverComponentsRequest{RootDirectory: root, Provider: "terragrunt", ProviderConfiguration: opts, IncludePaths: []string{test.scope}, Operation: &pluginv1.ComponentOperation{Mode: "native", Command: "list"}, Invocation: &pluginv1.Invocation{Environment: map[string]string{"ATMOS_TERRAFORM_BINARY": "must-not-execute"}}}
			response, err := p.Discover(t.Context(), request)
			require.NoError(t, err)
			var expected map[string]map[string]pluginv1.ComponentDependency_Kind
			if test.name == "advanced" {
				expected = map[string]map[string]pluginv1.ComponentDependency_Kind{
					"app":      {"network": pluginv1.ComponentDependency_ORDER, "database": pluginv1.ComponentDependency_OUTPUT, "cache": pluginv1.ComponentDependency_OUTPUT},
					"cache":    {"network": pluginv1.ComponentDependency_OUTPUT},
					"database": {"network": pluginv1.ComponentDependency_OUTPUT},
				}
			} else {
				expected = map[string]map[string]pluginv1.ComponentDependency_Kind{
					"dev/app":  {"dev/network": pluginv1.ComponentDependency_OUTPUT},
					"prod/app": {"prod/network": pluginv1.ComponentDependency_OUTPUT},
				}
			}
			names := []string{}
			for _, instance := range response.Components {
				names = append(names, instance.Address.Component)
				require.Equal(t, "terraform", instance.Address.ComponentType)
				require.NotEmpty(t, instance.Sources)
				dependencies := make(map[string]pluginv1.ComponentDependency_Kind)
				for _, dependency := range instance.Dependencies {
					require.NotNil(t, dependency.Target)
					require.Equal(t, instance.Address.Stack, dependency.Target.Stack)
					require.Equal(t, "terraform", dependency.Target.ComponentType)
					require.NotContains(t, dependencies, dependency.Target.Component)
					dependencies[dependency.Target.Component] = dependency.Kind
				}
				require.Len(t, dependencies, len(expected[instance.Address.Component]))
				for target, kind := range expected[instance.Address.Component] {
					require.Contains(t, dependencies, target)
					require.Equal(t, kind, dependencies[target])
				}
			}
			require.Equal(t, test.names, names)
			require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
				require.NoError(t, err)
				require.NotEqual(t, ".terragrunt-stack", entry.Name())
				require.NotEqual(t, ".terragrunt-cache", entry.Name())
				return nil
			}))
		})
	}
}

func TestExplicitStackStateAndPreparation(t *testing.T) {
	root := fixture(t, "explicit-stack")
	opts, err := encode(map[string]any{"enabled": true, "mode": "native", "discovery": map[string]any{"enabled": true, "included_paths": []string{"live/**"}}})
	require.NoError(t, err)
	provider := New(nil)
	inventory, err := provider.Discover(t.Context(), &pluginv1.DiscoverComponentsRequest{RootDirectory: root, Provider: "terragrunt", ProviderConfiguration: opts, IncludePaths: []string{"live/**"}, Operation: &pluginv1.ComponentOperation{Mode: "native", Command: "plan"}, Invocation: &pluginv1.Invocation{Environment: map[string]string{"ATMOS_TERRAFORM_BINARY": "must-not-execute"}}})
	require.NoError(t, err)
	network := inventory.Components[1]
	require.Equal(t, "dev/network", network.Address.Component)
	request := &pluginv1.ComponentRequest{RootDirectory: root, Provider: "terragrunt", ProviderConfiguration: opts, Address: network.Address, Source: network.Source, Configuration: network.Configuration, Operation: &pluginv1.ComponentOperation{Mode: "native", Command: "plan"}, Invocation: &pluginv1.Invocation{Environment: map[string]string{"ATMOS_TERRAFORM_BINARY": "must-not-execute"}}}
	state, err := provider.Resolve(t.Context(), request)
	require.NoError(t, err)
	require.NoDirExists(t, filepath.Join(root, "live", ".terragrunt-stack"))
	request.Effects = &pluginv1.ComponentEffects{Commands: true, Network: true, Secrets: true, Writes: true, StateReads: true}
	prepared, err := provider.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.DirExists(t, filepath.Join(root, "live", ".terragrunt-stack"))
	require.Equal(t, state.State.WorkingDirectory, prepared.State.WorkingDirectory)
	var before, after map[string]any
	require.NoError(t, decode(state.State.Backend, &before))
	require.NoError(t, decode(prepared.State.Backend, &after))
	require.Equal(t, before, after)
	require.Equal(t, filepath.Join(root, "state", "dev", "network.tfstate"), after["path"])
	require.Len(t, prepared.Files, 2)
	require.Equal(t, "terragrunt.hcl", prepared.Files[0].Path)
	require.Equal(t, "terragrunt.values.hcl", prepared.Files[1].Path)
	require.NotNil(t, prepared.Inputs)
}
